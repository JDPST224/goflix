package cinesrcjs

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/dop251/goja"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

var (
	base64Std    = base64.StdEncoding
	base64RawStd = base64.RawStdEncoding
	base64URL    = base64.URLEncoding
	base64RawURL = base64.RawURLEncoding
)

// powRuntime executes pow-v3.wasm (exports: memory, a(len)->ptr,
// b(ptr,len)->ptr). The challenge is the 32-byte decoded CSP3 blob; the
// result is a NUL-terminated proof string ("m3.<hex>") in linear memory.
type powRuntime struct {
	mu  sync.Mutex
	mod api.Module
	mem api.Memory
	fnA api.Function
	fnB api.Function
}

func newPowRuntime(wasmBytes []byte) (*powRuntime, error) {
	ctx := context.Background()
	eng := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfig().WithCloseOnContextDone(true))
	compiled, err := eng.CompileModule(ctx, wasmBytes)
	if err != nil {
		return nil, fmt.Errorf("pow wasm compile: %w", err)
	}
	mod, err := eng.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().WithName("cinesrc-pow"))
	if err != nil {
		return nil, fmt.Errorf("pow wasm instantiate: %w", err)
	}
	fnA := mod.ExportedFunction("a")
	fnB := mod.ExportedFunction("b")
	mem := mod.ExportedMemory("memory")
	if fnA == nil || fnB == nil || mem == nil {
		return nil, errors.New("pow wasm missing exports")
	}
	return &powRuntime{mod: mod, mem: mem, fnA: fnA, fnB: fnB}, nil
}

// solve runs the challenge blob and returns the proof string.
func (p *powRuntime) solve(challenge []byte) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	ctx := context.Background()
	ptrA, err := p.fnA.Call(ctx, uint64(len(challenge)))
	if err != nil {
		return "", fmt.Errorf("pow alloc: %w", err)
	}
	bufPtr := uint32(ptrA[0])
	if !p.mem.Write(bufPtr, challenge) {
		return "", errors.New("pow write failed")
	}
	ptrB, err := p.fnB.Call(ctx, uint64(bufPtr), uint64(len(challenge)))
	if err != nil {
		return "", fmt.Errorf("pow solve: %w", err)
	}
	resPtr := uint32(ptrB[0])
	out := make([]byte, 0, 64)
	for i := 0; i < 16; i++ {
		data, ok := p.mem.Read(resPtr+uint32(i*64), 64)
		if !ok {
			return "", errors.New("pow read failed")
		}
		done := false
		for _, c := range data {
			if c == 0 {
				done = true
				break
			}
			out = append(out, c)
		}
		if done {
			break
		}
	}
	return string(out), nil
}

// solvePackPoW mirrors donut.js's blob workers: find a counter in [start,end]
// whose hex form (zero-padded to ceil(difficulty/4) nibbles) appended to the
// salt hashes (SHA-256) exactly to the target. Returns the solution string.
func solvePackPoW(salt, target string, difficulty, start, end int) (string, bool) {
	width := (difficulty + 3) / 4
	target = strings.ToLower(target)
	for i := start; i <= end; i++ {
		suffix := fmt.Sprintf("%0*x", width, i)
		sum := sha256.Sum256([]byte(salt + suffix))
		if hex.EncodeToString(sum[:]) == target {
			return suffix, true
		}
	}
	return "", false
}

// installWorker registers the Worker constructor shim. Two call shapes exist:
//   - the challenge module posts {id, work:"CSP3..."} and expects
//     {id, ok, proof} — the pow-worker-v3.js RPC, solved natively via wazero;
//   - the donut pack posts [salt, target, difficulty, start, end] and expects
//     {solution} — the blob worker, solved natively in Go.
//
// Responses are delivered on the VM goroutine via the drain() queue.
func (rt *runtime) installWorker(g *goja.Object) {
	vm := rt.vm

	must(vm, g.Set("Worker", func(call goja.ConstructorCall) *goja.Object {
		this := call.This
		var msgHandler goja.Value

		deliver := func(data map[string]interface{}) {
			rt.mu.Lock()
			rt.workerJobs = append(rt.workerJobs, func() {
				h := this.Get("onmessage")
				fn, ok := goja.AssertFunction(h)
				if !ok && msgHandler != nil {
					fn, ok = goja.AssertFunction(msgHandler)
				}
				if ok {
					ev := vm.NewObject()
					_ = ev.Set("data", data)
					if _, err := fn(goja.Undefined(), ev); err != nil {
						rt.dbg("worker onmessage error: " + err.Error())
					}
				} else {
					rt.dbg("worker deliver: no onmessage handler")
				}
			})
			rt.mu.Unlock()
		}

		_ = this.Set("onmessage", goja.Undefined())
		_ = this.Set("addEventListener", func(c goja.FunctionCall) goja.Value {
			if c.Argument(0).String() == "message" {
				msgHandler = c.Argument(1)
			}
			return goja.Undefined()
		})
		_ = this.Set("removeEventListener", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
		_ = this.Set("terminate", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
		_ = this.Set("postMessage", func(c goja.FunctionCall) goja.Value {
			data := c.Argument(0)

			// donut pack worker: [salt, target, difficulty, start, end]
			if arr, ok := data.(*goja.Object); ok && arr.Get("length") != nil {
				if n := int(arr.Get("length").ToInteger()); n == 5 {
					salt := arr.Get("0").String()
					target := arr.Get("1").String()
					difficulty := int(arr.Get("2").ToInteger())
					start := int(arr.Get("3").ToInteger())
					end := int(arr.Get("4").ToInteger())
					solution, found := solvePackPoW(salt, target, difficulty, start, end)
					if found {
						deliver(map[string]interface{}{"solution": solution})
					} else {
						deliver(map[string]interface{}{"solution": nil})
					}
					return goja.Undefined()
				}
			}

			// pow worker RPC: {id, work}
			if obj, ok := data.(*goja.Object); ok {
				idV := obj.Get("id")
				workV := obj.Get("work")
				if idV != nil && workV != nil {
					id := idV.Export()
					work := workV.String()
					var proof string
					var ok bool
					if rt.getPow != nil {
						if pow, perr := rt.getPow(); perr == nil && pow != nil {
							if raw, derr := decodeBase64Any(work); derr == nil {
								if proof, derr = pow.solve(raw); derr == nil && strings.HasPrefix(proof, "m3.") {
									ok = true
								}
							}
						}
					}
					if ok {
						deliver(map[string]interface{}{"id": id, "ok": true, "proof": proof})
					} else {
						deliver(map[string]interface{}{"id": id, "ok": false, "error": "pow unavailable"})
					}
					return goja.Undefined()
				}
			}
			return goja.Undefined()
		})
		return nil
	}))
}

// decodeBase64Any decodes standard or URL-safe base64 (with or without
// padding), which is how the challenge blobs are encoded.
func decodeBase64Any(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if b, err := base64Std.DecodeString(s); err == nil {
		return b, nil
	}
	if b, err := base64RawStd.DecodeString(s); err == nil {
		return b, nil
	}
	if b, err := base64URL.DecodeString(s); err == nil {
		return b, nil
	}
	if b, err := base64RawURL.DecodeString(s); err == nil {
		return b, nil
	}
	return nil, errors.New("not base64")
}
