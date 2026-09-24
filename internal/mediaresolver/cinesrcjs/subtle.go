// Package cinesrcjs resolves CineSrc streams directly by running the site's
// own challenge scripts (donut.js + the *-prod.js module) in an embedded
// goja runtime with Go-native shims for WebCrypto, fetch and Workers, and
// the proof-of-work WASM via wazero. No browser is involved.
//
// The protocol this participates in is documented in
// docs/cinesrc-direct-protocol.md.
package cinesrcjs

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"errors"
	"fmt"
	"hash"
	"os"
	"strings"
	"sync"

	"github.com/dop251/goja"
)

// keyKind enumerates the key types the challenge scripts create.
type keyKind int

const (
	keyAES keyKind = iota
	keyHKDF
	keyRSAPublic
	keyECPublic
	keyECPrivate
)

type goKey struct {
	kind keyKind
	raw  []byte // AES / HKDF input material
	pub  *rsa.PublicKey
	// ECDH keys
	ecPriv      *ecdh.PrivateKey
	ecPub       *ecdh.PublicKey
	algName     string   // algorithm name reported by key.algorithm
	namedCurve  string   // EC named curve ("P-256", ...)
	usages      []string // usages reported by key.usages
	extractable bool
}

type subtleShim struct {
	vm    *goja.Runtime
	tag   string // per-runtime label for debug tracing
	mu    sync.Mutex
	keys  map[int]*goKey
	nextK int
}

func newSubtleShim(vm *goja.Runtime) *subtleShim {
	return &subtleShim{vm: vm, keys: map[int]*goKey{}}
}

func (s *subtleShim) newKeyObj(k *goKey) goja.Value {
	s.mu.Lock()
	id := s.nextK
	s.nextK++
	s.keys[id] = k
	s.mu.Unlock()

	o := s.vm.NewObject()
	_ = o.Set("__goKeyID", id)
	alg := s.vm.NewObject()
	switch k.kind {
	case keyAES:
		_ = o.Set("type", "secret")
		_ = alg.Set("name", k.algName)
		_ = alg.Set("length", len(k.raw)*8)
		_ = o.Set("algorithm", alg)
		_ = o.Set("extractable", k.extractable)
		_ = o.Set("usages", k.usages)
	case keyHKDF:
		_ = o.Set("type", "secret")
		_ = alg.Set("name", "HKDF")
		_ = alg.Set("hash", "SHA-256")
		_ = alg.Set("length", len(k.raw)*8)
		_ = o.Set("algorithm", alg)
		_ = o.Set("extractable", false)
		_ = o.Set("usages", []string{"deriveKey", "deriveBits"})
	case keyRSAPublic:
		_ = o.Set("type", "public")
		_ = alg.Set("name", k.algName)
		_ = alg.Set("hash", "SHA-256")
		_ = alg.Set("modulusLength", k.pub.N.BitLen())
		exp := s.vm.NewObject()
		_ = exp.Set("0", 1)
		_ = exp.Set("1", 0)
		_ = exp.Set("2", 1)
		_ = alg.Set("publicExponent", exp)
		_ = o.Set("algorithm", alg)
		_ = o.Set("extractable", true)
		_ = o.Set("usages", k.usages)
	case keyECPublic, keyECPrivate:
		if k.kind == keyECPublic {
			_ = o.Set("type", "public")
			_ = o.Set("extractable", true) // public keys are always extractable
		} else {
			_ = o.Set("type", "private")
			_ = o.Set("extractable", k.extractable)
		}
		_ = alg.Set("name", k.algName)
		_ = alg.Set("namedCurve", k.namedCurve)
		_ = o.Set("algorithm", alg)
		_ = o.Set("usages", k.usages)
	}
	return o
}

