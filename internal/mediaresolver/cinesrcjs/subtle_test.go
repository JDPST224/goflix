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

// TestSubtleAESGCMNonceSizes verifies AES-GCM against the IV lengths real
// WebCrypto accepts. cipher.NewGCM (the previous implementation) panics on
// any nonce that is not 12 bytes — a raw Go panic escaping the VM.
func TestSubtleAESGCMNonceSizes(t *testing.T) {
	plain := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	for _, ivLen := range []int{12, 8, 16} {
		vm, _ := newTestVM(t)
		js := `(function(){
			var key = subtle.importKey("raw", new Uint8Array(32), {name: "AES-GCM"});
			var iv = new Uint8Array(` + itoa(ivLen) + `);
			for (var i = 0; i < iv.length; i++) iv[i] = 0xA0 + i;
			var ct = subtle.encrypt({name: "AES-GCM", iv: iv}, key, new Uint8Array([1,2,3,4,5,6,7,8,9,10]));
			var pt = subtle.decrypt({name: "AES-GCM", iv: iv}, key, ct);
			var out = [];
			var u8 = new Uint8Array(pt);
			for (var i = 0; i < u8.length; i++) out.push(u8[i]);
			return out.join(",");
		})()`
		v, err := vm.RunString(js)
		if err != nil {
			t.Fatalf("iv %d: %v", ivLen, err)
		}
		got := v.String()
		want := "1,2,3,4,5,6,7,8,9,10"
		if got != want {
			t.Fatalf("iv %d: round-trip mismatch: got %q want %q", ivLen, got, want)
		}
		_ = plain
	}
}

// TestSubtleAESGCMEmptyIV checks that a zero-length IV errors (as WebCrypto
// does) instead of panicking in the GCM implementation.
func TestSubtleAESGCMZeroIV(t *testing.T) {
	vm, _ := newTestVM(t)
	_, err := vm.RunString(`(function(){
		var key = subtle.importKey("raw", new Uint8Array(32), {name: "AES-GCM"});
		subtle.encrypt({name: "AES-GCM", iv: new Uint8Array(0)}, key, new Uint8Array([1]));
	})()`)
	if err == nil {
		t.Fatal("expected encrypt with empty iv to fail")
	}
}

// TestSubtleDigestAlgorithms pins that digest honors the requested hash —
// silently returning a SHA-256 digest for SHA-1/384/512 requests produces
// wrong-length keys downstream.
func TestSubtleDigestAlgorithms(t *testing.T) {
	vm, _ := newTestVM(t)
	for _, tc := range []struct {
		alg string
		fn  func([]byte) []byte
	}{
		{"SHA-1", func(b []byte) []byte { s := sha1.Sum(b); return s[:] }},
		{"SHA-256", func(b []byte) []byte { s := sha256.Sum256(b); return s[:] }},
		{"SHA-384", func(b []byte) []byte { s := sha512.Sum384(b); return s[:] }},
		{"SHA-512", func(b []byte) []byte { s := sha512.Sum512(b); return s[:] }},
	} {
		v, err := vm.RunString(`subtle.digest("` + tc.alg + `", "abc")`)
		if err != nil {
			t.Fatalf("%s: %v", tc.alg, err)
		}
		ab, ok := v.Export().(interface{ Bytes() []byte })
		if !ok {
			t.Fatalf("%s: digest did not return an ArrayBuffer: %T", tc.alg, v.Export())
		}
		if want := tc.fn([]byte("abc")); !bytes.Equal(ab.Bytes(), want) {
			t.Errorf("%s: digest mismatch: got %x want %x", tc.alg, ab.Bytes(), want)
		}
	}
}

// TestSubtleDeriveBitsRejectsNonObject pins the guard that keeps a
// non-object algorithm argument from nil-dereferencing inside the Go
// callback (a raw Go panic there escapes RunString and kills the process).
func TestSubtleDeriveBitsRejectsNonObject(t *testing.T) {
	vm, _ := newTestVM(t)
	_, err := vm.RunString(`(function(){
		var key = subtle.importKey("raw", new Uint8Array(32), {name: "HKDF"});
		return subtle.deriveBits(1, key, 128);
	})()`)
	if err == nil {
		t.Fatal("expected deriveBits with non-object algorithm to fail")
	}
}

// TestSubtleHKDFDeriveRoundTrip checks that deriveKey/deriveBits with valid
// arguments still works after the guard was added.
func TestSubtleHKDFDeriveRoundTrip(t *testing.T) {
	vm, _ := newTestVM(t)
	v, err := vm.RunString(`(function(){
		var key = subtle.importKey("raw", new Uint8Array(32), {name: "HKDF"});
		var bits = subtle.deriveBits({name: "HKDF", salt: new Uint8Array(8), info: new Uint8Array(2)}, key, 256);
		return new Uint8Array(bits).length;
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if n := v.ToInteger(); n != 32 {
		t.Fatalf("deriveBits length = %d, want 32", n)
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
