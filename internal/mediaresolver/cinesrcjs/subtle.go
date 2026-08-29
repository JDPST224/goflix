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
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
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
)

type goKey struct {
	kind keyKind
	raw  []byte // AES / HKDF input material
	pub  *rsa.PublicKey
}

type subtleShim struct {
	vm    *goja.Runtime
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
	switch k.kind {
	case keyAES:
		_ = o.Set("type", "secret")
		alg := s.vm.NewObject()
		_ = alg.Set("name", "AES-GCM")
		_ = alg.Set("length", len(k.raw)*8)
		_ = o.Set("algorithm", alg)
		_ = o.Set("extractable", true)
		_ = o.Set("usages", []string{"encrypt", "decrypt"})
	case keyHKDF:
		_ = o.Set("type", "secret")
		alg := s.vm.NewObject()
		_ = alg.Set("name", "HKDF")
		_ = o.Set("hash", "SHA-256")
		_ = o.Set("length", len(k.raw)*8)
		_ = o.Set("algorithm", alg)
		_ = o.Set("extractable", false)
		_ = o.Set("usages", []string{"deriveKey", "deriveBits"})
	case keyRSAPublic:
		_ = o.Set("type", "public")
		alg := s.vm.NewObject()
		_ = alg.Set("name", "RSA-OAEP")
		_ = alg.Set("hash", "SHA-256")
		_ = alg.Set("modulusLength", k.pub.N.BitLen())
		exp := s.vm.NewObject()
		_ = exp.Set("0", 1)
		_ = exp.Set("1", 0)
		_ = exp.Set("2", 1)
		_ = alg.Set("publicExponent", exp)
		_ = o.Set("algorithm", alg)
		_ = o.Set("extractable", false)
		_ = o.Set("usages", []string{"encrypt"})
	}
	return o
}

