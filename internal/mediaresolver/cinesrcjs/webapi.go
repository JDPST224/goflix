package cinesrcjs

// webapi.go â€” minimal shims for the standard Web APIs the challenge scripts
// construct (URLSearchParams et al). Browsers provide these; goja does not,
// and the challenge's `new` opcode then fails with "constructor undefined".

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/dop251/goja"
)

func mustInstall(vm *goja.Runtime, err error) {
	if err != nil {
		panic(err)
	}
}

// installWebAPI shims the Web APIs the challenge scripts use.
func installWebAPI(vm *goja.Runtime, g *goja.Object) {
	// URLSearchParams: full query-string semantics.
	mustInstall(vm, g.Set("URLSearchParams", func(call goja.ConstructorCall) *goja.Object {
		this := call.This
		pairs := parseUSPInit(vm, call.Argument(0))
		_ = this.Set("__pairs", pairsToValue(vm, pairs))
		_ = this.Set("append", func(c goja.FunctionCall) goja.Value {
			pairs = append(pairs, uspPair{c.Argument(0).String(), c.Argument(1).String()})
			_ = this.Set("__pairs", pairsToValue(vm, pairs))
			return goja.Undefined()
		})
		_ = this.Set("delete", func(c goja.FunctionCall) goja.Value {
			k := c.Argument(0).String()
			out := pairs[:0]
			for _, p := range pairs {
				if p[0] != k {
					out = append(out, p)
				}
			}
			pairs = out
			_ = this.Set("__pairs", pairsToValue(vm, pairs))
			return goja.Undefined()
		})
		_ = this.Set("get", func(c goja.FunctionCall) goja.Value {
			k := c.Argument(0).String()
			for _, p := range pairs {
				if p[0] == k {
					return vm.ToValue(p[1])
				}
			}
			return goja.Null()
		})
		_ = this.Set("getAll", func(c goja.FunctionCall) goja.Value {
			k := c.Argument(0).String()
			arr := vm.NewArray()
			n := 0
			for _, p := range pairs {
				if p[0] == k {
					_ = arr.Set(strconv.Itoa(n), vm.ToValue(p[1]))
					n++
				}
			}
			return arr
		})
		_ = this.Set("has", func(c goja.FunctionCall) goja.Value {
			k := c.Argument(0).String()
			for _, p := range pairs {
				if p[0] == k {
					return vm.ToValue(true)
				}
			}
			return vm.ToValue(false)
		})
		_ = this.Set("set", func(c goja.FunctionCall) goja.Value {
			k, v := c.Argument(0).String(), c.Argument(1).String()
			found := false
			out := pairs[:0]
			for _, p := range pairs {
				if p[0] == k {
					if !found {
						out = append(out, uspPair{k, v})
						found = true
					}
					continue
				}
				out = append(out, p)
			}
			if !found {
				out = append(out, uspPair{k, v})
			}
			pairs = out
			_ = this.Set("__pairs", pairsToValue(vm, pairs))
			return goja.Undefined()
		})
		_ = this.Set("sort", func(c goja.FunctionCall) goja.Value { return goja.Undefined() })
		_ = this.Set("forEach", func(c goja.FunctionCall) goja.Value {
			if fn, ok := goja.AssertFunction(c.Argument(0)); ok {
				for _, p := range pairs {
					_, _ = fn(goja.Undefined(), vm.ToValue(p[1]), vm.ToValue(p[0]), this)
				}
			}
			return goja.Undefined()
		})
		_ = this.Set("toString", func(c goja.FunctionCall) goja.Value {
			return vm.ToValue(uspSerialize(pairs))
		})
		_ = this.Set("entries", func(c goja.FunctionCall) goja.Value {
			arr := vm.NewArray()
			for i, p := range pairs {
				_ = arr.Set(strconv.Itoa(i), pairsToValue(vm, []uspPair{p}))
			}
			return arr
		})
		_ = this.Set("keys", func(c goja.FunctionCall) goja.Value {
			arr := vm.NewArray()
			for i, p := range pairs {
				_ = arr.Set(strconv.Itoa(i), vm.ToValue(p[0]))
			}
			return arr
		})
		_ = this.Set("values", func(c goja.FunctionCall) goja.Value {
			arr := vm.NewArray()
			for i, p := range pairs {
				_ = arr.Set(strconv.Itoa(i), vm.ToValue(p[1]))
			}
			return arr
		})
		_ = this.Set("toString", func(c goja.FunctionCall) goja.Value {
			return vm.ToValue(uspSerialize(pairs))
		})
		return nil
	}))
	usp := g.Get("URLSearchParams").ToObject(vm)
	mustInstall(vm, usp.Set("toString", func(c goja.FunctionCall) goja.Value { return vm.ToValue("") }))

	// Headers: case-insensitive multi-map.
	installHeaders(vm, g)

	// AbortController / AbortSignal stubs.
	mustInstall(vm, g.Set("AbortController", func(call goja.ConstructorCall) *goja.Object {
		signal := vm.NewObject()
		_ = signal.Set("aborted", false)
		_ = signal.Set("reason", goja.Undefined())
		_ = signal.Set("addEventListener", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
		_ = signal.Set("removeEventListener", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
		_ = signal.Set("throwIfAborted", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
		_ = call.This.Set("signal", signal)
		_ = call.This.Set("abort", func(c goja.FunctionCall) goja.Value {
			_ = signal.Set("aborted", true)
			if r := c.Argument(0); r != nil && !goja.IsUndefined(r) {
				_ = signal.Set("reason", r)
			}
			return goja.Undefined()
		})
		return nil
	}))

	// Fetch-adjacent classes: existence plus the methods the challenge
	// touches generically.
	_ = g.Set("Response", func(call goja.ConstructorCall) *goja.Object {
		_ = call.This.Set("__body", call.Argument(0))
		_ = call.This.Set("ok", true)
		_ = call.This.Set("status", 200)
		hdr := vm.NewObject()
		_ = hdr.Set("get", func(goja.FunctionCall) goja.Value { return goja.Null() })
		_ = hdr.Set("has", func(goja.FunctionCall) goja.Value { return vm.ToValue(false) })
		_ = call.This.Set("headers", hdr)
		_ = call.This.Set("text", func(goja.FunctionCall) goja.Value {
			pr, res, _ := vm.NewPromise()
			if s, ok := call.This.Get("__body").Export().(string); ok {
				_ = res(vm.ToValue(s))
			} else {
				_ = res(vm.ToValue(""))
			}
			return vm.ToValue(pr)
		})
		_ = call.This.Set("json", func(goja.FunctionCall) goja.Value {
			pr, res, rej := vm.NewPromise()
			if s, ok := call.This.Get("__body").Export().(string); ok {
				if parsed, err := vm.RunString("(" + s + ")"); err == nil {
					_ = res(parsed)
				} else {
					_ = rej(vm.NewGoError(err))
				}
			} else {
				_ = rej(vm.NewGoError(nil))
			}
			return vm.ToValue(pr)
		})
		return nil
	})
	_ = g.Set("Request", func(call goja.ConstructorCall) *goja.Object {
		_ = call.This.Set("url", call.Argument(0).String())
		_ = call.This.Set("method", "GET")
		return nil
	})
	_ = g.Set("FormData", func(call goja.ConstructorCall) *goja.Object {
		_ = call.This.Set("append", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
		_ = call.This.Set("get", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
		return nil
	})
	_ = g.Set("EventTarget", func(call goja.ConstructorCall) *goja.Object {
		_ = call.This.Set("addEventListener", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
		_ = call.This.Set("removeEventListener", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
		_ = call.This.Set("dispatchEvent", func(goja.FunctionCall) goja.Value { return vm.ToValue(true) })
		return nil
	})
	_ = g.Set("MessageChannel", func(call goja.ConstructorCall) *goja.Object {
		mkPort := func() *goja.Object {
			p := vm.NewObject()
			_ = p.Set("postMessage", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
			_ = p.Set("start", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
			_ = p.Set("close", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
			_ = p.Set("onmessage", goja.Undefined())
			return p
		}
		_ = call.This.Set("port1", mkPort())
		_ = call.This.Set("port2", mkPort())
		return nil
	})
	_ = g.Set("DOMException", func(call goja.ConstructorCall) *goja.Object {
		_ = call.This.Set("message", call.Argument(0).String())
		_ = call.This.Set("name", call.Argument(1).String())
		return nil
	})
}

type uspPair [2]string

func parseUSPInit(vm *goja.Runtime, v goja.Value) []uspPair {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return nil
	}
	switch t := v.Export().(type) {
	case string:
		return uspParseString(t)
	case map[string]interface{}:
		var out []uspPair
		for k, val := range t {
			out = append(out, uspPair{k, gojaValueToString(val)})
		}
		return out
	}
	if obj, ok := v.(*goja.Object); ok {
		if l := obj.Get("length"); l != nil && !goja.IsUndefined(l) {
			// array of [k, v] pairs
			n := int(l.ToInteger())
			var out []uspPair
			for i := 0; i < n; i++ {
				p := obj.Get(strconv.Itoa(i))
				if po, ok := p.(*goja.Object); ok {
					out = append(out, uspPair{po.Get("0").String(), po.Get("1").String()})
				}
			}
			return out
		}
		// object literal
		var out []uspPair
		for _, k := range obj.Keys() {
			out = append(out, uspPair{k, obj.Get(k).String()})
		}
		return out
	}
	return uspParseString(v.String())
}

func gojaValueToString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

func uspParseString(s string) []uspPair {
	if strings.HasPrefix(s, "?") {
		s = s[1:]
	}
	var out []uspPair
	for _, part := range strings.Split(s, "&") {
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		out = append(out, uspPair{urlDecode(kv[0]), urlDecode(uspOrEmpty(kv, 1))})
	}
	return out
}

func uspOrEmpty(kv []string, i int) string {
	if i < len(kv) {
		return kv[i]
	}
	return ""
}

func urlDecode(s string) string {
	s = strings.ReplaceAll(s, "+", " ")
	// minimal percent-decode
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			if v := hexVal(s[i+1])*16 + hexVal(s[i+2]); v >= 0 {
				b.WriteByte(byte(v))
				i += 2
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func hexVal(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return 0
}

func uspSerialize(pairs []uspPair) string {
	var b strings.Builder
	for i, p := range pairs {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(urlEncode(p[0]))
		b.WriteByte('=')
		b.WriteString(urlEncode(p[1]))
	}
	return b.String()
}

func urlEncode(s string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.' || c == '~' || c == '*' || c == '!' || c == '\'' ||
			c == '(' || c == ')' {
			b.WriteByte(c)
			continue
		}
		if c == ' ' {
			b.WriteByte('+')
			continue
		}
		b.WriteByte('%')
		b.WriteByte(hex[c>>4])
		b.WriteByte(hex[c&0xF])
	}
	return b.String()
}

func pairsToValue(vm *goja.Runtime, pairs []uspPair) goja.Value {
	arr := vm.NewArray()
	for i, p := range pairs {
		pv := vm.NewArray()
		_ = pv.Set("0", vm.ToValue(p[0]))
		_ = pv.Set("1", vm.ToValue(p[1]))
		_ = arr.Set(strconv.Itoa(i), pv)
	}
	return arr
}

// installHeaders adds the Headers class.
func installHeaders(vm *goja.Runtime, g *goja.Object) {
	mustInstall(vm, g.Set("Headers", func(call goja.ConstructorCall) *goja.Object {
		this := call.This
		store := map[string]string{}
		if obj, ok := call.Argument(0).(*goja.Object); ok {
			// Seed from a plain object literal (browser-supported form).
			if l := obj.Get("length"); l == nil || goja.IsUndefined(l) {
				for _, k := range obj.Keys() {
					if k == "__store" {
						continue
					}
					store[strings.ToLower(k)] = obj.Get(k).String()
				}
			}
		}
		_ = this.Set("append", func(c goja.FunctionCall) goja.Value {
			store[strings.ToLower(c.Argument(0).String())] = c.Argument(1).String()
			return goja.Undefined()
		})
		_ = this.Set("set", func(c goja.FunctionCall) goja.Value {
			store[strings.ToLower(c.Argument(0).String())] = c.Argument(1).String()
			return goja.Undefined()
		})
		_ = this.Set("get", func(c goja.FunctionCall) goja.Value {
			if v, ok := store[strings.ToLower(c.Argument(0).String())]; ok {
				return vm.ToValue(v)
			}
			return goja.Null()
		})
		_ = this.Set("has", func(c goja.FunctionCall) goja.Value {
			_, ok := store[strings.ToLower(c.Argument(0).String())]
			return vm.ToValue(ok)
		})
		_ = this.Set("delete", func(c goja.FunctionCall) goja.Value {
			delete(store, strings.ToLower(c.Argument(0).String()))
			return goja.Undefined()
		})
		_ = this.Set("forEach", func(c goja.FunctionCall) goja.Value {
			if fn, ok := goja.AssertFunction(c.Argument(0)); ok {
				for k, v := range store {
					_, _ = fn(goja.Undefined(), vm.ToValue(v), vm.ToValue(k), this)
				}
			}
			return goja.Undefined()
		})
		return nil
	}))
}