// fileLog appends a tagged line to the CINESRCJS_DEBUG_LOG file when set.ged line to the CINESRCJS_DEBUG_LOG file when set.
func fileLog(tag, msg string) {
	f := os.Getenv("CINESRCJS_DEBUG_LOG")
	if f == "" {
		return
	}
	fh, err := os.OpenFile(f, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err == nil {
		fmt.Fprintln(fh, tag+" "+msg)
		fh.Close()
	}
}

// fileDbg appends a line to the CINESRCJS_DEBUG_LOG file when set. Unlike
// vmDbg it never forwards to the console, so hot-path tracing stays out of
// the application log.
func (s *subtleShim) fileDbg(msg string) {
	fileLog(s.tag, msg)
}

// describeVal renders a goja value for the debug log: its exported Go type
// plus a short String() preview.
func describeVal(v goja.Value) string {
	if v == nil {
		return "<nil>"
	}
	return fmt.Sprintf("exportT=%T str=%.80q", v.Export(), v.String())
}

// cryptoKeyArgError panics with the exact TypeError a browser raises when a
// subtle method receives a non-CryptoKey argument. The challenge scripts
// deliberately call subtle methods with bogus keys and validate the thrown
// message, so the shim must reproduce Chrome's wording verbatim.
func (s *subtleShim) cryptoKeyArgError(op string, param int) {
	subtleError(s.vm, fmt.Sprintf("Failed to execute '%s' on 'SubtleCrypto': parameter %d is not of type 'CryptoKey'.", op, param))
}

// keyFromVal unwraps a shim CryptoKey object. Failures panic with the same
// TypeError a browser raises (caught by promiseWrap and turned into a
// rejection), matching the challenge's environment probes.
func (s *subtleShim) keyFromVal(op string, param int, v goja.Value) *goKey {
	obj, ok := v.(*goja.Object)
	if !ok {
		s.fileDbg(fmt.Sprintf("subtle: key is not an object: %s", describeVal(v)))
		s.cryptoKeyArgError(op, param)
		return nil
	}
	idV := obj.Get("__goKeyID")
	if idV == nil {
		s.fileDbg(fmt.Sprintf("subtle: not a shim key: keys=%v preview=%.120s", obj.Keys(), obj.String()))
		s.cryptoKeyArgError(op, param)
		return nil
	}
	var id int
	switch n := idV.Export().(type) {
	case int:
		id = n
	case int64:
		id = int(n)
	case float64:
		id = int(n)
	default:
		s.cryptoKeyArgError(op, param)
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	k, ok := s.keys[id]
	if !ok {
		s.cryptoKeyArgError(op, param)
		return nil
	}
	return k
}

// toBytes converts a BufferSource (ArrayBuffer / Uint8Array / Int8Array ...)
// or a plain string into bytes.
func toBytes(vm *goja.Runtime, v goja.Value) ([]byte, error) {
	if v == nil || v == goja.Undefined() || v == goja.Null() {
		return nil, nil
	}
	switch t := v.Export().(type) {
	case string:
		// JS binary strings (from atob) hold one byte per code unit;
		// UTF-8 encoding would corrupt bytes >= 0x80.
		out := make([]byte, 0, len(t))
		binary := true
		for _, r := range t {
			if r >= 256 {
				binary = false
				break
			}
			out = append(out, byte(r))
		}
		if binary {
			return out, nil
		}
		return []byte(t), nil
	case []byte:
		return t, nil
	case goja.ArrayBuffer:
		return t.Bytes(), nil
	case map[string]interface{}:
		// typed array exported as an object
		obj := v.ToObject(vm)
		return typedArrayBytes(vm, obj)
	default:
		obj, ok := v.(*goja.Object)
		if !ok {
			return nil, fmt.Errorf("subtle: unsupported buffer source %T", v.Export())
		}
		return typedArrayBytes(vm, obj)
	}
}

func typedArrayBytes(vm *goja.Runtime, obj *goja.Object) ([]byte, error) {
	buf := obj.Get("buffer")
	if buf == nil || buf == goja.Undefined() {
		// maybe an array of numbers
		if l := obj.Get("length"); l != nil {
			n := int(l.ToInteger())
			out := make([]byte, n)
			for i := 0; i < n; i++ {
				out[i] = byte(obj.Get(fmt.Sprintf("%d", i)).ToInteger())
			}
			return out, nil
		}
		return nil, errors.New("subtle: not a buffer source")
	}
	ab, ok := buf.Export().(goja.ArrayBuffer)
	if !ok {
		return nil, errors.New("subtle: bad buffer")
	}
	raw := ab.Bytes()
	off := int(obj.Get("byteOffset").ToInteger())
	lv := obj.Get("byteLength")
	if lv == nil {
		lv = obj.Get("length")
	}
	l := int(lv.ToInteger())
	if off+l > len(raw) {
		return nil, errors.New("subtle: buffer out of range")
	}
	return raw[off : off+l], nil
}

func subtleError(vm *goja.Runtime, msg string) goja.Value {
	panic(vm.NewTypeError(msg))
}

// domError panics with a DOMException-style error carrying a custom name
// (e.g. "OperationError"), matching how browsers surface WebCrypto failures.
func domError(vm *goja.Runtime, name, msg string) {
	e := vm.NewGoError(errors.New(msg))
	_ = e.Set("name", name)
	panic(e)
}

func headBytes(b []byte, n int) []byte {
	if len(b) > n {
		return b[:n]
	}
	return b
}

func (s *subtleShim) vmDbg(msg string) {
	fileLog(s.tag, msg)
	if v := s.vm.GlobalObject().Get("__consoleFn"); v != nil && !goja.IsUndefined(v) {
		if fn, ok := goja.AssertFunction(v); ok {
			_, _ = fn(goja.Undefined(), s.vm.ToValue(msg))
		}
	}
}

// traceOps logs a subtle operation entry (op name + argument types) to the
// debug file when CINESRCJS_DEBUG_LOG is set.
func (s *subtleShim) traceOps(op string, args []goja.Value) {
	f := os.Getenv("CINESRCJS_DEBUG_LOG")
	if f == "" {
		return
	}
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = describeVal(a)
	}
	s.fileDbg(fmt.Sprintf("subtle.%s args=[%s]", op, strings.Join(parts, ", ")))
}

// promiseWrap adapts a synchronous subtle implementation to real WebCrypto
// semantics: every subtle method returns a Promise, and failures surface as
// promise rejections — never as synchronous throws. The challenge scripts
// rely on this (e.g. exportKey(...).then(ok, handler) with a bogus key), so
// a synchronous TypeError here would escape the script's handler and reject
// the whole challenge.
func (s *subtleShim) promiseWrap(fn func(call goja.FunctionCall) goja.Value) func(call goja.FunctionCall) goja.Value {
	vm := s.vm
	return func(call goja.FunctionCall) (result goja.Value) {
		p, resolve, reject := vm.NewPromise()
		func() {
			defer func() {
				if r := recover(); r != nil {
					switch v := r.(type) {
					case goja.Value:
						_ = reject(v)
					case error:
						_ = reject(vm.NewGoError(v))
					default:
						panic(r)
					}
					return
				}
			}()
			_ = resolve(fn(call))
		}()
		return vm.ToValue(p)
	}
}

func (s *subtleShim) install(target *goja.Object) {
	vm := s.vm
	mustSet(target.Set("importKey", vm.ToValue(s.promiseWrap(s.syncImportKey))))
	mustSet(target.Set("exportKey", vm.ToValue(s.promiseWrap(s.syncExportKey))))
	mustSet(target.Set("generateKey", vm.ToValue(s.promiseWrap(s.syncGenerateKey))))
	mustSet(target.Set("digest", vm.ToValue(s.promiseWrap(s.syncDigest))))
	mustSet(target.Set("encrypt", vm.ToValue(s.promiseWrap(func(call goja.FunctionCall) goja.Value {
		return s.syncCrypt(call, true)
	}))))
	mustSet(target.Set("decrypt", vm.ToValue(s.promiseWrap(func(call goja.FunctionCall) goja.Value {
		return s.syncCrypt(call, false)
	}))))
	mustSet(target.Set("deriveKey", vm.ToValue(s.promiseWrap(s.syncDeriveKey))))
	mustSet(target.Set("deriveBits", vm.ToValue(s.promiseWrap(s.syncDeriveBits))))
}

// usagesFromArg extracts a JS KeyUsage array into a Go slice.
func usagesFromArg(vm *goja.Runtime, v goja.Value) []string {
	arr, ok := v.(*goja.Object)
	if !ok {
		return nil
	}
	lv := arr.Get("length")
	if lv == nil || goja.IsUndefined(lv) {
		return nil
	}
	n := int(lv.ToInteger())
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, arr.Get(fmt.Sprint(i)).String())
	}
	return out
}

