// tmdb.go — one TMDB HTTP client shared by catalog refreshes, search,
// detail and episodes passthroughs.
package catalog

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	tmdbAPIBase   = "https://api.themoviedb.org/3"
	tmdbImageBase = "https://image.tmdb.org/t/p"
	responseCap   = 10 << 20
)

// Client is an authenticated TMDB client.
type Client struct {
	http   *http.Client
	token  string // v4 Bearer Read Access Token
	apiKey string // v3 API key (fallback)
}

// NewClient builds a client on the shared 15s-timeout HTTP client.
func NewClient(token, apiKey string) *Client {
	return &Client{
		http:   &http.Client{Timeout: 15 * time.Second},
		token:  token,
		apiKey: apiKey,
	}
}

// HasCredentials reports whether any TMDB credential is configured.
func (c *Client) HasCredentials() bool { return c.token != "" || c.apiKey != "" }

// buildURL prefixes the API base and appends api_key only when no Bearer
// token is configured — mirroring the original credential precedence.
func (c *Client) buildURL(endpoint string) string {
	if c.apiKey != "" && c.token == "" {
		sep := "?"
		if strings.Contains(endpoint, "?") {
			sep = "&"
		}
		return tmdbAPIBase + endpoint + sep + "api_key=" + c.apiKey
	}
	return tmdbAPIBase + endpoint
}

// List fetches a TMDB list endpoint and maps results to catalog Movies,
// dropping posterless/titleless entries. Returns nil when credentials are
// missing, the request fails, or the payload cannot be parsed — mirroring
// fetchTMDB's silent-nil error convention.
func (c *Client) List(endpoint string, categories []string, mediaType string) []Movie {
	if !c.HasCredentials() {
		log.Println("No TMDB credentials, skipping fetch for", endpoint)
		return nil
	}

	rawURL := c.buildURL(endpoint)
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		log.Println("Error creating request:", err)
		return nil
	}
	req.Header.Add("accept", "application/json")
	if c.token != "" {
		req.Header.Add("Authorization", "Bearer "+c.token)
	}

	res, err := c.http.Do(req)
	if err != nil {
		log.Println("Error fetching TMDB data:", err)
		return nil
	}
	defer res.Body.Close()

	body, err := io.ReadAll(io.LimitReader(res.Body, responseCap))
	if err != nil {
		log.Println("Error reading response body:", err)
		return nil
	}
	if res.StatusCode != http.StatusOK {
		log.Printf("TMDB API error for %s: status=%d body=%s\n", endpoint, res.StatusCode, string(body))
		return nil
	}

	var tmdbRes TMDBResponse
	if err := json.Unmarshal(body, &tmdbRes); err != nil {
		log.Println("Error unmarshaling TMDB JSON:", err)
		return nil
	}
	var movies []Movie
	for _, m := range tmdbRes.Results {
		mapped := MapTMDBMovie(m, mediaType, categories)
		if mapped != nil {
			movies = append(movies, *mapped)
		}
	}

	log.Printf("Fetched %d items for %v (%s)\n", len(movies), categories, mediaType)
	return movies
}

// SearchMulti hits /search/multi, keeps media_type movie/tv results and maps
// them under the "Search Results" category.
func (c *Client) SearchMulti(encodedQuery string) []Movie {
	if !c.HasCredentials() {
		return nil
	}

	endpoint := "/search/multi?query=" + encodedQuery + "&language=en-US&page=1"
	rawURL := c.buildURL(endpoint)

	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Add("accept", "application/json")
	if c.token != "" {
		req.Header.Add("Authorization", "Bearer "+c.token)
	}

	res, err := c.http.Do(req)
	if err != nil {
		return nil
	}
	defer res.Body.Close()

	body, err := io.ReadAll(io.LimitReader(res.Body, responseCap))
	if err != nil {
		return nil
	}
	if res.StatusCode != http.StatusOK {
		return nil
	}
	var raw TMDBResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil
	}

	var movies []Movie
	categories := []string{"Search Results"}
	for _, m := range raw.Results {
		if m.MediaType != "movie" && m.MediaType != "tv" {
			continue
		}
		mapped := MapTMDBMovie(m, m.MediaType, categories)
		if mapped != nil {
			movies = append(movies, *mapped)
		}
	}
	return movies
}

