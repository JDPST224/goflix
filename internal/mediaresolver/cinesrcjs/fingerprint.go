package cinesrcjs

import (
	_ "embed"
	"encoding/json"
)

//go:embed fp_canvas.json
var canvasURLsJSON string

// Fingerprint carries the browser-plausibility values embedded in the
// challenge payloads. Servers reject bot-looking fingerprints (missing
// cookieEnabled, "0|0" WebGL, 1x1-pixel canvas hashes), so the values must
// look genuine; see docs/cinesrc-direct-protocol.md.
type Fingerprint struct {
	CanvasURLs    []string // real canvas renders; rotated per canvas element
	WebGLVendor   string
	WebGLRenderer string
	TZ            string
	Language      string
	Languages     string
	Platform      string
	ScreenW       int
	ScreenH       int
}

// DefaultFingerprint matches the values captured from a real headless-Chrome
// session that the cinesrc server accepted.
func DefaultFingerprint() Fingerprint {
	urls := []string{}
	_ = json.Unmarshal([]byte(canvasURLsJSON), &urls)
	return Fingerprint{
		CanvasURLs:  urls,
		WebGLVendor: "Google Inc. (Microsoft)",
		WebGLRenderer: "ANGLE (Microsoft, Microsoft Basic Render Driver (0x0000008C) " +
			"Direct3D11 vs_5_0 ps_5_0, D3D11)",
		TZ:        "Asia/Taipei",
		Language:  "en-US",
		Languages: "en-US,en",
		Platform:  "Win32",
		ScreenW:   800,
		ScreenH:   600,
	}
}