// ecCurve maps a WebCrypto named curve to its Go implementation.
func ecCurve(named string) (ecdh.Curve, bool) {
	switch strings.ToUpper(named) {
	case "P-256":
		return ecdh.P256(), true
	case "P-384":
		return ecdh.P384(), true
	case "P-521":
		return ecdh.P521(), true
	}
	return nil, false
}

func (s *subtleShim) syncImportKey(call goja.FunctionCall) goja.Value {
	vm := s.vm
	s.traceOps("importKey", []goja.Value{call.Argument(0), call.Argument(1), call.Argument(2)})
	format := call.Argument(0).String()
	data, err := toBytes(vm, call.Argument(1))
	if err != nil {
		subtleError(vm, "importKey: "+err.Error())
	}
	algV := call.Argument(2)
	algName := algV.String()
	namedCurve := ""
	if obj, ok := algV.(*goja.Object); ok {
		if n := obj.Get("name"); n != nil {
			algName = n.String()
		}
		if c := obj.Get("namedCurve"); c != nil && !goja.IsUndefined(c) {
			namedCurve = c.String()
		}
	}
	usages := usagesFromArg(vm, call.Argument(4))
	switch strings.ToUpper(algName) {
	case "HKDF":
		return s.newKeyObj(&goKey{kind: keyHKDF, raw: data})
	case "AES-GCM", "AES-CBC", "AES-KW":
		// Browsers reject AES key data that is not 128/192/256 bits.
		if l := len(data); l != 16 && l != 24 && l != 32 {
			domError(vm, "OperationError", "length is not 128, 192, or 256 bits")
		}
		return s.newKeyObj(&goKey{
			kind: keyAES, raw: data, algName: strings.ToUpper(algName),
			usages: usages, extractable: call.Argument(3).ToBoolean(),
		})
	case "RSA-OAEP", "RSASSA-PKCS1-V1_5":
		pubAny, err := x509.ParsePKIXPublicKey(data)
		if err != nil {
			s.vmDbg(fmt.Sprintf("importKey spki fail: argtype=%T exporttype=%T len=%d head=%x",
				call.Argument(1), call.Argument(1).Export(), len(data), headBytes(data, 24)))
			subtleError(vm, "importKey: bad spki: "+err.Error())
		}
		pub, ok := pubAny.(*rsa.PublicKey)
		if !ok {
			subtleError(vm, "importKey: spki is not RSA")
		}
		return s.newKeyObj(&goKey{
			kind: keyRSAPublic, pub: pub, algName: strings.ToUpper(algName),
			usages: usages, extractable: true,
		})
	case "ECDH", "ECDSA":
		curve, ok := ecCurve(namedCurve)
		if !ok {
			domError(vm, "NotSupportedError", "Failed to execute 'importKey' on 'SubtleCrypto': unsupported named curve "+namedCurve)
		}
		var ecPub *ecdh.PublicKey
		switch strings.ToLower(format) {
		case "raw":
			// uncompressed point 0x04||X||Y
			p, err := curve.NewPublicKey(data)
			if err != nil {
				subtleError(vm, "importKey: bad EC raw point: "+err.Error())
			}
			ecPub = p
		case "spki":
			pubAny, err := x509.ParsePKIXPublicKey(data)
			if err != nil {
				subtleError(vm, "importKey: bad EC spki: "+err.Error())
			}
			ecPub, err = ecToECDH(pubAny)
			if err != nil {
				subtleError(vm, "importKey: spki is not EC: "+err.Error())
			}
		default:
			subtleError(vm, "importKey: unsupported EC format "+format)
		}
		return s.newKeyObj(&goKey{
			kind: keyECPublic, ecPub: ecPub, algName: strings.ToUpper(algName),
			namedCurve: namedCurve, usages: usages, extractable: true,
		})
	default:
		subtleError(vm, "importKey: unsupported algorithm "+algName+" format "+format)
	}
	return goja.Undefined()
}

