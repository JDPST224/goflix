package subtitles

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
)

// FetchVidloveSubtitles queries the Vidlove subtitle API and maps the results
// to the frontend subtitle shape. Subtitle entries are identified by their
// list position; the raw file URL rides in the download link's query string.
func FetchVidloveSubtitles(ctx context.Context, mediaType, id, season, episode string) []FrontendSubtitle {
	apiURL := vidloveAPIOrigin + "/" + mediaType + "?id=" + url.QueryEscape(id) + "&mode=json"
	if mediaType == "tv" && season != "" && episode != "" {
		apiURL += "&season=" + url.QueryEscape(season) + "&episode=" + url.QueryEscape(episode)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Referer", "https://player.vidlove.cc/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")

	res, err := Client.Do(req)
	if err != nil {
		log.Printf("[Subtitles] Vidlove search error id=%s: %v", id, err)
		return nil
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil
	}

	var apiRes struct {
		Subtitles []struct {
			File  string `json:"file"`
			Label string `json:"label"`
			Type  string `json:"type"`
		} `json:"subtitles"`
	}

	if err := json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(&apiRes); err != nil {
		return nil
	}

	result := make([]FrontendSubtitle, 0, len(apiRes.Subtitles))
	for i, sub := range apiRes.Subtitles {
		if sub.File == "" {
			continue
		}
		label := sub.Label
		if label == "" {
			label = "Unknown"
		}
		result = append(result, FrontendSubtitle{
			ID:       fmt.Sprintf("vidlove_%d", i),
			Label:    label,
			Language: label,
			URL:      "/api/subtitles/vidlove/download?url=" + url.QueryEscape(sub.File),
		})
	}
	return result
}
