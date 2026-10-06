package mediaresolver

// Muxed-audio measurement. VidsrcMe's transcoder mutes a single stereo AAC
// encode into every quality tier — the same ~125-131 kbps track plays at 360p
// and 1080p — and it does this regardless of the source release: both YIFY
// rips and proper NF WEB-DL DDP5.1 masters come out as the same crushed
// stereo mix. Release *names* (errLowQualityRelease) catch the YIFY case, and
// this file measures the actual muxed audio so a downmixed 5.1 source is
// caught too.

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// adtsSampleRates maps an ADTS sampling-frequency index to Hz.
var adtsSampleRates = []int{
	96000, 88200, 64000, 48000, 44100, 32000, 24000, 22050,
	16000, 12000, 11025, 8000, 7350,
}

// vidsrcmeMinMuxedAudioKbps is the floor below which a resolved vidsrcme
// master is flagged as low quality. Measured transcode values cluster at
// ~125-131 kbps (stereo, 44.1 kHz) whatever the source release, while
// providers carrying proper encodes measure at 180-256 kbps; 144 keeps a
// margin above the transcode ceiling without flagging a compact-but-honest
// stereo encode.
const vidsrcmeMinMuxedAudioKbps = 144

// demuxAudioPES extracts the audio elementary stream from an MPEG-TS buffer.
// The audio PID is identified through its PES stream id (0xC0 = audio); PES
// headers are stripped from packet starts and continuation packets of that
// PID are appended, yielding a contiguous ADTS stream.
func demuxAudioPES(buf []byte) []byte {
	audioPID := uint16(0xFFFF)
	var out []byte
	for off := 0; off+188 <= len(buf); off += 188 {
		if buf[off] != 0x47 {
			continue
		}
		pid := uint16(buf[off+1]&0x1F)<<8 | uint16(buf[off+2])
		pusi := buf[off+1]&0x40 != 0
		afc := (buf[off+3] >> 4) & 0x3
		p := off + 4
		if afc&0x2 != 0 {
			// Adaptation field: skip the length byte and its stuffing.
			p += 1 + int(buf[off+4])
		}
		if afc&0x1 == 0 || p >= off+188 {
			continue
		}
		payload := buf[p : off+188]
		if pusi && len(payload) >= 9 && payload[0] == 0x00 &&
			payload[1] == 0x00 && payload[2] == 0x01 && payload[3] == 0xC0 {
			audioPID = pid
			hlen := 9 + int(payload[8])
			if hlen > len(payload) {
				continue
			}
			out = append(out, payload[hlen:]...)
			continue
		}
		if pid == audioPID {
			out = append(out, payload...)
		}
	}
	return out
}

// walkADTS follows a contiguous ADTS frame chain and reports its aggregate
// parameters. Chains shorter than three frames are rejected so stray 0xFF
// bytes inside earlier video payloads cannot fabricate a track.
func walkADTS(audio []byte) (frames int, bytes int, rate int, channels int) {
	for i := 0; i+7 < len(audio); i++ {
		if audio[i] != 0xFF || audio[i+1]&0xF6 != 0xF0 {
			continue
		}
		fi := int(audio[i+2]>>2) & 0x0F
		fl := int(audio[i+3]&0x3)<<11 | int(audio[i+4])<<3 | int(audio[i+5]>>5)
		if fl < 7 || i+fl > len(audio) || fi >= len(adtsSampleRates) {
			continue
		}
		ch := (int(audio[i+2]&0x01) << 2) | (int(audio[i+3]) >> 6)
		// Per-candidate counters: a rejected chain must not leak its frame
		// count into the next candidate (that could push a short chain over
		// the three-frame threshold and fabricate a track).
		chainFrames, chainBytes := 0, 0
		for pos := i; pos+7 <= len(audio); {
			if audio[pos] != 0xFF || audio[pos+1]&0xF6 != 0xF0 {
				break
			}
			l := int(audio[pos+3]&0x3)<<11 | int(audio[pos+4])<<3 | int(audio[pos+5]>>5)
			if l < 7 || pos+l > len(audio) {
				break
			}
			chainFrames++
			chainBytes += l
			pos += l
		}
		if chainFrames < 3 {
			continue
		}
		return chainFrames, chainBytes, adtsSampleRates[fi], ch
	}
	return 0, 0, 0, 0
}

// measureMuxedAudioKbps reports the bitrate of the audio track muxed into a
// resolved master playlist, in kbps. It reads the first media segment of the
// highest-bandwidth tier — a single segment is representative because the
// transcode embeds one constant stereo encode — and returns 0 when the audio
// cannot be measured (fMP4 sources, fetch failures), in which case callers
// must not flag the source: an unmeasurable stream stays playable.
func (r *Resolver) measureMuxedAudioKbps(ctx context.Context, client *http.Client, headers http.Header, masterText string, base *url.URL) float64 {
	variantURL := resolveMediaURL(base, lastPlaylistLine(forceHighestQuality(masterText)))
	if variantURL == "" {
		return 0
	}
	vu, err := url.Parse(variantURL)
	if err != nil || vu.Host == "" {
		return 0
	}
	media, ok := r.probeMediaGet(ctx, client, headers, variantURL, 1<<20)
	if !ok {
		return 0
	}
	vbase, _ := url.Parse(variantURL)
	seg := firstSegmentURI(string(media))
	if seg == "" {
		return 0
	}
	segURL := resolveMediaURL(vbase, seg)
	if segURL == "" {
		return 0
	}
	body, ok := r.probeMediaGet(ctx, client, headers, segURL, 16<<20)
	if !ok {
		return 0
	}
	audio := demuxAudioPES(body)
	frames, abytes, rate, _ := walkADTS(audio)
	if frames == 0 || rate == 0 {
		return 0
	}
	secs := float64(frames) * 1024 / float64(rate)
	return float64(abytes) * 8 / secs / 1000
}

// probeMediaGet fetches a probe URL with the session's playback headers,
// returning the body and ok=false on any failure.
func (r *Resolver) probeMediaGet(ctx context.Context, client *http.Client, headers http.Header, raw string, limit int64) ([]byte, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, false
	}
	for _, k := range playbackHeaders {
		if v := headers.Get(k); v != "" {
			req.Header.Set(k, v)
		}
	}
	resp, err := r.resolveFetch(ctx, client, req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, false
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return nil, false
	}
	return data, true
}

// firstSegmentURI returns the first non-directive line of a media playlist
// (its first segment URI, relative or absolute).
func firstSegmentURI(mediaPlaylist string) string {
	for _, line := range strings.Split(mediaPlaylist, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		return line
	}
	return ""
}
