package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func runCmd(t *testing.T, stdin string, args ...string) (code int, out, errOut string) {
	t.Helper()
	var o, e bytes.Buffer
	code = run(args, strings.NewReader(stdin), &o, &e)
	return code, o.String(), e.String()
}

func TestList(t *testing.T) {
	code, out, _ := runCmd(t, "", "list", "../../testdata/loops/11_nested_bubble.go")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	for _, want := range []string{"5 blocks (go)", "F1 ", "  L1", "    L2", "8-19", "classic for", "for i := 0; i < n-1; i++",
		"function", "func main()", "if a[j] > a[j+1]", "1 branch", "complete"} {
		if !strings.Contains(out, want) {
			t.Errorf("output tidak memuat %q:\n%s", want, out)
		}
	}
}

func TestListIncomplete(t *testing.T) {
	_, out, _ := runCmd(t, "", "list", "../../testdata/loops/14_incomplete_noclose.go")
	for _, want := range []string{"incomplete", "Parser notes"} {
		if !strings.Contains(out, want) {
			t.Errorf("output tidak memuat %q:\n%s", want, out)
		}
	}
}

func TestListJSONFromStdin(t *testing.T) {
	code, out, _ := runCmd(t, "for i := 0; i < 3; i++ {}\n", "list", "-", "--json")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	var res struct {
		Scene struct {
			Loops []struct {
				Kind string `json:"kind"`
				Body []any  `json:"body"`
			} `json:"loops"`
		} `json:"scene"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("JSON tidak valid: %v\n%s", err, out)
	}
	if l := res.Scene.Loops; len(l) != 1 || l[0].Kind != "for3" || l[0].Body == nil || len(l[0].Body) != 0 {
		t.Errorf("loop = %+v", l)
	}
	if !strings.Contains(out, "i < 3") {
		t.Errorf("JSON meng-escape karakter <:\n%s", out)
	}
}

func TestTraceCmd(t *testing.T) {
	code, out, _ := runCmd(t, "", "trace", "../../testdata/loops/01_for3.go:7")
	for _, want := range []string{"mode simulated", "i < 3 → 0 < 3 → true", "⇒ 2", "condition false"} {
		if code != 0 || !strings.Contains(out, want) {
			t.Errorf("output tidak memuat %q (exit %d):\n%s", want, code, out)
		}
	}
	code, out, _ = runCmd(t, "", "trace", "--json", "../../testdata/loops/01_for3.go:7")
	if code != 0 || !strings.Contains(out, `"mode": "simulated"`) {
		t.Errorf("JSON trace salah (exit %d): %.200s", code, out)
	}
}

func TestServe(t *testing.T) {
	code, out, _ := runCmd(t, `{"id":7,"method":"hello","params":{"client":"t","protocol":2}}`+"\n", "serve")
	if code != 0 || !strings.Contains(out, `"id":7`) || !strings.Contains(out, `"protocol":2`) {
		t.Errorf("exit %d, output %q", code, out)
	}
}

func TestCommands(t *testing.T) {
	tests := []struct {
		args []string
		code int
		want string
	}{
		{[]string{"version"}, 0, "scene v2, trace v2, protocol v2"},
		{[]string{"help"}, 0, "Usage"},
		{[]string{"trace"}, 2, "usage: gopherviz trace"},
		{[]string{"trace", "../../testdata/loops/01_for3.go:2"}, 1, "no block"},
		{[]string{"trace", "../../testdata/blocks/03_recursion.go:12"}, 0, "fib(n = 4) returns 3"},
		{[]string{"show"}, 2, "after Phase 9"},
		{[]string{"terbang"}, 2, "unknown command"},
		{[]string{"list"}, 2, "usage"},
		{[]string{"list", "--warna", "a.go"}, 2, "unknown flag"},
		{[]string{"list", "tidak-ada.go"}, 1, "failed to read"},
		{nil, 2, "Usage"},
	}
	for _, tt := range tests {
		code, out, errOut := runCmd(t, "", tt.args...)
		if code != tt.code || !strings.Contains(out+errOut, tt.want) {
			t.Errorf("%v: exit %d, output %q; mau exit %d dengan %q", tt.args, code, out+errOut, tt.code, tt.want)
		}
	}
}
