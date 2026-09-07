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

const cinesrcSubsOrigin = "https://subs.bright67.online"

// CineSrcSubtitleItem matches the structure returned by https://subs.bright67.online/search
type CineSrcSubtitleItem struct {
	ID                string `json:"id"`
	URL               string `json:"url"`
	Format            string `json:"format"`
	Encoding          string `json:"encoding"`
	Display           string `json:"display"`
	Language          string `json:"language"`
	IsHearingImpaired bool   `json:"isHearingImpaired"`
	IsForced          bool   `json:"isForced"`
}

// FetchCinesrcSubtitles queries the CineSrc subtitle API (subs.bright67.online)
// and maps the results to the frontend subtitle shape.
func FetchCinesrcSubtitles(ctx context.Context, mediaType, id, season, episode string) []FrontendSubtitle {
	apiURL := cinesrcSubsOrigin + "/search?id=" + url.QueryEscape(id)
	if mediaType == "tv" && season != "" && episode != "" {
		apiURL += "&season=" + url.QueryEscape(season) + "&episode=" + url.QueryEscape(episode)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://cinesrc.st/")
	req.Header.Set("Accept", "application/json")

	res, err := Client.Do(req)
	if err != nil {
		log.Printf("[Subtitles] CineSrc search error id=%s: %v", id, err)
		return nil
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil
	}

	var items []CineSrcSubtitleItem
	if err := json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(&items); err != nil {
		log.Printf("[Subtitles] CineSrc decode error id=%s: %v", id, err)
		return nil
	}

	result := make([]FrontendSubtitle, 0, len(items))
	seen := make(map[string]bool)

	for _, sub := range items {
		if sub.URL == "" {
			continue
		}
		label := sub.Display
		if label == "" {
			label = sub.Language
		}
		if label == "" {
			label = "Unknown"
		}
		if sub.IsForced {
			label += " [Forced]"
		} else if sub.IsHearingImpaired {
			label += " [CC]"
		}

		key := fmt.Sprintf("%s|%s", sub.Language, label)
		if seen[key] {
			continue
		}
		seen[key] = true

		result = append(result, FrontendSubtitle{
			ID:       fmt.Sprintf("cinesrc_%s", sub.ID),
			Label:    label,
			Language: sub.Language,
			URL:      "/api/subtitles/cinesrc/download?url=" + url.QueryEscape(sub.URL),
		})
	}

	return result
}