// MovieDetail fetches /movie|tv/{id} with the detail modal's append_to_response
// set. It replicates the legacy handler's exact status mapping:
//
//	500 {}  — request-build or response-read failure (our fault)
//	502 {}  — transport failure or upstream non-200 (their fault)
//	200 + raw body — passthrough
func (c *Client) MovieDetail(mediaType, id string) (int, []byte) {
	endpoint := "/movie/" + id
	if mediaType == "tv" {
		endpoint = "/tv/" + id
	}
	return c.fetchPassthrough(endpoint)
}

func (c *Client) fetchPassthrough(endpoint string) (int, []byte) {
	rawURL := c.buildURL(endpoint)
	u, err := url.Parse(rawURL)
	if err != nil {
		return http.StatusInternalServerError, []byte(`{}`)
	}
	q := u.Query()
	q.Set("append_to_response", "credits,videos,recommendations,external_ids")
	q.Set("language", "en-US")
	u.RawQuery = q.Encode()

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return http.StatusInternalServerError, []byte(`{}`)
	}
	req.Header.Add("accept", "application/json")
	if c.token != "" {
		req.Header.Add("Authorization", "Bearer "+c.token)
	}

	res, err := c.http.Do(req)
	if err != nil {
		return http.StatusBadGateway, []byte(`{}`)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return http.StatusBadGateway, []byte(`{}`)
	}

	body, err := io.ReadAll(io.LimitReader(res.Body, responseCap))
	if err != nil {
		return http.StatusInternalServerError, []byte(`{}`)
	}
	return http.StatusOK, body
}

// SeasonEpisodes fetches /tv/{id}/season/{s} for the episode picker, with the
// same 500/502 mapping as MovieDetail.
func (c *Client) SeasonEpisodes(id, season string) (int, []byte) {
	endpoint := fmt.Sprintf("/tv/%s/season/%s?language=en-US", id, season)
	rawURL := c.buildURL(endpoint)

	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return http.StatusInternalServerError, []byte(`{}`)
	}
	req.Header.Add("accept", "application/json")
	if c.token != "" {
		req.Header.Add("Authorization", "Bearer "+c.token)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return http.StatusBadGateway, []byte(`{}`)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return http.StatusBadGateway, []byte(`{}`)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, responseCap))
	if err != nil {
		return http.StatusInternalServerError, []byte(`{}`)
	}
	return http.StatusOK, body
}

// ExternalID retrieves the IMDb identifier used by the subtitle ladders.
// For movies: /movie/{id}/external_ids; for TV episodes the season and
// episode-scoped endpoint is required.
func (c *Client) ExternalID(mediaType, id, season, episode string) (string, error) {
	var endpoint string
	if mediaType == "movie" {
		endpoint = fmt.Sprintf("/movie/%s/external_ids", id)
	} else {
		if season == "" || episode == "" {
			return "", fmt.Errorf("season and episode required for TV")
		}
		endpoint = fmt.Sprintf("/tv/%s/season/%s/episode/%s/external_ids", id, season, episode)
	}

	rawURL := c.buildURL(endpoint)
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Add("accept", "application/json")
	if c.token != "" {
		req.Header.Add("Authorization", "Bearer "+c.token)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("TMDB external_ids %s: status %d", endpoint, res.StatusCode)
	}
	var ext struct {
		IMDbID string `json:"imdb_id"`
	}
	if json.Unmarshal(body, &ext) != nil {
		return "", fmt.Errorf("invalid external_ids payload")
	}
	if ext.IMDbID == "" {
		return "", fmt.Errorf("no IMDb ID found for %s %s", mediaType, id)
	}
	return ext.IMDbID, nil
}
