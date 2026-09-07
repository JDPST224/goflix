package cinesrcjs

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
)

// startEpoch anchors performance.now().
var startEpoch = time.Now()

// runtime bundles the goja VM with its Go-side environment: HTTP client with
// cookie jar, WebCrypto shims, timer/event queue and Worker emulation.
type runtime struct {
	vm       *goja.Runtime
	client   *http.Client
	jar      *cookiejar.Jar
	origin   string
	pagePath string
	ua       string
	fp       Fingerprint

	mu         sync.Mutex
	injected   map[string]string               // x-cs-* headers injected around gc()
	getPow     func() (*powRuntime, error)     // shared, pre-warmed wasm PoW
	getPK      func(url string) (string, bool) // /api/c/pk cache lookup
	storePK    func(url, body string)          // /api/c/pk cache store
	nextCanvas func() string                   // engine-wide canvas fingerprint rotation
	timerSeq   int
	timers     map[int]goja.Callable
	dueTimers  map[int]time.Time
	workerJobs []func() // deferred worker onmessage deliveries
	consoleFn  func(string)
}

func newRuntime(origin, pagePath, ua string, fp Fingerprint, jar *cookiejar.Jar, transport http.RoundTripper, consoleFn func(string)) (*runtime, error) {
	client := &http.Client{Transport: transport, Timeout: 15 * time.Second}
	rt := &runtime{
		vm:        goja.New(),
		client:    client,
		jar:       jar,
		origin:    strings.TrimRight(origin, "/"),
		pagePath:  pagePath,
		ua:        ua,
		fp:        fp,
		injected:  map[string]string{},
		timers:    map[int]goja.Callable{},
		dueTimers: map[int]time.Time{},
		consoleFn: consoleFn,
	}
	if err := rt.setup(); err != nil {
		return nil, err
	}
	return rt, nil
}

func must(vm *goja.Runtime, err error) {
	if err != nil {
		panic(err)
	}
}

func (rt *runtime) dbg(s string) {
	if rt.consoleFn != nil {
		rt.consoleFn(s)
	}
}

