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
	if hello.Protocol != 1 || hello.Scene != 1 {
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
		Diagnostics []any  `json:"diagnostics"`
	}
	if err := json.Unmarshal(resps[1].Result, &upd); err != nil {
		t.Fatal(err)
	}
	if len(upd.Scene.Loops) != 2 || upd.FocusLoop != "L2" || upd.Diagnostics == nil {
		t.Errorf("update = %s", resps[1].Result)
	}
	if !strings.Contains(string(resps[1].Result), `"cond":"i < 3"`) {
		t.Errorf("karakter < ter-escape: %s", resps[1].Result)
	}

	if got := string(resps[2].Result); got != `{"focusLoop":"L1"}` {
		t.Errorf("focus baris 4 = %s", got)
	}
	if got := string(resps[3].Result); got != `{}` {
		t.Errorf("focus di luar loop = %s", got)
	}
	if resps[5].Error == nil || !strings.Contains(resps[5].Error.Message, "belum pernah dikirim") {
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
	wants := []string{"bukan JSON", "tidak dikenal", "wajib", "belum pernah dikirim", ""}
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
