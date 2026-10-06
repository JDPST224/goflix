package cinesrcjs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dop251/goja"
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

// runAction performs a fetch() call from INSIDE the VM so any fetch wrapper
// the challenge module installed sees and processes the response, mirroring
// the embed's own action calls. Returns the (possibly wrapper-transformed)
// response text.
func (rt *runtime) runAction(ctx context.Context, path string, headers map[string]string, body []byte) (string, error) {
	hdrJSON, err := json.Marshal(headers)
	if err != nil {
		return "", err
	}
	bodyJSON, err := json.Marshal(string(body))
	if err != nil {
		return "", err
	}
	script := `(function(){
		__actionOut = null;
		try { if (typeof __fileLog === "function") __fileLog("runAction: fetch wrapped=" + (fetch !== window.__baseFetch)); } catch(e) {}
		fetch(` + strconv.Quote(path) + `, {method: "POST", headers: ` + string(hdrJSON) + `, body: ` + string(bodyJSON) + `}).then(function(r){
			return r.text().then(function(t){ return {status: r.status, text: t}; });
		}).then(function(v){ __actionOut = {ok: true, v: v}; },
		      function(e){ __actionOut = {ok: false, err: String(e && e.message || e)}; });
	})()`
	if _, err := rt.runSrc(script); err != nil {
		return "", err
	}
	for i := 0; i < 800; i++ {
		rt.drain(ctx)
		out := rt.vm.GlobalObject().Get("__actionOut")
		if out != nil && !goja.IsUndefined(out) && !goja.IsNull(out) {
			o := out.ToObject(rt.vm)
			if !o.Get("ok").ToBoolean() {
				return "", fmt.Errorf("action fetch failed: %s", o.Get("err").String())
			}
			v := o.Get("v").ToObject(rt.vm)
			return v.Get("text").String(), nil
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
	return "", errors.New("action fetch timeout")
}