func (rt *runtime) setup() error {
	vm := rt.vm
	g := vm.GlobalObject()

	if rt.consoleFn != nil {
		consoleFn := rt.consoleFn
		must(vm, g.Set("__consoleFn", func(call goja.FunctionCall) goja.Value {
			consoleFn(call.Argument(0).String())
			return goja.Undefined()
		}))
	}

	// Identity shims. `global` must exist because the obfuscated module
	// eval()s a wrapper that captures it.
	for _, name := range []string{"window", "self", "top", "parent", "frames", "global"} {
		must(vm, g.Set(name, g))
	}
	must(vm, g.Set("module", vm.NewObject()))

	must(vm, g.Set("navigator", map[string]interface{}{
		"userAgent":           rt.ua,
		"language":            rt.fp.Language,
		"languages":           strings.Split(rt.fp.Languages, ","),
		"platform":            rt.fp.Platform,
		"hardwareConcurrency": 8,
		"deviceMemory":        8,
		"maxTouchPoints":      0,
		"webdriver":           false,
		"vendor":              "Google Inc.",
		"cookieEnabled":       true,
	}))

	must(vm, g.Set("location", map[string]interface{}{
		"href":     rt.origin + rt.pagePath,
		"protocol": "https:",
		"host":     hostOf(rt.origin),
		"hostname": hostOf(rt.origin),
		"origin":   rt.origin,
		"pathname": rt.pagePath,
		"search":   "",
		"hash":     "",
	}))

	must(vm, g.Set("screen", map[string]interface{}{
		"width": rt.fp.ScreenW, "height": rt.fp.ScreenH,
		"availWidth": rt.fp.ScreenW, "availHeight": rt.fp.ScreenH,
		"colorDepth": 24,
	}))

	// Timers. The pump in drain() runs due callbacks between JS turns
	// (goja is single-goroutine; no locking needed beyond the mutex above).
	// Delays are respected: challenge scripts register long rejection
	// timeouts that must not fire early.
	must(vm, g.Set("setTimeout", func(call goja.FunctionCall) goja.Value {
		fn, ok := goja.AssertFunction(call.Argument(0))
		if !ok {
			return vm.ToValue(0)
		}
		delayMs := call.Argument(1).ToInteger()
		rt.mu.Lock()
		rt.timerSeq++
		id := rt.timerSeq
		rt.timers[id] = fn
		rt.dueTimers[id] = time.Now().Add(time.Duration(delayMs) * time.Millisecond)
		rt.mu.Unlock()
		return vm.ToValue(id)
	}))
	must(vm, g.Set("clearTimeout", func(call goja.FunctionCall) goja.Value {
		rt.mu.Lock()
		delete(rt.timers, int(call.Argument(0).ToInteger()))
		rt.mu.Unlock()
		return nil
	}))
	must(vm, g.Set("setInterval", func(goja.FunctionCall) goja.Value { return vm.ToValue(0) }))
	must(vm, g.Set("clearInterval", func(goja.FunctionCall) goja.Value { return goja.Undefined() }))
	must(vm, g.Set("queueMicrotask", func(call goja.FunctionCall) goja.Value {
		if fn, ok := goja.AssertFunction(call.Argument(0)); ok {
			_, _ = fn(goja.Undefined(), nil)
		}
		return nil
	}))
	must(vm, g.Set("requestAnimationFrame", func(call goja.FunctionCall) goja.Value {
		if fn, ok := goja.AssertFunction(call.Argument(0)); ok {
			_, _ = fn(goja.Undefined(), nil)
		}
		return vm.ToValue(0)
	}))
	must(vm, g.Set("cancelAnimationFrame", func(goja.FunctionCall) goja.Value { return goja.Undefined() }))

	perf := vm.NewObject()
	_ = perf.Set("now", func(goja.FunctionCall) goja.Value {
		return vm.ToValue(float64(time.Since(startEpoch).Microseconds()) / 1000.0)
	})
	must(vm, g.Set("performance", perf))

	// Intl (goja has none; only the timezone is read by the fingerprint).
	// Called both with and without `new` by the fingerprint code.
	intl := vm.NewObject()
	_ = intl.Set("DateTimeFormat", func(call goja.FunctionCall) goja.Value {
		o := vm.NewObject()
		_ = o.Set("resolvedOptions", func(goja.FunctionCall) goja.Value {
			opts := vm.NewObject()
			_ = opts.Set("timeZone", rt.fp.TZ)
			return opts
		})
		return o
	})
	must(vm, g.Set("Intl", intl))

	// base64 with binary-string semantics: each byte ↔ one UTF-16 code unit
	must(vm, g.Set("btoa", func(call goja.FunctionCall) goja.Value {
		s := call.Argument(0).String()
		b := make([]byte, 0, len(s))
		for _, r := range s {
			if r > 0xFF {
				panic(vm.NewGoError(errors.New("btoa: char out of range")))
			}
			b = append(b, byte(r))
		}
		out := base64.StdEncoding.EncodeToString(b)
		return vm.ToValue(out)
	}))
	must(vm, g.Set("atob", func(call goja.FunctionCall) goja.Value {
		s := call.Argument(0).String()
		b, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		runes := make([]rune, len(b))
		for i, c := range b {
			runes[i] = rune(c)
		}
		return vm.ToValue(string(runes))
	}))

	// TextEncoder / TextDecoder (UTF-8 only, which is all the scripts use).
	// encode() must return a real Uint8Array — the challenge code reads
	// .length and indexes it directly.
	must(vm, g.Set("TextEncoder", func(call goja.ConstructorCall) *goja.Object {
		_ = call.This.Set("encode", func(c goja.FunctionCall) goja.Value {
			u8ctor, ok := goja.AssertFunction(vm.GlobalObject().Get("Uint8Array"))
			if !ok {
				panic(vm.NewGoError(errors.New("Uint8Array missing")))
			}
			ab := vm.ToValue(vm.NewArrayBuffer([]byte(c.Argument(0).String())))
			out, err := u8ctor(goja.Undefined(), ab)
			if err != nil {
				panic(vm.NewGoError(err))
			}
			return out
		})
		return nil
	}))
	must(vm, g.Set("TextDecoder", func(call goja.ConstructorCall) *goja.Object {
		_ = call.This.Set("decode", func(c goja.FunctionCall) goja.Value {
			b, err := toBytes(vm, c.Argument(0))
			if err != nil {
				panic(vm.NewGoError(err))
			}
			return vm.ToValue(string(b))
		})
		return nil
	}))

	// Event plumbing: the challenge module announces itself via a `_cs` event.
	must(vm, g.Set("__listeners", vm.NewObject()))
	listeners := g.Get("__listeners")
	addL := func(call goja.FunctionCall) goja.Value {
		typ := call.Argument(0).String()
		key := typ
		arrV := listeners.ToObject(vm).Get(key)
		var arr *goja.Object
		if arrV == nil || goja.IsUndefined(arrV) || goja.IsNull(arrV) {
			arr = vm.NewArray()
			_ = listeners.ToObject(vm).Set(key, arr)
		} else {
			arr = arrV.ToObject(vm)
		}
		n := arr.Get("length").ToInteger()
		_ = arr.Set(fmt.Sprintf("%d", n), call.Argument(1))
		return goja.Undefined()
	}
	must(vm, g.Set("addEventListener", addL))
	must(vm, g.Set("removeEventListener", func(goja.FunctionCall) goja.Value { return goja.Undefined() }))
	must(vm, g.Set("dispatchEvent", func(call goja.FunctionCall) goja.Value {
		ev := call.Argument(0)
		typ := ev.ToObject(vm).Get("type").String()
		rt.fireEvent(typ, ev)
		return vm.ToValue(true)
	}))
	must(vm, g.Set("CustomEvent", func(call goja.ConstructorCall) *goja.Object {
		_ = call.This.Set("type", call.Argument(0).String())
		if d := call.Argument(1); d != nil && !goja.IsUndefined(d) && !goja.IsNull(d) {
			if o, ok := d.(*goja.Object); ok {
				if detail := o.Get("detail"); detail != nil {
					_ = call.This.Set("detail", detail)
				}
			}
		}
		return nil
	}))
	must(vm, g.Set("Event", func(call goja.ConstructorCall) *goja.Object {
		_ = call.This.Set("type", call.Argument(0).String())
		return nil
	}))

	must(vm, g.Set("document", rt.documentObject()))
	must(vm, g.Set("history", map[string]interface{}{"pushState": func(goja.FunctionCall) goja.Value { return goja.Undefined() }}))
	must(vm, g.Set("localStorage", map[string]interface{}{
		"getItem":    func(goja.FunctionCall) goja.Value { return goja.Null() },
		"setItem":    func(goja.FunctionCall) goja.Value { return goja.Undefined() },
		"removeItem": func(goja.FunctionCall) goja.Value { return goja.Undefined() },
	}))
	must(vm, g.Set("sessionStorage", g.Get("localStorage")))
	must(vm, g.Set("getComputedStyle", func(goja.FunctionCall) goja.Value {
		return vm.NewObject()
	}))
	must(vm, g.Set("innerWidth", rt.fp.ScreenW))
	must(vm, g.Set("innerHeight", rt.fp.ScreenH))
	must(vm, g.Set("devicePixelRatio", 1))

	// crypto.subtle + getRandomValues
	subtle := newSubtleShim(vm)
	cryptoObj := vm.NewObject()
	subtleObj := vm.NewObject()
	subtle.install(subtleObj)
	_ = cryptoObj.Set("subtle", subtleObj)
	_ = cryptoObj.Set("getRandomValues", func(call goja.FunctionCall) goja.Value {
		obj, ok := call.Argument(0).(*goja.Object)
		if !ok {
			panic(vm.NewTypeError("getRandomValues: not a typed array"))
		}
		b, err := typedArrayBytes(vm, obj)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		if _, err := rand.Read(b); err != nil {
			panic(vm.NewGoError(err))
		}
		return obj
	})
	must(vm, g.Set("crypto", cryptoObj))

	// Blob + URL.createObjectURL: workers are emulated natively, so blobs
	// only need to exist under a synthetic URL.
	must(vm, g.Set("Blob", func(call goja.ConstructorCall) *goja.Object {
		_ = call.This.Set("__blobID", rt.nextBlobID())
		return nil
	}))
	// URL: constructible (new URL(x)) with static blob helpers.
	must(vm, g.Set("URL", func(call goja.ConstructorCall) *goja.Object {
		href := call.Argument(0).String()
		_ = call.This.Set("href", href)
		return nil
	}))
	urlFn := g.Get("URL").ToObject(vm)
	_ = urlFn.Set("createObjectURL", func(call goja.FunctionCall) goja.Value {
		return vm.ToValue("blob:goja/registered")
	})
	_ = urlFn.Set("revokeObjectURL", func(goja.FunctionCall) goja.Value { return goja.Undefined() })

	// fetch with cookie jar and x-cs-* header injection
	must(vm, g.Set("fetch", rt.fetchShim))

	// Worker emulation (worker.go)
	rt.installWorker(g)

	// Console (noise sink)
	console := vm.NewObject()
	logFn := func(call goja.FunctionCall) goja.Value { rt.dbg(argsToString(call)); return goja.Undefined() }
	_ = console.Set("log", logFn)
	_ = console.Set("error", logFn)
	_ = console.Set("warn", logFn)
	_ = console.Set("info", logFn)
	must(vm, g.Set("console", console))

	if rt.consoleFn != nil {
		// debug: trace zero-length Uint8Array constructions (payload bugs)
		_, _ = vm.RunString(`
			(function(){
				var Real = Uint8Array;
				var W = function(a, b, c){
					var out = new Real(a, b, c);
					if (out.length === 0) {
						__consoleFn("U8(0) constructed");
					}
					return out;
				};
				W.prototype = Real.prototype;
				window.Uint8Array = W;
			})()
		`)
	}

	return nil
}

