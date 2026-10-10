package irsim

import (
	"runtime"
	"strings"
	"testing"
	"time"

	"gopherviz/internal/lang"
	"gopherviz/internal/scene"
	"gopherviz/internal/trace"
)

// TestEventLoop: program JS async membawa potret event loop di setiap langkah: call stack, antrian
// microtask (then, lanjutan await), dan antrian timer; callback yang dijadwalkan dari blok fokus
// ikut direkam setelah fungsi fokus selesai, dengan urutan seperti JS asli.
func TestEventLoop(t *testing.T) {
	src := `async function ambil(nama) {
  console.log("mulai " + nama);
  await null;
  console.log("lanjut " + nama);
  return nama.length;
}

function main() {
  ambil("A").then(n => console.log("panjang " + n));
  setTimeout(() => console.log("timer"), 0);
  Promise.resolve().then(() => console.log("promise"));
  console.log("sinkron selesai");
}
main();
`
	res := lang.Analyze("javascript", "a.js", src)
	id := ""
	for _, b := range res.Scene.Blocks {
		if b.Kind == scene.BlockFunc && b.Name == "main" {
			id = b.ID
		}
	}
	tr := Trace(res, id, Options{})
	if tr.Mode != trace.ModeSimulated {
		t.Fatalf("mode %s %v", tr.Mode, tr.Warnings)
	}
	var out strings.Builder
	sawMicro, sawMacro, sawPick := false, false, false
	for _, s := range tr.Steps {
		out.WriteString(s.Output)
		if s.EvLoop == nil {
			t.Fatalf("langkah tanpa potret event loop: %s", s.Note)
		}
		sawMicro = sawMicro || len(s.EvLoop.Micro) > 0
		sawMacro = sawMacro || len(s.EvLoop.Macro) > 0
		sawPick = sawPick || s.EvLoop.Task != "" && len(s.EvLoop.Stack) == 0
	}
	if want := "mulai A\nsinkron selesai\nlanjut A\npromise\npanjang 1\ntimer\n"; out.String() != want {
		t.Errorf("output %q, mau %q", out.String(), want)
	}
	if !sawMicro || !sawMacro || !sawPick {
		t.Errorf("micro %v macro %v ambil-tiket %v", sawMicro, sawMacro, sawPick)
	}
	if s := tr.Steps[0]; len(s.EvLoop.Stack) != 1 || s.EvLoop.Stack[0] != "main" {
		t.Errorf("call stack langkah pertama: %v", s.EvLoop.Stack)
	}
}

// TestAsyncNoLeak: fungsi async yang menunggu promise yang tidak pernah selesai dihentikan saat
// program selesai (goroutine coroutine-nya tidak tertinggal).
func TestAsyncNoLeak(t *testing.T) {
	src := "async function macet() {\n  await new Promise(() => {});\n  console.log('tidak pernah');\n}\nfunction main() {\n  for (let i = 0; i < 20; i++) macet();\n  console.log('selesai');\n}\nmain();\n"
	res := lang.Analyze("javascript", "a.js", src)
	before := runtime.NumGoroutine()
	for k := 0; k < 5; k++ {
		out, why := Run(res, Options{})
		if out != "selesai\n" || why != "" {
			t.Fatalf("output %q, error %q", out, why)
		}
		tr := Trace(res, res.Scene.Blocks[len(res.Scene.Blocks)-1].ID, Options{})
		_ = tr
	}
	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > before+2 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if n := runtime.NumGoroutine(); n > before+2 {
		t.Fatalf("goroutine tertinggal: %d sebelum, %d sesudah", before, n)
	}
}

// TestPopOps: elemen yang keluar dari depan (antrian) atau belakang (tumpukan) koleksi menjadi
// op "pop" dengan indeks dan nilainya, di semua bahasa.
func TestPopOps(t *testing.T) {
	cases := []struct {
		lang, file, src string
		front           bool
	}{
		{"python", "s.py", "def f():\n    s = []\n    s.append(1)\n    s.append(2)\n    x = s.pop()\n    return x\n\nf()\n", false},
		{"python", "q.py", "from collections import deque\n\ndef f():\n    q = deque()\n    q.append(1)\n    q.append(2)\n    x = q.popleft()\n    return x\n\nf()\n", true},
		{"javascript", "q.js", "function f() {\n  const q = [];\n  q.push(1);\n  q.push(2);\n  return q.shift();\n}\nf();\n", true},
		{"java", "S.java", "import java.util.*;\npublic class S {\n    static int f() {\n        Deque<Integer> s = new ArrayDeque<>();\n        s.push(1);\n        s.push(2);\n        return s.pop();\n    }\n    public static void main(String[] a) { f(); }\n}\n", true},
		{"cpp", "q.cpp", "#include <queue>\nint f() {\n    std::queue<int> q;\n    q.push(1);\n    q.push(2);\n    q.pop();\n    return q.front();\n}\nint main() { return f(); }\n", true},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			res := lang.Analyze(c.lang, c.file, c.src)
			id := ""
			for _, b := range res.Scene.Blocks {
				if b.Kind == scene.BlockFunc && (b.Name == "f" || strings.HasSuffix(b.Name, ".f")) {
					id = b.ID
				}
			}
			tr := Trace(res, id, Options{})
			var pops []trace.Op
			for _, s := range tr.Steps {
				for _, o := range s.Ops {
					if o.Op == trace.OpPop {
						pops = append(pops, o)
					}
				}
			}
			if len(pops) != 1 {
				t.Fatalf("pop %v (mode %s %v)", pops, tr.Mode, tr.Warnings)
			}
			if p := pops[0]; (p.A == 0) != c.front || p.Value == "" {
				t.Fatalf("pop %+v, depan %v", p, c.front)
			}
		})
	}
}
