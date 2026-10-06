package cinesrcjs

// Regression test for the location shim: the challenge module binds the
// decrypted response to the season/episode in location.search, so the runtime
// must report the full page URL (and split pathname/query) exactly like a
// browser would. A query-less location made every TV episode but the default
// S1E1 fail dr() with resp_media_mismatch.

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"testing"
)

func TestRuntimeLocationMirrorsPageURL(t *testing.T) {
	cases := []struct {
		pagePath     string
		wantPathname string
		wantSearch   string
	}{
		{"/embed/tv/105248?s=1&e=4", "/embed/tv/105248", "?s=1&e=4"},
		{"/embed/tv/105248", "/embed/tv/105248", ""},
		{"/embed/movie/550", "/embed/movie/550", ""},
	}
	for _, tc := range cases {
		jar, err := cookiejar.New(nil)
		if err != nil {
			t.Fatalf("cookiejar: %v", err)
		}
		rt, err := newRuntime(context.Background(), "https://cinesrc.st", tc.pagePath,
			"test-agent", DefaultFingerprint(), jar, http.DefaultTransport, nil)
		if err != nil {
			t.Fatalf("newRuntime(%q): %v", tc.pagePath, err)
		}
		v, err := rt.vm.RunString(`location.pathname + "|" + location.search + "|" + location.href`)
		if err != nil {
			t.Fatalf("RunString: %v", err)
		}
		want := tc.wantPathname + "|" + tc.wantSearch + "|https://cinesrc.st" + tc.pagePath
		if got := v.String(); got != want {
			t.Errorf("pagePath %q: location = %q, want %q", tc.pagePath, got, want)
		}
	}
}
