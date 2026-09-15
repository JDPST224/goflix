package cinesrcjs

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
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
	if ck := rt.cookieHeader(); ck != "" {
		req.Header.Set("cookie", ck)
	}
	resp, err := rt.client.Do(req)
	if err != nil {
		return nil, err
	}
	if cookies := resp.Cookies(); len(cookies) > 0 {
		if u, perr := url.Parse(abs); perr == nil {
			rt.jar.SetCookies(u, cookies)
		}
	}
	return resp, nil
}