func (s *subtleShim) syncExportKey(call goja.FunctionCall) goja.Value {
	vm := s.vm
	s.traceOps("exportKey", []goja.Value{call.Argument(0), call.Argument(1)})
	format := strings.ToLower(call.Argument(0).String())
	k := s.keyFromVal("exportKey", 2, call.Argument(1))
	switch k.kind {
	case keyECPublic:
		// ECDH/ECDSA public keys are always exportable; servers receive
		// them as raw uncompressed points or SPKI.
		switch format {
		case "raw":
			return vm.ToValue(vm.NewArrayBuffer(append([]byte(nil), k.ecPub.Bytes()...)))
		case "spki":
			spki, err := ecdhToSPKI(k.ecPub)
			if err != nil {
				subtleError(vm, "exportKey: "+err.Error())
			}
			return vm.ToValue(vm.NewArrayBuffer(spki))
		default:
			subtleError(vm, "exportKey: unsupported format "+format)
		}
	case keyAES, keyHKDF:
		if !k.extractable {
			domError(vm, "InvalidAccessError", "key is not extractable")
		}
		s.vmDbg(fmt.Sprintf("EXPORTKEY %x", k.raw))
		return vm.ToValue(vm.NewArrayBuffer(append([]byte(nil), k.raw...)))
	default:
		domError(vm, "InvalidAccessError", "key is not extractable")
	}
	return goja.Undefined()
}

