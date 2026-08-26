package subtitles

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// osSubtitle represents a single subtitle entry from the OpenSubtitles search API.
type osSubtitle struct {
	IDSubtitleFile     string `json:"IDSubtitleFile"`
	SubFileName        string `json:"SubFileName"`
	LanguageName       string `json:"LanguageName"`
	SubDownloadLink    string `json:"SubDownloadLink"`
	SubFormat          string `json:"SubFormat"`
	SubHearingImpaired string `json:"SubHearingImpaired"`
	ISO639             string `json:"ISO639"`
}

// osSearchResponse is the top-level envelope returned by OpenSubtitles API.
type osSearchResponse []osSubtitle

// FrontendSubtitle is the shape we send back to the browser.
type FrontendSubtitle struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Language string `json:"language"`
	URL      string `json:"url"`
}

var (
	opensubtitlesAPIOrigin = "https://rest.opensubtitles.org"
	vidloveAPIOrigin = "https://api.shows.st"
)

// Client is shared across all subtitle search/download requests.
var Client = &http.Client{Timeout: 10 * time.Second}

// FetchOpenSubtitles queries the OpenSubtitles search API and maps the results
// to the frontend subtitle shape.
func FetchOpenSubtitles(ctx context.Context, queryID, season, episode string) []FrontendSubtitle {
	if queryID == "" {
		return nil
	}

	// Clean IMDb ID by removing "tt" prefix if present
	cleanID := strings.TrimPrefix(queryID, "tt")
	if len(cleanID) < 7 {
		cleanID = strings.Repeat("0", 7-len(cleanID)) + cleanID
	}

	var searchURL string
	if season != "" && episode != "" {
		searchURL = fmt.Sprintf("%s/search/episode-%s/imdbid-%s/season-%s", opensubtitlesAPIOrigin, episode, cleanID, season)
	} else {
		searchURL = fmt.Sprintf("%s/search/imdbid-%s", opensubtitlesAPIOrigin, cleanID)
	}

	searchReq, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil
	}
	searchReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")

	searchRes, err := Client.Do(searchReq)
	if err != nil {
		log.Printf("[Subtitles] OpenSubtitles search error query=%s: %v", queryID, err)
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

	var osResults osSearchResponse
	if err := json.Unmarshal(searchBody, &osResults); err != nil {
		// Log the error because it could be HTML if they limit requests
		log.Printf("[Subtitles] OpenSubtitles decode error: %v", err)
		return nil
	}

	result := make([]FrontendSubtitle, 0, len(osResults))
	for _, sub := range osResults {
		if sub.SubDownloadLink == "" {
			continue
		}
		format := strings.ToLower(sub.SubFormat)
		if format != "srt" && format != "vtt" {
			continue
		}
		label := sub.LanguageName
		if sub.SubHearingImpaired == "1" {
			label += " (HI)"
		}
		
		encodedLink := url.QueryEscape(sub.SubDownloadLink)
		result = append(result, FrontendSubtitle{
			ID:       "os_" + sub.IDSubtitleFile,
			Label:    label,
			Language: sub.ISO639, // Provide a short language code if possible
			URL:      "/api/subtitles/opensubtitles/download?url=" + encodedLink,
		})
	}
	return result
}
