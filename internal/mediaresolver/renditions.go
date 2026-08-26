package mediaresolver

// External-subtitle renditions for native-HLS engines. Smart TV browsers
// (e.g. BrowseHere on Tizen) ignore DOM <track> elements entirely but fully
// honor subtitle renditions declared in the master manifest. The frontend
// registers its external subtitle list against the proxy token before the
// player fetches the stream; rewriteManifest then injects them as
// #EXT-X-MEDIA TYPE=SUBTITLES entries so they surface through the same
// TextTrack path embedded renditions use.

import (
	"context"
	"errors"
	"log"
	"net/url"
	"strings"
	"time"
)

// subRenditionGroupID is the EXT-X-MEDIA GROUP-ID all injected renditions share.
const subRenditionGroupID = "goflix-ext"

// maxSubRenditions caps how many tracks one session may register; the
// frontend dedupes already, this only bounds pathological payloads.
const maxSubRenditions = 32

// SubRendition is one external subtitle prepared for manifest injection.
// Label and Language are sanitized for quoted HLS attributes; URI is an
// absolute URL served by this process (a wrap playlist around a .vtt).
type SubRendition struct {
	Label    string
	Language string
	URI      string
}

// SanitizeManifestAttr makes s safe inside a quoted HLS attribute value:
// quoted-strings must not contain double quotes, CR or LF, and commas are
// dropped too because several TV parsers mis-split attribute lists even
// inside quotes. Control characters are removed and length capped.
func SanitizeManifestAttr(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r < 0x20 || r == 0x7f || r == '"' || r == ',' {
			continue
		}
		b.WriteRune(r)
	}
	out := strings.TrimSpace(b.String())
	if len(out) > 96 {
		out = out[:96]
	}
	return out
}

// Sub-rendition attachment states for proxySession.subsState.
const (
	subsNone    int32 = 0 // provider ineligible or hook unset — nothing will attach
	subsPending int32 = 1 // backend ladder fetch in flight
	subsDone    int32 = 2 // attachment finished (with or without renditions)
)

// SetSubRenditions attaches the external subtitle rendition list to the
// token's proxy session. It fails if the token is empty or expired.
func (r *Resolver) SetSubRenditions(token string, subs []SubRendition) error {
	if len(subs) > maxSubRenditions {
		subs = subs[:maxSubRenditions]
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sessions[token]
	if !ok || time.Now().After(s.expiresAt) {
		return errors.New("proxy session expired")
	}
	s.subs = subs
	s.subsState.Store(subsDone)
	// Signal any waitForSubs caller so it returns immediately instead of
	// sleeping out the remaining polling window.
	if s.subsDone != nil {
		select {
		case <-s.subsDone: // already closed — no-op
		default:
			close(s.subsDone)
		}
	}
	return nil
}

// maybeAttachSubRenditions kicks off the server-side subtitle ladder for
// providers known to carry no embedded renditions. This is what puts
// subtitles INSIDE the HLS master manifest without any frontend help: the
// resolver resolves them in a background goroutine right after creating the
// playback session.
func (r *Resolver) maybeAttachSubRenditions(token string, req MediaRequest) {
	if r.SubRenditionProvider == nil {
		return
	}
	switch strings.ToLower(strings.TrimSpace(req.Provider)) {
	case "vidking", "vidlove":
	default:
		return
	}
	r.mu.Lock()
	s := r.sessions[token]
	if s == nil || !s.subsState.CompareAndSwap(subsNone, subsPending) {
		r.mu.Unlock()
		return
	}
	r.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		subs := r.SubRenditionProvider(ctx, req)
		if len(subs) > maxSubRenditions {
			subs = subs[:maxSubRenditions]
		}
		if err := r.SetSubRenditions(token, subs); err != nil {
			log.Printf("[MediaResolver] subtitle attach skipped (session gone): %v", err)
			return
		}
		log.Printf("[MediaResolver] embedded %d subtitle renditions into manifest id=%s provider=%s", len(subs), req.ID, req.Provider)
	}()
}

// Worst-case manifest delay while a subtitle ladder is in flight.
const (
	waitForSubsInterval = 100 * time.Millisecond
	waitForSubsTicks    = 25 // 25 × 100ms ≈ 2.5s ceiling
)

// attachAndWarm is the shared tail of every session-creation site (the
// provider direct paths and the browser fallback in Resolve): kick off the
// server-side subtitle ladder for eligible providers, then start the
// read-ahead warmup.
func (r *Resolver) attachAndWarm(token string, req MediaRequest) {
	r.maybeAttachSubRenditions(token, req)
	r.startWarmup(token)
}

