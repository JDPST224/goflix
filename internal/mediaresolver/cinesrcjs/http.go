package cinesrcjs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// postJSON posts a JSON body (or none, when body is nil) and parses the JSON
// response into a map.
func (rt *runtime) postJSON(ctx context.Context, path string, headers map[string]string, body any) (map[string]any, error) {
	var data []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		data = b
		headers["content-type"] = "application/json"
	}
	resp, err := rt.do(ctx, "POST", path, headers, data)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Decode failures on an HTML error page read as cryptic JSON
		// errors; surface the status (and a body snippet) instead.
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return nil, fmt.Errorf("%s: status %d: %s", path, resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
	var out map[string]any
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// postRaw performs the HTTP request and returns the body text.
func (rt *runtime) postRaw(ctx context.Context, path string, headers map[string]string, body []byte) (string, error) {
	resp, err := rt.do(ctx, "POST", path, headers, body)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return "", fmt.Errorf("%s: status %d: %s", path, resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<22))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (rt *runtime) do(ctx context.Context, method, path string, headers map[string]string, body []byte) (*http.Response, error) {
	abs, err := resolveURL(path, rt.origin)
	if err != nil {
		return nil, err
	}
	var reader *bytes.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, abs, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("user-agent", rt.ua)
	req.Header.Set("origin", rt.origin)
	req.Header.Set("referer", rt.origin+"/")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	// Session cookies come from the client's jar (set by newRuntime),
	// which also stores Set-Cookie from every redirect hop under the
	// response's own URL.
	resp, err := rt.client.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}
