package mediaresolver

// One-off live probe: resolves a real title through every provider's direct
// chain and reports exactly what the dashboard resolution column would see
// (RESOLUTION attribute vs quality tokens vs tier label). Run with:
//
//	go test -v -run TestProviderHeightLive ./internal/mediaresolver -timeout 10m

import (
	"context"
	"testing"
	"time"
)

func TestProviderHeightLive(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live provider resolution probe in short mode")
	}
	r, err := New(Config{MaxBrowserSessions: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	req := MediaRequest{Type: Movie, ID: "27205"} // Inception

	type probe struct {
		name string
		run  func(context.Context) (*directResolution, error)
	}
	probes := []probe{
		{"vixsrc", func(ctx context.Context) (*directResolution, error) { return r.resolveVixsrcDirect(ctx, req) }},
		{"vidsrcme", func(ctx context.Context) (*directResolution, error) { return r.resolveVidsrcmeDirect(ctx, req) }},
		{"cinesrc", func(ctx context.Context) (*directResolution, error) { return r.resolveCinesrcDirect(ctx, req) }},
	}
	for _, p := range probes {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		res, err := p.run(ctx)
		cancel()
		if err != nil {
			t.Logf("[%s] resolve failed: %v", p.name, err)
			continue
		}
		t.Logf("[%s] source=%s", p.name, redactQuery(res.Source))
		t.Logf("[%s] master=%d bytes streamInf=%v", p.name, len(res.MasterText),
			include(res.MasterText, "#EXT-X-STREAM-INF"))
		t.Logf("[%s] height: tier=%q attr=%d tokens=%d -> %d",
			p.name, res.Tier, highestVariantHeight(res.MasterText),
			heightFromQualityToken(res.MasterText),
			pickHeight(res.Tier, res.MasterText))
	}
}

func include(hay, needle string) bool {
	return len(hay) > 0 && contains(hay, needle)
}

func contains(hay, needle string) bool {
	for i := 0; i+len(needle) <= len(hay); i++ {
		if hay[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// pickHeight mirrors noteStreamHeightToken's decision order.
func pickHeight(tier, masterText string) int {
	h := heightFromTier(tier)
	if h == 0 && masterText != "" {
		if h = highestVariantHeight(masterText); h == 0 {
			h = heightFromQualityToken(masterText)
		}
	}
	return h
}