func (s *subtleShim) syncGenerateKey(call goja.FunctionCall) goja.Value {
	vm := s.vm
	s.traceOps("generateKey", []goja.Value{call.Argument(0), call.Argument(1), call.Argument(2)})
	algObj, ok := call.Argument(0).(*goja.Object)
	if !ok {
		subtleError(vm, "generateKey: algorithm must be an object")
	}
	algName := strings.ToUpper(algObj.Get("name").String())
	extractable := call.Argument(1).ToBoolean()
	usages := usagesFromArg(vm, call.Argument(2))

	switch algName {
	case "ECDH", "ECDSA":
		namedCurve := algObj.Get("namedCurve").String()
		curve, ok := ecCurve(namedCurve)
		if !ok {
			domError(vm, "NotSupportedError", "Failed to execute 'generateKey' on 'SubtleCrypto': unsupported named curve "+namedCurve)
		}
		for _, u := range usages {
			if algName == "ECDH" && u != "deriveKey" && u != "deriveBits" {
				domError(vm, "SyntaxError", "Cannot create a key using the requested usages")
			}
		}
		priv, err := curve.GenerateKey(rand.Reader)
		if err != nil {
			subtleError(vm, "generateKey: "+err.Error())
		}
		pubObj := s.newKeyObj(&goKey{
			kind: keyECPublic, ecPub: priv.PublicKey(), algName: algName,
			namedCurve: namedCurve, usages: usages, extractable: true,
		})
		privObj := s.newKeyObj(&goKey{
			kind: keyECPrivate, ecPriv: priv, algName: algName,
			namedCurve: namedCurve, usages: usages, extractable: extractable,
		})
		pair := vm.NewObject()
		_ = pair.Set("publicKey", pubObj)
		_ = pair.Set("privateKey", privObj)
		return pair
	case "AES-GCM", "AES-CBC", "AES-KW":
		length := 256
		if l := algObj.Get("length"); l != nil && !goja.IsUndefined(l) {
			length = int(l.ToInteger())
		}
		if length != 128 && length != 192 && length != 256 {
			domError(vm, "OperationError", "length is not 128, 192, or 256 bits")
		}
		for _, u := range usages {
			if u != "encrypt" && u != "decrypt" && u != "wrapKey" && u != "unwrapKey" {
				domError(vm, "SyntaxError", "Cannot create a key using the requested usages")
			}
		}
		if len(usages) == 0 {
			domError(vm, "SyntaxError", "Usages cannot be empty when creating a key")
		}
		raw := make([]byte, length/8)
		if _, err := rand.Read(raw); err != nil {
			subtleError(vm, "generateKey: "+err.Error())
		}
		return s.newKeyObj(&goKey{
			kind: keyAES, raw: raw, algName: algName,
			usages: usages, extractable: extractable,
		})
	default:
		domError(vm, "NotSupportedError", "Failed to execute 'generateKey' on 'SubtleCrypto': algorithm is not supported")
	}
	return goja.Undefined()
}

func (s *subtleShim) syncDigest(call goja.FunctionCall) goja.Value {
	vm := s.vm
	data, err := toBytes(vm, call.Argument(1))
	if err != nil {
		subtleError(vm, "digest: "+err.Error())
	}
	// Real WebCrypto honors the algorithm name; silently returning a
	// different-length digest would only surface as an opaque
	// downstream AES failure.
	var h func() hash.Hash
	switch strings.ToUpper(call.Argument(0).String()) {
	case "SHA-1":
		h = sha1.New
	case "SHA-384":
		h = sha512.New384
	case "SHA-512":
		h = sha512.New
	default: // SHA-256
		h = sha256.New
	}
	dig := h()
	dig.Write(data)
	return vm.ToValue(vm.NewArrayBuffer(dig.Sum(nil)))
}

