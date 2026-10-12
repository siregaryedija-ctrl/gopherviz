package serve

import (
	"bufio"
	"encoding/json"
	"strings"
	"testing"
)

type response struct {
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// session mengirim request (satu per baris) dan mengembalikan semua response.
func session(t *testing.T, requests ...string) []response {
	t.Helper()
	var out strings.Builder
	if err := New("test").Run(strings.NewReader(strings.Join(requests, "\n")+"\n"), &out); err != nil {
		t.Fatal(err)
	}
	var resps []response
	sc := bufio.NewScanner(strings.NewReader(out.String()))
	for sc.Scan() {
		var r response
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			t.Fatalf("response bukan JSON: %q", sc.Text())
		}
		resps = append(resps, r)
	}
	return resps
}

func update(id int, text string, line int) string {
	b, _ := json.Marshal(map[string]any{
		"id": id, "method": "update",
		"params": map[string]any{"file": "main.go", "text": text, "cursor": map[string]int{"line": line, "col": 1}},
	})
	return string(b)
}

const bubble = "package main\n\nfunc main() {\n\tfor i := 0; i < 3; i++ {\n\t\tfor j := 0; j < i; j++ {\n\t\t}\n\t}\n}\n"

func TestSession(t *testing.T) {
	resps := session(t,
		`{"id":1,"method":"hello","params":{"client":"test","protocol":1}}`,
		update(2, bubble, 5),
		`{"id":3,"method":"focus","params":{"file":"main.go","cursor":{"line":4,"col":1}}}`,
		`{"id":4,"method":"focus","params":{"file":"main.go","cursor":{"line":1,"col":1}}}`,
		`{"id":5,"method":"close","params":{"file":"main.go"}}`,
		`{"id":6,"method":"focus","params":{"file":"main.go","cursor":{"line":4,"col":1}}}`,
	)
	if len(resps) != 6 {
		t.Fatalf("%d response, mau 6", len(resps))
	}
	for i, r := range resps {
		if r.ID != i+1 {
			t.Errorf("response ke-%d punya id %d", i, r.ID)
		}
	}

	var hello struct {
		Protocol int `json:"protocol"`
		Scene    int `json:"scene"`
	}
	_ = json.Unmarshal(resps[0].Result, &hello)
	if hello.Protocol != 2 || hello.Scene != 2 {
		t.Errorf("hello = %s", resps[0].Result)
	}

	var upd struct {
		Scene struct {
			Loops []struct {
				ID   string `json:"id"`
				Body []any  `json:"body"`
			} `json:"loops"`
		} `json:"scene"`
		FocusLoop   string `json:"focusLoop"`
		FocusBlock  string `json:"focusBlock"`
		Diagnostics []any  `json:"diagnostics"`
	}
	if err := json.Unmarshal(resps[1].Result, &upd); err != nil {
		t.Fatal(err)
	}
	if len(upd.Scene.Loops) != 2 || upd.FocusLoop != "L2" || upd.FocusBlock != "L2" || upd.Diagnostics == nil {
		t.Errorf("update = %s", resps[1].Result)
	}
	if !strings.Contains(string(resps[1].Result), `"cond":"i < 3"`) {
		t.Errorf("karakter < ter-escape: %s", resps[1].Result)
	}

	if got := string(resps[2].Result); got != `{"focusLoop":"L1","focusBlock":"L1"}` {
		t.Errorf("focus baris 4 = %s", got)
	}
	if got := string(resps[3].Result); got != `{}` {
		t.Errorf("focus di luar blok = %s", got)
	}
	if resps[5].Error == nil || !strings.Contains(resps[5].Error.Message, "never sent") {
		t.Errorf("focus setelah close harus error: %+v", resps[5])
	}
}

func TestErrors(t *testing.T) {
	resps := session(t,
		`bukan json`,
		`{"id":2,"method":"terbang"}`,
		`{"id":3,"method":"update"}`,
		`{"id":4,"method":"trace","params":{"file":"main.go","loopId":"L1"}}`,
		update(5, "for {", 1),
	)
	wants := []string{"not valid JSON", "unknown method", "required", "never sent", ""}
	for i, want := range wants {
		r := resps[i]
		switch {
		case want == "" && r.Error != nil:
			t.Errorf("request %d: error tak terduga %q", i+1, r.Error.Message)
		case want != "" && (r.Error == nil || !strings.Contains(r.Error.Message, want)):
			t.Errorf("request %d: error = %+v, mau memuat %q", i+1, r.Error, want)
		}
	}
}

func TestTrace(t *testing.T) {
	src := "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfor i := 0; i < 2; i++ {\n\t\tfmt.Println(i)\n\t}\n}\n"
	resps := session(t, update(1, src, 6), `{"id":2,"method":"trace","params":{"file":"main.go","loopId":"L1"}}`)
	var res struct {
		Trace struct {
			Mode  string `json:"mode"`
			Steps []struct {
				Output string `json:"output"`
			} `json:"steps"`
		} `json:"trace"`
	}
	if err := json.Unmarshal(resps[1].Result, &res); err != nil {
		t.Fatalf("%v: %s", err, resps[1].Result)
	}
	var out strings.Builder
	for _, s := range res.Trace.Steps {
		out.WriteString(s.Output)
	}
	if res.Trace.Mode != "simulated" || out.String() != "0\n1\n" {
		t.Errorf("mode %s, output %q", res.Trace.Mode, out.String())
	}
}

