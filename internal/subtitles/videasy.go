package subtitles

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"
)

// videasySubtitle represents a single subtitle entry from the Videasy search API.
type videasySubtitle struct {
	ID                string `json:"id"`
	Display           string `json:"display"`
	Language          string `json:"language"`
	Format            string `json:"format"`
	IsHearingImpaired bool   `json:"isHearingImpaired"`
}

// videasySearchResponse is the top-level envelope returned by
// https://subs.videasy.to/search?id=<imdb-id>.
// The API may return either an array directly or a wrapper object — we handle
// the array form here (the most common one observed).
type videasySearchResponse []videasySubtitle

// FrontendSubtitle is the shape we send back to the browser.
type FrontendSubtitle struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Language string `json:"language"`
	URL      string `json:"url"`
}

// Client is shared across all subtitle search/download requests.
var Client = &http.Client{Timeout: 10 * time.Second}

// Origins are package variables so tests can point them at a local
// httptest server instead of the real APIs.
var (
	videasyAPIOrigin = "https://subs.videasy.to"
	vidloveAPIOrigin = "https://api.shows.st"
)

// FetchVideasySubtitles queries the Videasy search API and maps the results
// to the frontend subtitle shape. A nil result means the lookup failed or
// found nothing; callers treat both identically.
func FetchVideasySubtitles(ctx context.Context, queryID, season, episode string) []FrontendSubtitle {
	if queryID == "" {
		return nil
	}
	// Build the query with proper parameter encoding so season/episode are
	// sent as separate parameters (escaping the whole "id&season=..." string
	// would send them as part of the id value and break the lookup).
	params := url.Values{"id": {queryID}}
	if season != "" {
		params.Set("season", season)
	}
	if episode != "" {
		params.Set("episode", episode)
	}
	searchURL := videasyAPIOrigin + "/search?" + params.Encode()
	searchReq, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil
	}
	searchReq.Header.Set("Accept", "application/json")
	searchReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")

	searchRes, err := Client.Do(searchReq)
	if err != nil {
		log.Printf("[Subtitles] Videasy search error query=%s: %v", queryID, err)
		return nil
	}
	defer searchRes.Body.Close()

	if searchRes.StatusCode != http.StatusOK {
		return nil
	}

	searchBody, err := io.ReadAll(io.LimitReader(searchRes.Body, 2<<20))
	if err != nil {
		return nil
	}

	var videasyResults videasySearchResponse
	if err := json.Unmarshal(searchBody, &videasyResults); err != nil {
		return nil
	}

	result := make([]FrontendSubtitle, 0, len(videasyResults))
	for _, sub := range videasyResults {
		if sub.ID == "" {
			continue
		}
		label := sub.Display
		if label == "" {
			label = sub.Language
		}
		if sub.IsHearingImpaired {
			label += " (HI)"
		}
		result = append(result, FrontendSubtitle{
			ID:       "videasy_" + sub.ID,
			Label:    label,
			Language: sub.Language,
			URL:      "/api/subtitles/videasy/download/" + sub.ID,
		})
	}
	return result
}