func (s *subtleShim) syncCrypt(call goja.FunctionCall, encrypt bool) goja.Value {
	vm := s.vm
	out, err := s.crypt(call, encrypt)
	if err != nil {
		subtleError(vm, "crypt: "+err.Error())
	}
	return vm.ToValue(vm.NewArrayBuffer(out))
}

func (s *subtleShim) syncDeriveKey(call goja.FunctionCall) goja.Value {
	vm := s.vm
	s.traceOps("deriveKey", []goja.Value{call.Argument(0), call.Argument(1), call.Argument(2), call.Argument(3)})
	algObj, ok := call.Argument(0).(*goja.Object)
	if !ok {
		subtleError(vm, "deriveKey: algorithm must be an object")
	}
	algName := strings.ToUpper(algObj.Get("name").String())
	var secret []byte
	switch algName {
	case "ECDH":
		secret = s.ecdhBits(algObj, call.Argument(1))
	default: // HKDF
		salt, err := toBytes(vm, algObj.Get("salt"))
		if err != nil {
			subtleError(vm, "deriveKey: salt "+err.Error())
		}
		info, err := toBytes(vm, algObj.Get("info"))
		if err != nil {
			subtleError(vm, "deriveKey: info "+err.Error())
		}
		base := s.keyFromVal("deriveKey", 2, call.Argument(1))
		if base.kind != keyHKDF {
			domError(vm, "InvalidAccessError", "key is not an HKDF base key")
		}
		length := 256
		if derived, ok := call.Argument(2).(*goja.Object); ok {
			if l := derived.Get("length"); l != nil && !goja.IsUndefined(l) {
				length = int(l.ToInteger())
			}
		}
		var derr error
		secret, derr = hkdf.Key(sha256.New, base.raw, salt, string(info), length/8)
		if derr != nil {
			subtleError(vm, "deriveKey: "+derr.Error())
		}
	}
	// The derived key takes its algorithm/usages from the derivedKeyInfo
	// argument (importKey semantics on the shared secret).
	derived, ok := call.Argument(2).(*goja.Object)
	if !ok {
		subtleError(vm, "deriveKey: derived algorithm must be an object")
	}
	derivedName := strings.ToUpper(derived.Get("name").String())
	extractable := call.Argument(3).ToBoolean()
	usages := usagesFromArg(vm, call.Argument(4))
	switch derivedName {
	case "AES-GCM", "AES-CBC", "AES-KW":
		return s.newKeyObj(&goKey{
			kind: keyAES, raw: secret, algName: derivedName,
			usages: usages, extractable: extractable,
		})
	default:
		subtleError(vm, "deriveKey: unsupported derived algorithm "+derivedName)
	}
	return goja.Undefined()
}

func (s *subtleShim) syncDeriveBits(call goja.FunctionCall) goja.Value {
	vm := s.vm
	s.traceOps("deriveBits", []goja.Value{call.Argument(0), call.Argument(1), call.Argument(2)})
	algObj, ok := call.Argument(0).(*goja.Object)
	if !ok {
		subtleError(vm, "deriveBits: algorithm must be an object")
	}
	algName := strings.ToUpper(algObj.Get("name").String())
	length := 256
	if l := call.Argument(2); l != nil && !goja.IsUndefined(l) {
		length = int(l.ToInteger())
	}
	switch algName {
	case "ECDH":
		secret := s.ecdhBits(algObj, call.Argument(1))
		return vm.ToValue(vm.NewArrayBuffer(secret[:length/8]))
	default: // HKDF
		salt, _ := toBytes(vm, algObj.Get("salt"))
		info, _ := toBytes(vm, algObj.Get("info"))
		base := s.keyFromVal("deriveBits", 2, call.Argument(1))
		if base.kind != keyHKDF {
			domError(vm, "InvalidAccessError", "key is not an HKDF base key")
		}
		key, err := hkdf.Key(sha256.New, base.raw, salt, string(info), length/8)
		if err != nil {
			subtleError(vm, "deriveBits: "+err.Error())
		}
		return vm.ToValue(vm.NewArrayBuffer(key))
	}
}