func TestCRLF(t *testing.T) {
	resps := session(t, update(1, strings.ReplaceAll(bubble, "\n", "\r\n"), 4))
	if !strings.Contains(string(resps[0].Result), `"text":"for i := 0; i < 3; i++"`) {
		t.Errorf("CRLF tidak dinormalisasi: %s", resps[0].Result)
	}
}

// TestOtherLanguage: file bahasa lain dianalisis dengan parsernya sendiri; languageId
// editor menang atas ekstensi (mis. dokumen untitled).
func TestOtherLanguage(t *testing.T) {
	req := func(id int, file, language, text string, line int) string {
		b, _ := json.Marshal(map[string]any{
			"id": id, "method": "update",
			"params": map[string]any{"file": file, "language": language, "text": text, "cursor": map[string]int{"line": line, "col": 1}},
		})
		return string(b)
	}
	js := "function f(n) {\n  for (let i = 0; i < n; i++) {\n    console.log(i)\n  }\n}\nf(2)\n"
	py := "for i in range(3):\n    print(i)\n"
	resps := session(t,
		req(1, "a.js", "", js, 3),
		req(2, "Untitled-1", "python", py, 2),
		`{"id":3,"method":"trace","params":{"file":"a.js","blockId":"L1"}}`,
	)
	var up struct {
		Scene struct {
			Language string `json:"language"`
		} `json:"scene"`
		FocusBlock string `json:"focusBlock"`
	}
	for i, want := range []string{"javascript", "python"} {
		if err := json.Unmarshal(resps[i].Result, &up); err != nil {
			t.Fatal(err)
		}
		if up.Scene.Language != want || up.FocusBlock != "L1" {
			t.Errorf("update %d: bahasa %s fokus %s", i+1, up.Scene.Language, up.FocusBlock)
		}
	}
	var tr struct {
		Trace struct {
			Mode     string   `json:"mode"`
			Warnings []string `json:"warnings"`
		} `json:"trace"`
	}
	if err := json.Unmarshal(resps[2].Result, &tr); err != nil {
		t.Fatal(err)
	}
	if tr.Trace.Mode != "simulated" { // JavaScript disimulasikan sejak Fase 9a
		t.Errorf("trace %+v", tr.Trace)
	}
}

// TestRace: dua fungsi sort dilombakan dengan list acak yang sama; bubble sort butuh operasi jauh
// lebih banyak daripada merge sort untuk n = 40.
func TestRace(t *testing.T) {
	src := `package main

func bubble(a []int) {
	for i := range a {
		for j := 0; j+1 < len(a)-i; j++ {
			if a[j] > a[j+1] {
				a[j], a[j+1] = a[j+1], a[j]
			}
		}
	}
}

func merge(a []int) []int {
	if len(a) <= 1 {
		return a
	}
	m := len(a) / 2
	l, r := merge(a[:m]), merge(a[m:])
	out := make([]int, 0, len(a))
	i, j := 0, 0
	for i < len(l) && j < len(r) {
		if l[i] <= r[j] {
			out = append(out, l[i])
			i++
		} else {
			out = append(out, r[j])
			j++
		}
	}
	out = append(out, l[i:]...)
	return append(out, r[j:]...)
}

func main() {}
`
	resps := session(t, update(1, src, 3), `{"id":2,"method":"race","params":{"file":"main.go","blocks":["F1","F2"],"n":6,"seed":4}}`)
	var res struct {
		Input []int `json:"input"`
		Sizes []int `json:"sizes"`
		Runs  []struct {
			Block string `json:"block"`
			Trace struct {
				Mode  string `json:"mode"`
				Steps []any  `json:"steps"`
			} `json:"trace"`
			Work []int `json:"work"`
		} `json:"runs"`
	}
	if resps[1].Error != nil {
		t.Fatal(resps[1].Error.Message)
	}
	if err := json.Unmarshal(resps[1].Result, &res); err != nil {
		t.Fatalf("%v: %s", err, resps[1].Result)
	}
	if len(res.Input) != 6 || len(res.Runs) != 2 || len(res.Sizes) != 4 {
		t.Fatalf("hasil %s", resps[1].Result)
	}
	for _, r := range res.Runs {
		if r.Trace.Mode != "simulated" || len(r.Trace.Steps) == 0 || len(r.Work) != 4 || r.Work[0] <= 0 {
			t.Fatalf("%s: mode %s langkah %d operasi %v", r.Block, r.Trace.Mode, len(r.Trace.Steps), r.Work)
		}
	}
	if b, m := res.Runs[0].Work[3], res.Runs[1].Work[3]; b <= m {
		t.Errorf("n=40: bubble %d operasi, merge %d", b, m)
	}
}
