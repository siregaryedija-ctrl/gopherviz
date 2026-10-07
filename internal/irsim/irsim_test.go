package irsim

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopherviz/internal/lang"
)

// oracle mengembalikan perintah untuk menjalankan file dengan interpreter/compiler asli.
type oracle struct {
	lang string
	ext  string
	cmd  func(path, dir string) ([]string, bool)
}

var oracles = []oracle{
	{"javascript", ".js", func(p, dir string) ([]string, bool) { return []string{"node", p}, true }},
	{"python", ".py", func(p, dir string) ([]string, bool) { return []string{"python", p}, true }},
	{"typescript", ".ts", func(p, dir string) ([]string, bool) {
		return []string{"node", "--experimental-transform-types", "--no-warnings", p}, true
	}},
}

func runOracle(t *testing.T, o oracle, path string) (string, bool) {
	t.Helper()
	argv, ok := o.cmd(path, t.TempDir())
	if !ok {
		return "", false
	}
	if _, err := exec.LookPath(argv[0]); err != nil {
		t.Skipf("%s tidak tersedia", argv[0])
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	out, err := cmd.Output()
	if err != nil {
		if _, isExit := err.(*exec.ExitError); !isExit {
			t.Fatalf("%v", err)
		}
	}
	return strings.ReplaceAll(string(out), "\r\n", "\n"), true
}

// TestAgainstReal membandingkan stdout simulasi dengan interpreter asli untuk setiap contoh
// di testdata/<bahasa>/ (kecuali file *.noracle.*, yang output-nya sengaja berbeda).
func TestAgainstReal(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh interpreter asli")
	}
	for _, o := range oracles {
		files, _ := filepath.Glob(filepath.Join("../../testdata", o.lang, "*"+o.ext))
		for _, path := range files {
			if strings.Contains(path, ".noracle.") {
				continue
			}
			t.Run(o.lang+"/"+filepath.Base(path), func(t *testing.T) {
				t.Parallel()
				src, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				res := lang.Analyze(o.lang, filepath.Base(path), string(src))
				got, why := Run(res, Options{Budget: 2_000_000})
				want, ok := runOracle(t, o, path)
				if !ok {
					return
				}
				if why != "" && !strings.HasPrefix(why, "exception") {
					t.Fatalf("simulasi tidak selesai: %s", why)
				}
				if got != want {
					t.Errorf("output berbeda\nsimulator:\n%s\nasli:\n%s", got, want)
				}
			})
		}
	}
}

// FuzzTrace: simulator tidak boleh mengalami kesalahan internal untuk program apa pun.
func FuzzTrace(f *testing.F) {
	for _, l := range []string{"javascript", "typescript", "python"} {
		files, _ := filepath.Glob(filepath.Join("../../testdata", l, "*.*"))
		for _, p := range files {
			if lang.Detect(p) != l {
				continue
			}
			src, _ := os.ReadFile(p)
			f.Add(l, string(src))
		}
	}
	f.Fuzz(func(t *testing.T, language, src string) {
		if !Supported(language) {
			return
		}
		res := lang.Analyze(language, "fuzz", src)
		for _, b := range res.Scene.Blocks {
			tr := Trace(res, b.ID, Options{Budget: 20_000, MaxSteps: 200})
			for _, w := range tr.Warnings {
				if strings.Contains(w, "kesalahan internal") {
					t.Fatalf("%s %s: %s", language, b.ID, w)
				}
			}
		}
		if _, why := Run(res, Options{Budget: 20_000}); strings.Contains(why, "kesalahan internal") {
			t.Fatalf("%s: %s", language, why)
		}
	})
}
