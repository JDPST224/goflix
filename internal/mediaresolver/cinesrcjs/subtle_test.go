package cinesrcjs

import (
	"bytes"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"testing"

	"github.com/dop251/goja"
)

// newTestVM builds a bare goja VM with the subtle shim installed as the
// global `subtle` — enough to exercise the crypto shims without the full
// challenge runtime.
func newTestVM(t *testing.T) (*goja.Runtime, *subtleShim) {
	t.Helper()
	vm := goja.New()
	s := newSubtleShim(vm)
	subtleObj := vm.NewObject()
	s.install(subtleObj)
	if err := vm.GlobalObject().Set("subtle", subtleObj); err != nil {
		t.Fatal(err)
	}
	return vm, s
}

// runAsync evaluates js (which must evaluate to a Promise), services the
// promise job queue by returning to Go, and reports the settled outcome.
func runAsync(t *testing.T, vm *goja.Runtime, js string) (res goja.Value, errStr string) {
	t.Helper()
	if _, err := vm.RunString(`(function(){
		__res = null; __err = null;
		(` + js + `).then(function(v){ __res = v; }, function(e){ __err = String(e && e.message || e); });
	})()`); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	res = vm.GlobalObject().Get("__res")
	if v := vm.GlobalObject().Get("__err"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		errStr = v.String()
	}
	return res, errStr
}

// TestSubtleAESGCMNonceSizes verifies AES-GCM against the IV lengths real
// WebCrypto accepts. cipher.NewGCM (the previous implementation) panics on
// any nonce that is not 12 bytes — a raw Go panic escaping the VM.
func TestSubtleAESGCMNonceSizes(t *testing.T) {
	for _, ivLen := range []int{12, 8, 16} {
		vm, _ := newTestVM(t)
		js := `(function(){
			var key = subtle.importKey("raw", new Uint8Array(32), {name: "AES-GCM"});
			var iv = new Uint8Array(` + itoa(ivLen) + `);
			for (var i = 0; i < iv.length; i++) iv[i] = 0xA0 + i;
			return key.then(function(key){
				return subtle.encrypt({name: "AES-GCM", iv: iv}, key, new Uint8Array([1,2,3,4,5,6,7,8,9,10]))
					.then(function(ct){ return subtle.decrypt({name: "AES-GCM", iv: iv}, key, ct); });
			});
		})().then(function(pt){
			var out = [];
			var u8 = new Uint8Array(pt);
			for (var i = 0; i < u8.length; i++) out.push(u8[i]);
			return out.join(",");
		})`
		res, errStr := runAsync(t, vm, js)
		if errStr != "" {
			t.Fatalf("iv %d: %s", ivLen, errStr)
		}
		if got := res.String(); got != "1,2,3,4,5,6,7,8,9,10" {
			t.Fatalf("iv %d: round-trip mismatch: got %q", ivLen, got)
		}
	}
}

// TestSubtleAESGCMZeroIV checks that a zero-length IV rejects (as WebCrypto
// does) instead of panicking in the GCM implementation.
func TestSubtleAESGCMZeroIV(t *testing.T) {
	vm, _ := newTestVM(t)
	js := `(function(){
		var key = subtle.importKey("raw", new Uint8Array(32), {name: "AES-GCM"});
		return key.then(function(key){
			return subtle.encrypt({name: "AES-GCM", iv: new Uint8Array(0)}, key, new Uint8Array([1]));
		});
	})()`
	_, errStr := runAsync(t, vm, js)
	if errStr == "" {
		t.Fatal("expected encrypt with empty iv to reject")
	}
}

