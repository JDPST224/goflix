package cinesrcjs

// Regression test for the URLSearchParams shim: values from a plain object
// initializer must be stringified, not silently dropped (gojaValueToString
// used to return "" for every non-string).

import (
	"strings"
	"testing"

	"github.com/dop251/goja"
)

func TestURLSearchParamsObjectInitStringifiesValues(t *testing.T) {
	vm := goja.New()
	installWebAPI(vm, vm.GlobalObject())

	v, err := vm.RunString(`new URLSearchParams({page: 2, q: "a b", flag: true}).toString()`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	got := v.String()
	for _, want := range []string{"page=2", "q=a+b", "flag=true"} {
		if !strings.Contains(got, want) {
			t.Errorf("URLSearchParams string = %q, missing %q", got, want)
		}
	}
}