func (rt *runtime) nextBlobID() int {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.timerSeq++
	return rt.timerSeq
}

func (rt *runtime) documentObject() *goja.Object {
	vm := rt.vm
	doc := vm.NewObject()
	head := vm.NewObject()
	_ = head.Set("appendChild", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
	_ = head.Set("removeChild", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
	_ = doc.Set("head", head)
	_ = doc.Set("body", head)
	_ = doc.Set("documentElement", vm.NewObject())
	_ = doc.Set("querySelector", func(goja.FunctionCall) goja.Value { return goja.Null() })
	_ = doc.Set("querySelectorAll", func(goja.FunctionCall) goja.Value { return vm.NewArray() })
	_ = doc.Set("addEventListener", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
	_ = doc.Set("removeEventListener", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
	_ = doc.Set("cookie", "")
	_ = doc.Set("readyState", "complete")
	_ = doc.Set("visibilityState", "visible")
	_ = doc.Set("currentScript", goja.Null())
	_ = doc.Set("createElement", func(call goja.FunctionCall) goja.Value {
		tag := strings.ToLower(call.Argument(0).String())
		el := vm.NewObject()
		_ = el.Set("tagName", tag)
		_ = el.Set("style", vm.NewObject())
		_ = el.Set("dataset", vm.NewObject())
		_ = el.Set("setAttribute", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
		_ = el.Set("remove", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
		_ = el.Set("appendChild", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
		_ = el.Set("addEventListener", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
		if tag == "canvas" {
			_ = el.Set("toDataURL", func(goja.FunctionCall) goja.Value {
				if rt.nextCanvas != nil {
					return vm.ToValue(rt.nextCanvas())
				}
				return vm.ToValue("")
			})
			_ = el.Set("getContext", func(call goja.FunctionCall) goja.Value {
				// Universal context stub: answers the known WebGL
				// fingerprint probes and no-ops everything else.
				vendor, renderer := rt.fp.WebGLVendor, rt.fp.WebGLRenderer
				src := `(function(vendor, renderer){
						return new Proxy({canvas:{}}, {
							get: function(t, p){
								if (p === 'getParameter') return function(k){
									if (k === 37445) return vendor;
									if (k === 37446) return renderer;
									if (k === 7936) return "WebKit";
									if (k === 7937) return "WebKit WebGL";
									if (k === 7938) return "WebGL GLSL ES 1.0";
									return 0;
								};
								if (p === 'getExtension') return function(){
									return {UNMASKED_VENDOR_WEBGL: 37445, UNMASKED_RENDERER_WEBGL: 37446};
								};
								if (p === 'getSupportedExtensions') return function(){
									return ["WEBGL_debug_renderer_info"];
								};
								if (p === 'measureText') return function(){ return {width: 10}; };
								if (p === 'getShaderPrecisionFormat') return function(){
									return {precision: 23, rangeMin: 127, rangeMax: 127};
								};
								return function(){ return undefined; };
							}
						});
					})`
				scriptVal, err := vm.RunString("(" + src + ")")
				if err != nil {
					panic(vm.NewGoError(err))
				}
				fn, ok := goja.AssertFunction(scriptVal)
				if !ok {
					panic(vm.NewGoError(errors.New("ctx stub: not a function")))
				}
				out, err := fn(goja.Undefined(), vm.ToValue(vendor), vm.ToValue(renderer))
				if err != nil {
					panic(vm.NewGoError(err))
				}
				return out
			})
		}
		return el
	})
	return doc
}

func (rt *runtime) fireEvent(typ string, ev goja.Value) {
	vm := rt.vm
	listeners := vm.GlobalObject().Get("__listeners")
	if listeners == nil || goja.IsUndefined(listeners) {
		return
	}
	arrV := listeners.ToObject(vm).Get(typ)
	if arrV == nil || goja.IsUndefined(arrV) || goja.IsNull(arrV) {
		return
	}
	arr := arrV.ToObject(vm)
	n := arr.Get("length").ToInteger()
	for i := int64(0); i < n; i++ {
		if fn, ok := goja.AssertFunction(arr.Get(fmt.Sprintf("%d", i))); ok {
			_, _ = fn(goja.Undefined(), ev)
		}
	}
}

// drain runs due timer callbacks and queued worker deliveries until quiet.
// It must be called on the VM's goroutine between JS turns.
func (rt *runtime) drain(ctx context.Context) {
	for i := 0; i < 128; i++ {
		now := time.Now()
		rt.mu.Lock()
		var dueFns []goja.Callable
		for id, due := range rt.dueTimers {
			if !now.Before(due) {
				if fn, ok := rt.timers[id]; ok {
					dueFns = append(dueFns, fn)
				}
				delete(rt.timers, id)
				delete(rt.dueTimers, id)
			}
		}
		jobs := rt.workerJobs
		rt.workerJobs = nil
		rt.mu.Unlock()
		if len(dueFns) == 0 && len(jobs) == 0 {
			return
		}
		for _, fn := range dueFns {
			if _, err := fn(goja.Undefined()); err != nil {
				rt.dbg("timer callback error: " + err.Error())
			}
		}
		for _, j := range jobs {
			j()
		}
		if ctx.Err() != nil {
			return
		}
	}
}

func (rt *runtime) fetchShim(call goja.FunctionCall) goja.Value {
	vm := rt.vm
	p, resolve, reject := vm.NewPromise()
	input := call.Argument(0)
	init, _ := call.Argument(1).(*goja.Object)

	rawURL := input.String()
	if o, ok := input.(*goja.Object); ok {
		if u := o.Get("url"); u != nil && !goja.IsUndefined(u) {
			rawURL = u.String()
		}
	}
	abs, err := resolveURL(rawURL, rt.origin)
	if err != nil {
		_ = reject(vm.NewGoError(err))
		return vm.ToValue(p)
	}

	method := "GET"
	if init != nil && init.Get("method") != nil && !goja.IsUndefined(init.Get("method")) {
		method = strings.ToUpper(init.Get("method").String())
	}
	headers := map[string]string{}
	if init != nil {
		if h, ok := init.Get("headers").(*goja.Object); ok && h != nil {
			for _, k := range h.Keys() {
				headers[strings.ToLower(k)] = h.Get(k).String()
			}
		}
	}
	var body []byte
	if init != nil {
		bv := init.Get("body")
		if bv != nil && !goja.IsUndefined(bv) && !goja.IsNull(bv) {
			body = []byte(bv.String())
		}
	}

	// injected challenge headers (x-cs-r/q/p around gc())
	rt.mu.Lock()
	for k, v := range rt.injected {
		headers[strings.ToLower(k)] = v
	}
	rt.mu.Unlock()
	headers["cookie"] = rt.cookieHeader()
	headers["user-agent"] = rt.ua
	headers["origin"] = rt.origin
	headers["referer"] = rt.origin + "/"

	// The challenge's RSA public key is static: serve /api/c/pk from the
	// engine-wide cache and skip the round trip entirely.
	if method == "GET" && strings.HasSuffix(pathOf(abs), "/api/c/pk") && rt.getPK != nil {
		if cached, ok := rt.getPK(abs); ok {
			pk := vm.NewObject()
			_ = pk.Set("ok", true)
			_ = pk.Set("status", 200)
			_ = pk.Set("statusText", "200 OK")
			hdr := vm.NewObject()
			_ = hdr.Set("get", func(goja.FunctionCall) goja.Value { return goja.Null() })
			_ = pk.Set("headers", hdr)
			_ = pk.Set("url", abs)
			_ = pk.Set("text", func(goja.FunctionCall) goja.Value {
				pr, res, _ := vm.NewPromise()
				_ = res(vm.ToValue(cached))
				return vm.ToValue(pr)
			})
			_ = pk.Set("json", func(goja.FunctionCall) goja.Value {
				pr, res, rej := vm.NewPromise()
				parsed, err := vm.RunString("(" + cached + ")")
				if err != nil {
					_ = rej(vm.NewGoError(err))
				} else {
					_ = res(parsed)
				}
				return vm.ToValue(pr)
			})
			_ = pk.Set("arrayBuffer", func(goja.FunctionCall) goja.Value {
				pr, res, _ := vm.NewPromise()
				_ = res(vm.ToValue(vm.NewArrayBuffer([]byte(cached))))
				return vm.ToValue(pr)
			})
			_ = resolve(pk)
			return vm.ToValue(p)
		}
	}

	req, err := http.NewRequest(method, abs, bytes.NewReader(body))
	if err != nil {
		_ = reject(vm.NewGoError(err))
		return vm.ToValue(p)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	fetchStart := time.Now()
	resp, err := rt.client.Do(req)
	if err != nil {
		_ = reject(vm.NewGoError(err))
		return vm.ToValue(p)
	}
	rt.dbg(fmt.Sprintf("fetch %s %s -> %d (%d ms)", method, pathOf(abs), resp.StatusCode, time.Since(fetchStart).Milliseconds()))
	defer resp.Body.Close()
	const limit = 4 << 20
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		_ = reject(vm.NewGoError(err))
		return vm.ToValue(p)
	}
	if cookies := resp.Cookies(); len(cookies) > 0 {
		if u, err := url.Parse(abs); err == nil {
			rt.jar.SetCookies(u, cookies)
		}
	}
	if strings.HasSuffix(pathOf(abs), "/api/c/pk") && rt.storePK != nil {
		rt.storePK(abs, string(bodyBytes))
	}

	respObj := vm.NewObject()
	_ = respObj.Set("ok", resp.StatusCode >= 200 && resp.StatusCode < 300)
	_ = respObj.Set("status", resp.StatusCode)
	_ = respObj.Set("statusText", resp.Status)
	hdr := vm.NewObject()
	_ = hdr.Set("get", func(c goja.FunctionCall) goja.Value {
		v := resp.Header.Get(c.Argument(0).String())
		if v == "" {
			return goja.Null()
		}
		return vm.ToValue(v)
	})
	_ = respObj.Set("headers", hdr)
	_ = respObj.Set("url", abs)

	_ = respObj.Set("text", func(c goja.FunctionCall) goja.Value {
		pr, res, _ := vm.NewPromise()
		_ = res(vm.ToValue(string(bodyBytes)))
		return vm.ToValue(pr)
	})
	_ = respObj.Set("json", func(c goja.FunctionCall) goja.Value {
		pr, res, rej := vm.NewPromise()
		parsed, err := vm.RunString("(" + string(bodyBytes) + ")")
		if err != nil {
			_ = rej(vm.NewGoError(err))
		} else {
			_ = res(parsed)
		}
		return vm.ToValue(pr)
	})
	_ = respObj.Set("arrayBuffer", func(c goja.FunctionCall) goja.Value {
		pr, res, _ := vm.NewPromise()
		_ = res(vm.ToValue(vm.NewArrayBuffer(bodyBytes)))
		return vm.ToValue(pr)
	})

	_ = resolve(respObj)
	return vm.ToValue(p)
}

func (rt *runtime) cookieHeader() string {
	u, err := url.Parse(rt.origin + rt.pagePath)
	if err != nil {
		return ""
	}
	cookies := rt.jar.Cookies(u)
	parts := make([]string, 0, len(cookies))
	for _, c := range cookies {
		parts = append(parts, c.Name+"="+c.Value)
	}
	return strings.Join(parts, "; ")
}

func resolveURL(raw, origin string) (string, error) {
	if strings.HasPrefix(raw, "https://") || strings.HasPrefix(raw, "http://") {
		return raw, nil
	}
	o, err := url.Parse(origin)
	if err != nil {
		return "", err
	}
	r, err := o.Parse(raw)
	if err != nil {
		return "", err
	}
	return r.String(), nil
}

func hostOf(origin string) string {
	u, err := url.Parse(origin)
	if err != nil {
		return origin
	}
	return u.Hostname()
}

func pathOf(abs string) string {
	if u, err := url.Parse(abs); err == nil {
		return u.Path
	}
	return abs
}

func argsToString(call goja.FunctionCall) string {
	parts := make([]string, 0, len(call.Arguments))
	for _, a := range call.Arguments {
		parts = append(parts, a.String())
	}
	return strings.Join(parts, " ")
}