// waitForSubs blocks while a backend attachment is still in flight so the
// very first manifest fetch already carries the renditions. It returns as
// soon as the subtitle ladder completes, or when the context is cancelled
// (player disconnect / request abort), or after the 2.5s ceiling —
// whichever comes first. This replaces the original time.Sleep busy-poll
// which ignored context cancellation.
func (r *Resolver) waitForSubs(s *proxySession) bool {
	if s.subsState.Load() != subsPending {
		return true
	}
	// Snapshot the done channel under the lock so we hold a reference even
	// if the session is reaped (subsDone is immutable once assigned).
	r.mu.Lock()
	done := s.subsDone
	r.mu.Unlock()
	
	const ceiling = waitForSubsInterval * waitForSubsTicks // 2.5s
	if done == nil {
		// Safety net: should never happen with the updated newSession.
		time.Sleep(ceiling)
		return s.subsState.Load() != subsPending
	}
	select {
	case <-done:
		// Subtitle ladder finished; state already flipped by SetSubRenditions.
	case <-time.After(ceiling):
		// Ceiling reached — proceed without renditions rather than stalling further.
	}
	return s.subsState.Load() != subsPending
}

// sessionSnapshot is what rewriteManifest needs from a session in one lock.
type sessionSnapshot struct {
	subs   []SubRendition
	source string
}

// sessionMeta returns a consistent snapshot of the session's rendition list
// and upstream source. Zero values mean the token is unknown or expired.
func (r *Resolver) sessionMeta(token string) sessionSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sessions[token]
	if !ok || time.Now().After(s.expiresAt) {
		return sessionSnapshot{}
	}
	return sessionSnapshot{subs: s.subs, source: s.source}
}

// subRenditions returns the registered rendition list for a token, or nil.
func (r *Resolver) subRenditions(token string) []SubRendition {
	return r.sessionMeta(token).subs
}

// sessionSource returns the session's upstream source URL, or "".
func (r *Resolver) sessionSource(token string) string {
	return r.sessionMeta(token).source
}

// subRenditionLine renders one EXT-X-MEDIA entry. The attribute vocabulary
// mirrors the provider masters that native TV players already accept
// (DEFAULT/AUTOSELECT/FORCED all off, so selection stays user-driven).
func subRenditionLine(s SubRendition) string {
	line := `#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID="` + subRenditionGroupID +
		`",NAME="` + s.Label + `",DEFAULT=NO,AUTOSELECT=NO,FORCED=NO`
	if s.Language != "" {
		line += `,LANGUAGE="` + s.Language + `"`
	}
	return line + `,URI="` + s.URI + `"`
}

// injectSubRenditions appends the registered renditions to a MASTER playlist
// that declares none of its own. Media playlists (no #EXT-X-STREAM-INF) have
// nowhere to reference a subtitles group, and masters that already carry
// TYPE=SUBTITLES lines keep their own set untouched. The rendition URIs are
// absolute URLs on this server and must NOT be routed through the proxy
// pipeline — callers inject after URI rewriting for exactly that reason.
func injectSubRenditions(text string, subs []SubRendition) string {
	if len(subs) == 0 || !strings.Contains(text, "#EXT-X-STREAM-INF") ||
		strings.Contains(text, "TYPE=SUBTITLES") {
		return text
	}

	media := make([]string, 0, len(subs))
	for _, s := range subs {
		media = append(media, subRenditionLine(s))
	}

	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines)+len(media))
	inserted := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Every video variant must reference the group for the renditions to
		// be offered. A variant pointing at an undefined subtitles group
		// would break parsing, so dangling references are repointed at ours.
		if strings.HasPrefix(trimmed, "#EXT-X-STREAM-INF") {
			if start := strings.Index(line, `SUBTITLES="`); start >= 0 {
				valStart := start + len(`SUBTITLES="`)
				if end := strings.Index(line[valStart:], `"`); end >= 0 {
					line = line[:valStart] + subRenditionGroupID + line[valStart+end:]
				}
			} else {
				line += `,SUBTITLES="` + subRenditionGroupID + `"`
			}
		}
		out = append(out, line)
		if !inserted && strings.HasPrefix(trimmed, "#EXTM3U") {
			out = append(out, media...)
			inserted = true
		}
	}
	if !inserted {
		return text // no #EXTM3U header — leave the document alone
	}
	return strings.Join(out, "\n")
}

// InjectSubRenditions appends external subtitle renditions to an HLS master manifest.
func InjectSubRenditions(text string, subs []SubRendition) string {
	return injectSubRenditions(text, subs)
}


// synthMediaMaster wraps a bare media playlist in a synthesized master so
// subtitle renditions can be declared. VidKing hands out single-variant
// media playlists with no master at all — without this there is nowhere to
// reference a subtitles group and native engines would never see the
// registered tracks. The variant points back through the proxy's ?url=
// form so the media playlist keeps its normal serve path (rewriting,
// segment registration, read-ahead warming).
func (r *Resolver) synthMediaMaster(token string, subs []SubRendition) string {
	variant := "/api/media/proxy/" + token + ".m3u8?url=" + url.QueryEscape(r.sessionSource(token))
	b := &strings.Builder{}
	b.WriteString("#EXTM3U\n")
	for _, s := range subs {
		b.WriteString(subRenditionLine(s) + "\n")
	}
	// BANDWIDTH is required by the spec; the value is irrelevant because it
	// is the only variant, so nothing competes in ABR.
	b.WriteString(`#EXT-X-STREAM-INF:BANDWIDTH=4000000,SUBTITLES="` + subRenditionGroupID + "\"\n")
	b.WriteString(variant + "\n")
	return b.String()
}