// TestSubtleDigestAlgorithms pins that digest honors the requested hash —
// silently returning a SHA-256 digest for SHA-1/384/512 requests produces
// wrong-length keys downstream.
func TestSubtleDigestAlgorithms(t *testing.T) {
	for _, tc := range []struct {
		alg string
		fn  func([]byte) []byte
	}{
		{"SHA-1", func(b []byte) []byte { s := sha1.Sum(b); return s[:] }},
		{"SHA-256", func(b []byte) []byte { s := sha256.Sum256(b); return s[:] }},
		{"SHA-384", func(b []byte) []byte { s := sha512.Sum384(b); return s[:] }},
		{"SHA-512", func(b []byte) []byte { s := sha512.Sum512(b); return s[:] }},
	} {
		vm, _ := newTestVM(t)
		res, errStr := runAsync(t, vm, `subtle.digest("`+tc.alg+`", "abc")`)
		if errStr != "" {
			t.Fatalf("%s: %s", tc.alg, errStr)
		}
		ab, ok := res.Export().(interface{ Bytes() []byte })
		if !ok {
			t.Fatalf("%s: digest did not return an ArrayBuffer: %T", tc.alg, res.Export())
		}
		if want := tc.fn([]byte("abc")); !bytes.Equal(ab.Bytes(), want) {
			t.Errorf("%s: digest mismatch: got %x want %x", tc.alg, ab.Bytes(), want)
		}
	}
}

// TestSubtleDeriveBitsRejectsNonObject pins that a non-object algorithm
// argument rejects the promise (a raw Go panic there escapes RunString and
// kills the process).
func TestSubtleDeriveBitsRejectsNonObject(t *testing.T) {
	vm, _ := newTestVM(t)
	js := `(function(){
		var key = subtle.importKey("raw", new Uint8Array(32), {name: "HKDF"});
		return key.then(function(key){ return subtle.deriveBits(1, key, 128); });
	})()`
	_, errStr := runAsync(t, vm, js)
	if errStr == "" {
		t.Fatal("expected deriveBits with non-object algorithm to reject")
	}
}

// TestSubtleHKDFDeriveRoundTrip checks that deriveKey/deriveBits with valid
// arguments still works after the guards were added.
func TestSubtleHKDFDeriveRoundTrip(t *testing.T) {
	vm, _ := newTestVM(t)
	res, errStr := runAsync(t, vm, `(function(){
		var key = subtle.importKey("raw", new Uint8Array(32), {name: "HKDF"});
		return key.then(function(key){
			return subtle.deriveBits({name: "HKDF", salt: new Uint8Array(8), info: new Uint8Array(2)}, key, 256);
		});
	})().then(function(bits){ return new Uint8Array(bits).length; })`)
	if errStr != "" {
		t.Fatal(errStr)
	}
	if n := res.ToInteger(); n != 32 {
		t.Fatalf("deriveBits length = %d, want 32", n)
	}
}

// TestSubtleBogusKeyRejects pins that a bogus key argument rejects with the
// browser-shaped TypeError the challenge scripts validate.
func TestSubtleBogusKeyRejects(t *testing.T) {
	vm, _ := newTestVM(t)
	res, errStr := runAsync(t, vm, `subtle.exportKey("raw", undefined)`)
	_ = res
	want := "Failed to execute 'exportKey' on 'SubtleCrypto': parameter 2 is not of type 'CryptoKey'."
	if errStr != want {
		t.Fatalf("bogus key rejection = %q, want %q", errStr, want)
	}
}

// TestSubtleImportKeyBadAESLength pins the browser behavior the challenge's
// environment probes rely on: AES key data must be 128/192/256 bits.
func TestSubtleImportKeyBadAESLength(t *testing.T) {
	for _, n := range []int{0, 15, 23, 33, 64} {
		vm, _ := newTestVM(t)
		_, errStr := runAsync(t, vm, `subtle.importKey("raw", new Uint8Array(`+itoa(n)+`), {name: "AES-GCM"})`)
		if errStr == "" {
			t.Fatalf("%d-byte AES key data: expected rejection", n)
		}
	}
	for _, n := range []int{16, 24, 32} {
		vm, _ := newTestVM(t)
		_, errStr := runAsync(t, vm, `subtle.importKey("raw", new Uint8Array(`+itoa(n)+`), {name: "AES-GCM"})`)
		if errStr != "" {
			t.Fatalf("%d-byte AES key data: unexpected rejection %q", n, errStr)
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		return "-" + string(b)
	}
	return string(b)
}
