package mediaresolver

// Muxed-audio measurement regression tests: the TS demuxer and ADTS walker
// that feed the vidsrcme quality gate must read a real MPEG-TS audio track.

import (
	"testing"
)

// adtsFrame builds an ADTS header prefix of headerLen bytes, with the given
// frame length encoded in the header per spec. Callers append the header to
// a payload of exactly fl bytes.
func adtsHeader(fl int) []byte {
	f := make([]byte, 7)
	f[0], f[1] = 0xFF, 0xF1 // sync, MPEG-4, no CRC
	f[2] = 0x50             // profile LC, freq index 4 (44100), channels high
	f[3] = 0x80             // channels 2 + length bits 0 (fl < 2048)
	f[4] = byte(fl >> 3)
	f[5] = byte((fl & 7) << 5)
	f[6] = 0xFC // frame alignment filler
	return f
}

// TestADTSHeaderRoundtrip pins the length encoding the walker parses.
func TestADTSHeaderRoundtrip(t *testing.T) {
	fl := 100
	f := adtsHeader(fl)
	got := int(f[3]&0x3)<<11 | int(f[4])<<3 | int(f[5])>>5
	if got != fl {
		t.Fatalf("ADTS header decodes %d, want %d", got, fl)
	}
}

func TestFirstSegmentURI(t *testing.T) {
	media := "#EXTM3U\n#EXT-X-VERSION:3\n#EXTINF:5.005,\n/content/page-0.html?token=a\n#EXTINF:5.005,\n/content/page-1.html\n"
	if got := firstSegmentURI(media); got != "/content/page-0.html?token=a" {
		t.Fatalf("firstSegmentURI = %q", got)
	}
	if got := firstSegmentURI("#EXTM3U\n#EXT-X-ENDLIST"); got != "" {
		t.Fatalf("empty playlist should yield no segment, got %q", got)
	}
}

func TestWalkADTS(t *testing.T) {
	// Three chained frames; each header is followed by fl-7 filler bytes.
	const fl = 60
	frame := adtsHeader(fl)
	frame = append(frame, make([]byte, fl-7)...)
	audio := make([]byte, 0, 3*fl)
	for i := 0; i < 3; i++ {
		audio = append(audio, frame...)
	}
	frames, abytes, rate, ch := walkADTS(audio)
	if frames != 3 || abytes != 3*fl || rate != 44100 || ch != 2 {
		t.Fatalf("walkADTS = %d/%d/%d/%d, want 3/%d/44100/2", frames, abytes, rate, ch, 3*fl)
	}
	// A lone frame must not count as a track (chain-of-three requirement).
	if frames, _, _, _ := walkADTS(audio[:fl]); frames != 0 {
		t.Fatalf("single frame reported as a track: %d", frames)
	}
}

// audioTSPacket builds one 188-byte TS packet for PID 257 carrying the whole
// payload (184 bytes max) with the payload-unit-start flag.
func audioTSPacket(payload []byte, pusi bool) []byte {
	p := make([]byte, 188)
	p[0] = 0x47
	p[1] = 0x01 // PID 257 high bits
	if pusi {
		p[1] |= 0x40
	}
	p[2] = 0x01
	p[3] = 0x10 // payload only
	copy(p[4:], payload)
	return p
}

func TestDemuxAudioPES(t *testing.T) {
	// Three chained ADTS frames carried in one audio PES on PID 257.
	// Budget: a single TS packet carries 184 payload bytes and the PES
	// header + flags + PTS take 14, so the ADTS stream must fit in 170 —
	// 3×50 does. (The old 60-byte frames truncated the third frame and the
	// expectation only held because walkADTS leaked counts across
	// candidates — the leak this package's regression test now pins.)
	const fl = 50
	var adts []byte
	for i := 0; i < 3; i++ {
		adts = append(adts, adtsHeader(fl)...)
		adts = append(adts, make([]byte, fl-7)...)
	}
	// PES: start code + stream id + length + flags + PTS (header_data_length=5).
	pts := []byte{0x21, 0x00, 0x01, 0x00, 0x01}
	hdrData := append([]byte{0x84, 0x80, 0x05}, pts...)
	pesLen := len(hdrData) + len(adts) - 2 // excludes the 2 length bytes
	pes := append([]byte{0x00, 0x00, 0x01, 0xC0, byte(pesLen >> 8), byte(pesLen)}, hdrData...)
	pes = append(pes, adts...)
	restored := demuxAudioPES(audioTSPacket(pes, true))
	frames, abytes, rate, ch := walkADTS(restored)
	if frames != 3 || abytes != 3*fl || rate != 44100 || ch != 2 {
		t.Fatalf("walkADTS = %d/%d/%d/%d, want 3/%d/44100/2", frames, abytes, rate, ch, 3*fl)
	}
}

// Regression: a rejected sub-three-frame chain must not leak its frame and
// byte counts into the next candidate — the stale totals could push a later
// chain over the three-frame threshold and fabricate a track with inflated
// byte counts (skewing the muxed-audio bitrate the quality gate computes).
func TestWalkADTSRejectsShortChainWithoutLeak(t *testing.T) {
	const fl = 60
	var frame []byte
	frame = append(frame, adtsHeader(fl)...)
	frame = append(frame, make([]byte, fl-7)...)

	var audio []byte
	audio = append(audio, frame...)            // candidate 1: a lone frame
	audio = append(audio, make([]byte, 16)...) // gap with no sync bytes
	for i := 0; i < 3; i++ {                   // candidate 2: the real chain
		audio = append(audio, frame...)
	}
	frames, abytes, rate, ch := walkADTS(audio)
	if frames != 3 || abytes != 3*fl || rate != 44100 || ch != 2 {
		t.Fatalf("walkADTS = %d/%d/%d/%d, want 3/%d/44100/2 (stale counts leaked)", frames, abytes, rate, ch, 3*fl)
	}
}