// ecdhBits computes the ECDH shared secret: alg must carry the peer public
// key and base must be the private key.
func (s *subtleShim) ecdhBits(algObj *goja.Object, baseV goja.Value) []byte {
	peer := s.keyFromVal("deriveBits", 2, algObj.Get("public"))
	if peer.kind != keyECPublic {
		domError(s.vm, "InvalidAccessError", "ECDH peer is not an EC public key")
	}
	base := s.keyFromVal("deriveBits", 2, baseV)
	if base.kind != keyECPrivate {
		domError(s.vm, "InvalidAccessError", "ECDH base is not an EC private key")
	}
	secret, err := base.ecPriv.ECDH(peer.ecPub)
	if err != nil {
		subtleError(s.vm, "deriveBits: ECDH failed: "+err.Error())
	}
	return secret
}

// ecToECDH converts an x509-parsed public key (ECDSA form) into an ecdh
// public key on the same curve.
func ecToECDH(pubAny any) (*ecdh.PublicKey, error) {
	ec, ok := pubAny.(*ecdsa.PublicKey)
	if !ok {
		return nil, errors.New("not an EC public key")
	}
	var curve ecdh.Curve
	switch ec.Curve {
	case elliptic.P256():
		curve = ecdh.P256()
	case elliptic.P384():
		curve = ecdh.P384()
	case elliptic.P521():
		curve = ecdh.P521()
	default:
		return nil, errors.New("unsupported curve")
	}
	return curve.NewPublicKey(elliptic.Marshal(ec.Curve, ec.X, ec.Y))
}

// ecdhToSPKI serializes an ecdh public key as SubjectPublicKeyInfo (DER).
func ecdhToSPKI(pub *ecdh.PublicKey) ([]byte, error) {
	var curve elliptic.Curve
	switch pub.Curve() {
	case ecdh.P256():
		curve = elliptic.P256()
	case ecdh.P384():
		curve = elliptic.P384()
	case ecdh.P521():
		curve = elliptic.P521()
	default:
		return nil, errors.New("unsupported curve")
	}
	raw := pub.Bytes() // 0x04||X||Y
	x, y := elliptic.Unmarshal(curve, raw)
	if x == nil {
		return nil, errors.New("bad public point")
	}
	return x509.MarshalPKIXPublicKey(&ecdsa.PublicKey{Curve: curve, X: x, Y: y})
}

// crypt implements subtle.encrypt/decrypt for AES-GCM and RSA-OAEP.
func (s *subtleShim) crypt(call goja.FunctionCall, encrypt bool) ([]byte, error) {
	vm := s.vm
	op := "decrypt"
	if encrypt {
		op = "encrypt"
	}
	s.traceOps(op, []goja.Value{call.Argument(0), call.Argument(1), call.Argument(2)})
	algObj, ok := call.Argument(0).(*goja.Object)
	if !ok {
		return nil, errors.New("algorithm must be an object")
	}
	name := strings.ToUpper(algObj.Get("name").String())
	key := s.keyFromVal(op, 2, call.Argument(1))
	data, err := toBytes(vm, call.Argument(2))
	if err != nil {
		return nil, err
	}

	switch name {
	case "AES-GCM":
		iv, err := toBytes(vm, algObj.Get("iv"))
		if err != nil {
			return nil, err
		}
		aad, err := toBytes(vm, algObj.Get("additionalData"))
		if err != nil {
			return nil, err
		}
		block, err := aes.NewCipher(key.raw)
		if err != nil {
			return nil, err
		}
		if len(iv) == 0 {
			return nil, errors.New("AES-GCM requires a non-empty iv")
		}
		// WebCrypto accepts arbitrary IV lengths; Go's fixed 12-byte GCM
		// would panic on anything else, so size the GCM to the IV.
		gcm, err := cipher.NewGCMWithNonceSize(block, len(iv))
		if err != nil {
			return nil, err
		}
		if encrypt {
			return gcm.Seal(nil, iv, data, aad), nil
		}
		return gcm.Open(nil, iv, data, aad)
	case "RSA-OAEP":
		var label []byte
		if l := algObj.Get("label"); l != nil && !goja.IsUndefined(l) && !goja.IsNull(l) {
			label, err = toBytes(vm, l)
			if err != nil {
				return nil, err
			}
		}
		if encrypt {
			return rsa.EncryptOAEP(sha256.New(), rand.Reader, key.pub, data, label)
		}
		return nil, errors.New("RSA-OAEP decrypt unsupported")
	default:
		return nil, errors.New("unsupported algorithm " + name)
	}
}

func mustSet(err error) {
	if err != nil {
		panic(err)
	}
}
