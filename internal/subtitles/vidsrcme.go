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

var vidsrcmeDataOrigin = "https://data.vidsrcme.ru"

// FetchVidsrcmeSubtitles queries the VidSrcMe API and maps the default_subs results
// to the frontend subtitle shape.
func FetchVidsrcmeSubtitles(ctx context.Context, mediaType, id, season, episode string) []FrontendSubtitle {
	subs, _ := FetchVidsrcmeSubtitlesWithIMDb(ctx, mediaType, id, season, episode)
	return subs
}

// FetchVidsrcmeSubtitlesWithIMDb queries the VidSrcMe API and returns both the subtitle list
// and any IMDb ID provided in data.imdb_id (for fallback providers).
func FetchVidsrcmeSubtitlesWithIMDb(ctx context.Context, mediaType, id, season, episode string) ([]FrontendSubtitle, string) {
	apiURL := vidsrcmeDataOrigin + "/api.php?type=" + url.QueryEscape(mediaType) + "&tmdb=" + url.QueryEscape(id)
	if mediaType == "tv" && season != "" && episode != "" {
		apiURL += "&season=" + url.QueryEscape(season) + "&episode=" + url.QueryEscape(episode)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, ""
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://cloudorchestranova.com/")
	req.Header.Set("Accept", "application/json")

	res, err := Client.Do(req)
	if err != nil {
		log.Printf("[Subtitles] VidSrcMe search error id=%s: %v", id, err)
		return nil, ""
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, ""
	}

	var apiRes struct {
		Data struct {
			ImdbID string `json:"imdb_id"`
		} `json:"data"`
		DefaultSubs []struct {
			Lang string `json:"lang"`
			Code string `json:"code"`
			URL  string `json:"url"`
		} `json:"default_subs"`
	}

	if err := json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(&apiRes); err != nil {
		return nil, ""
	}

	result := make([]FrontendSubtitle, 0, len(apiRes.DefaultSubs))
	for i, sub := range apiRes.DefaultSubs {
		if sub.URL == "" {
			continue
		}
		label := sub.Lang
		if label == "" {
			label = "Unknown"
		}
		result = append(result, FrontendSubtitle{
			ID:       fmt.Sprintf("vidsrcme_%d", i),
			Label:    label,
			Language: label,
			URL:      "/api/subtitles/vidsrcme/download?url=" + url.QueryEscape(sub.URL),
		})
	}

	return result, apiRes.Data.ImdbID
}