func (s *subtleShim) keyFromVal(v goja.Value) (*goKey, error) {
	obj, ok := v.(*goja.Object)
	if !ok {
		return nil, errors.New("subtle: key is not an object")
	}
	idV := obj.Get("__goKeyID")
	if idV == nil {
		return nil, errors.New("subtle: not a shim key")
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
		return nil, errors.New("subtle: bad key id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	k, ok := s.keys[id]
	if !ok {
		return nil, errors.New("subtle: unknown key")
	}
	return k, nil
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

func headBytes(b []byte, n int) []byte {
	if len(b) > n {
		return b[:n]
	}
	return b
}

func (s *subtleShim) vmDbg(msg string) {
	if f := os.Getenv("CINESRCJS_DEBUG_LOG"); f != "" {
		fh, err := os.OpenFile(f, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err == nil {
			fmt.Fprintln(fh, msg)
			fh.Close()
		}
	}
	if v := s.vm.GlobalObject().Get("__consoleFn"); v != nil && !goja.IsUndefined(v) {
		if fn, ok := goja.AssertFunction(v); ok {
			_, _ = fn(goja.Undefined(), s.vm.ToValue(msg))
		}
	}
}

func (s *subtleShim) install(target *goja.Object) {
	mustSet(target.Set("importKey", s.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		vm := s.vm
		format := call.Argument(0).String()
		data, err := toBytes(vm, call.Argument(1))
		if err != nil {
			subtleError(vm, "importKey: "+err.Error())
		}
		algV := call.Argument(2)
		algName := algV.String()
		if obj, ok := algV.(*goja.Object); ok {
			if n := obj.Get("name"); n != nil {
				algName = n.String()
			}
		}
		switch strings.ToUpper(algName) {
		case "HKDF":
			return s.newKeyObj(&goKey{kind: keyHKDF, raw: data})
		case "AES-GCM", "AES-CBC", "AES-KW":
			return s.newKeyObj(&goKey{kind: keyAES, raw: data})
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
			return s.newKeyObj(&goKey{kind: keyRSAPublic, pub: pub})
		default:
			subtleError(vm, "importKey: unsupported algorithm "+algName+" format "+format)
		}
		return goja.Undefined()
	})))

	mustSet(target.Set("exportKey", s.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		vm := s.vm
		k, err := s.keyFromVal(call.Argument(1))
		if err != nil {
			subtleError(vm, err.Error())
		}
		s.vmDbg(fmt.Sprintf("EXPORTKEY %x", k.raw))
		return vm.ToValue(vm.NewArrayBuffer(append([]byte(nil), k.raw...)))
	})))

	mustSet(target.Set("generateKey", s.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		vm := s.vm
		alg := call.Argument(0)
		length := 256
		if obj, ok := alg.(*goja.Object); ok {
			if l := obj.Get("length"); l != nil && !goja.IsUndefined(l) {
				length = int(l.ToInteger())
			}
		}
		raw := make([]byte, length/8)
		if _, err := rand.Read(raw); err != nil {
			subtleError(vm, "generateKey: "+err.Error())
		}
		return s.newKeyObj(&goKey{kind: keyAES, raw: raw})
	})))

	mustSet(target.Set("digest", s.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		vm := s.vm
		data, err := toBytes(vm, call.Argument(1))
		if err != nil {
			subtleError(vm, "digest: "+err.Error())
		}
		sum := sha256.Sum256(data)
		return vm.ToValue(vm.NewArrayBuffer(sum[:]))
	})))

	mustSet(target.Set("encrypt", s.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		vm := s.vm
		out, err := s.crypt(call, true)
		if err != nil {
			subtleError(vm, "encrypt: "+err.Error())
		}
		return vm.ToValue(vm.NewArrayBuffer(out))
	})))

	mustSet(target.Set("decrypt", s.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		vm := s.vm
		out, err := s.crypt(call, false)
		if err != nil {
			subtleError(vm, "decrypt: "+err.Error())
		}
		return vm.ToValue(vm.NewArrayBuffer(out))
	})))

	mustSet(target.Set("deriveKey", s.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		vm := s.vm
		algObj, ok := call.Argument(0).(*goja.Object)
		if !ok {
			subtleError(vm, "deriveKey: algorithm must be an object")
		}
		salt, err := toBytes(vm, algObj.Get("salt"))
		if err != nil {
			subtleError(vm, "deriveKey: salt "+err.Error())
		}
		info, err := toBytes(vm, algObj.Get("info"))
		if err != nil {
			subtleError(vm, "deriveKey: info "+err.Error())
		}
		base, err := s.keyFromVal(call.Argument(1))
		if err != nil {
			subtleError(vm, err.Error())
		}
		length := 256
		if derived, ok := call.Argument(2).(*goja.Object); ok {
			if l := derived.Get("length"); l != nil && !goja.IsUndefined(l) {
				length = int(l.ToInteger())
			}
		}
		key, err := hkdf.Key(sha256.New, base.raw, salt, string(info), length/8)
		if err != nil {
			subtleError(vm, "deriveKey: "+err.Error())
		}
		return s.newKeyObj(&goKey{kind: keyAES, raw: key})
	})))

	mustSet(target.Set("deriveBits", s.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		vm := s.vm
		algObj, _ := call.Argument(0).(*goja.Object)
		salt, _ := toBytes(vm, algObj.Get("salt"))
		info, _ := toBytes(vm, algObj.Get("info"))
		base, err := s.keyFromVal(call.Argument(1))
		if err != nil {
			subtleError(vm, err.Error())
		}
		length := 256
		if l := call.Argument(2); l != nil && !goja.IsUndefined(l) {
			length = int(l.ToInteger())
		}
		key, err := hkdf.Key(sha256.New, base.raw, salt, string(info), length/8)
		if err != nil {
			subtleError(vm, "deriveBits: "+err.Error())
		}
		return vm.ToValue(vm.NewArrayBuffer(key))
	})))
}

// crypt implements subtle.encrypt/decrypt for AES-GCM and RSA-OAEP.
func (s *subtleShim) crypt(call goja.FunctionCall, encrypt bool) ([]byte, error) {
	vm := s.vm
	algObj, ok := call.Argument(0).(*goja.Object)
	if !ok {
		return nil, errors.New("algorithm must be an object")
	}
	name := strings.ToUpper(algObj.Get("name").String())
	key, err := s.keyFromVal(call.Argument(1))
	if err != nil {
		return nil, err
	}
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
		gcm, err := cipher.NewGCM(block)
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

// unused type guards against accidental ecdsa/ed25519 imports removal
var _ = []interface{}{ecdsa.PublicKey{}, ed25519.PublicKey{}}
var _ = hex.EncodeToString
var _ = url.Values{}
var _ cookiejar.Options
var _ http.Transport

func mustSet(err error) {
	if err != nil {
		panic(err)
	}
}
