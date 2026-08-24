// Package subtitle implements subtitle discovery (Videasy, Vidlove) and the
// SRT→WebVTT conversion served to the browser's <track> elements.
package subtitles

import (
	"strconv"
	"strings"
)

// vttCue is a parsed subtitle cue kept while de-duplicating a track. Cues are
// indexed by normalized text, so only the timing range is stored per entry.
type vttCue struct {
	start, end float64
}

// parseCueTimestamp parses "HH:MM:SS.mmm" / "MM:SS.mmm" (WebVTT) and their
// SRT comma variants into seconds.
func parseCueTimestamp(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	s = strings.ReplaceAll(s, ",", ".")
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, false
	}
	var hours float64
	if len(parts) == 3 {
		var err error
		if hours, err = strconv.ParseFloat(parts[0], 64); err != nil {
			return 0, false
		}
		parts = parts[1:]
	}
	mins, err1 := strconv.ParseFloat(parts[0], 64)
	secs, err2 := strconv.ParseFloat(parts[1], 64)
	if err1 != nil || err2 != nil {
		return 0, false
	}
	return hours*3600 + mins*60 + secs, true
}

// normalizeCueText folds a cue's text lines into one comparison key.
func normalizeCueText(lines []string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.Join(lines, " ")), " "))
}

// srtToWebVTTCore converts an SRT-formatted subtitle string to standard
// WebVTT. It strips UTF-8 BOMs, replaces commas in timestamps with periods,
// ensures a valid WEBVTT header, and drops duplicate cues: provider files
// sometimes merge two caption variants carrying the same line inside
// overlapping time ranges, which the browser then renders as two identical
// stacked subtitles.
func SrtToWebVTT(srt string) string {
	// Strip UTF-8 BOM and zero-width characters if present.
	srt = strings.TrimPrefix(srt, "\xef\xbb\xbf")
	srt = strings.TrimPrefix(srt, "\ufeff")

	// Normalise line endings.
	srt = strings.ReplaceAll(srt, "\r\n", "\n")
	srt = strings.ReplaceAll(srt, "\r", "\n")
	srt = strings.TrimSpace(srt)

	isVTT := strings.HasPrefix(srt, "WEBVTT")

	var out strings.Builder
	out.WriteString("WEBVTT\n\n")

	// Duplicate cues are indexed by normalized text so each new cue only
	// compares against cues with identical text. A linear scan over every
	// kept cue made large SRTs quadratic (~10^8 comparisons at 15k cues),
	// stalling the download handler for seconds.
	keptByText := make(map[string][]vttCue, 512)
	for _, block := range strings.Split(srt, "\n\n") {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		lines := strings.Split(block, "\n")
		tsIdx := -1
		for i, line := range lines {
			if strings.Contains(line, "-->") {
				tsIdx = i
				break
			}
		}
		if tsIdx == -1 {
			// Non-cue block: VTT NOTE/STYLE blocks pass through; SRT index
			// noise and the VTT header (rewritten fresh above) are dropped.
			if isVTT && !strings.HasPrefix(block, "WEBVTT") {
				out.WriteString(block)
				out.WriteString("\n\n")
			}
			continue
		}

		// Timestamp line: "00:00:01,000 --> 00:00:03,500" (SRT) or the dotted
		// WebVTT form, optionally with trailing cue settings.
		tsParts := strings.SplitN(lines[tsIdx], "-->", 2)
		if len(tsParts) != 2 {
			continue
		}
		start, okStart := parseCueTimestamp(tsParts[0])
		endFields := strings.Fields(tsParts[1])
		if !okStart || len(endFields) == 0 {
			continue
		}
		end, okEnd := parseCueTimestamp(endFields[0])
		if !okEnd || end <= start {
			continue
		}
		textLines := make([]string, 0, len(lines)-tsIdx-1)
		for _, l := range lines[tsIdx+1:] {
			if strings.TrimSpace(l) != "" {
				textLines = append(textLines, strings.TrimRight(l, " \t"))
			}
		}
		if len(textLines) == 0 {
			continue
		}

		// Drop cues repeating the exact same line inside an overlapping time
		// range — legitimate subtitles never do that, and the browser renders
		// every copy it is given.
		key := normalizeCueText(textLines)
		dup := false
		for _, k := range keptByText[key] {
			if start < k.end && k.start < end {
				dup = true
				break
			}
		}
		if dup {
			continue
		}
		keptByText[key] = append(keptByText[key], vttCue{start: start, end: end})

		// Re-emit the timestamp line in WebVTT form (commas → dots), keeping
		// any trailing cue settings, followed by the cue text.
		tsLine := strings.TrimSpace(tsParts[0]) + " --> " + endFields[0]
		if len(endFields) > 1 {
			tsLine += " " + strings.Join(endFields[1:], " ")
		}
		out.WriteString(strings.ReplaceAll(tsLine, ",", "."))
		out.WriteByte('\n')
		for _, l := range textLines {
			out.WriteString(l)
			out.WriteByte('\n')
		}
		out.WriteByte('\n')
	}
	return out.String()
}

// ValidSubtitleID checks that a Videasy subtitle ID contains only safe
// characters (alphanumeric, hyphens, underscores) and is a reasonable length.
func ValidSubtitleID(s string) bool {
	if s == "" || len(s) > 128 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '-' || c == '_') {
			return false
		}
	}
	return true
}
