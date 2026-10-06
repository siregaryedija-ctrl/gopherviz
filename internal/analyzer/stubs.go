package analyzer

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"strings"
	"sync"
)

// stubSources berisi tanda tangan (bukan implementasi) package standar yang sering dipakai
// pemula. Dengan ini go/types tahu tipe hasil strings.Fields, sync.WaitGroup, time.Duration,
// dan seterusnya, tanpa membaca GOROOT atau menjalankan `go list` (tetap cepat per ketikan).
// Perilakunya disimulasikan di internal/sim.
var stubSources = map[string]string{
	"fmt": `package fmt
type Stringer interface{ String() string }
func Println(a ...any) (n int, err error)
func Print(a ...any) (n int, err error)
func Printf(format string, a ...any) (n int, err error)
func Sprintf(format string, a ...any) string
func Sprint(a ...any) string
func Sprintln(a ...any) string
func Errorf(format string, a ...any) error
`,
	"errors": `package errors
func New(text string) error
func Is(err, target error) bool
func Unwrap(err error) error
`,
	"strings": `package strings
func Contains(s, substr string) bool
func HasPrefix(s, prefix string) bool
func HasSuffix(s, suffix string) bool
func Index(s, substr string) int
func ToUpper(s string) string
func ToLower(s string) string
func TrimSpace(s string) string
func Split(s, sep string) []string
func Fields(s string) []string
func Join(elems []string, sep string) string
func Repeat(s string, count int) string
func ReplaceAll(s, old, new string) string
`,
	"strconv": `package strconv
func Itoa(i int) string
func Atoi(s string) (int, error)
`,
	"math": `package math
const Pi = 3.14159265358979323846264338327950288419716939937510582097494459
const MaxInt = 1<<63 - 1
const MinInt = -1 << 63
const MaxInt64 = 1<<63 - 1
const MinInt64 = -1 << 63
const MaxInt32 = 1<<31 - 1
func Sqrt(x float64) float64
func Abs(x float64) float64
func Pow(x, y float64) float64
func Floor(x float64) float64
func Ceil(x float64) float64
func Max(x, y float64) float64
func Min(x, y float64) float64
func Mod(x, y float64) float64
`,
	"unicode": `package unicode
func IsUpper(r rune) bool
func IsLower(r rune) bool
func IsDigit(r rune) bool
func IsLetter(r rune) bool
func IsSpace(r rune) bool
func ToUpper(r rune) rune
func ToLower(r rune) rune
`,
	"time": `package time
type Duration int64
const (
	Nanosecond  Duration = 1
	Microsecond          = 1000 * Nanosecond
	Millisecond          = 1000 * Microsecond
	Second               = 1000 * Millisecond
	Minute               = 60 * Second
	Hour                 = 60 * Minute
)
func Sleep(d Duration)
type Time struct{ ns int64 }
func After(d Duration) <-chan Time
`,
	"runtime": `package runtime
func Gosched()
func NumGoroutine() int
`,
	"sync": `package sync
type WaitGroup struct{ n int }
func (wg *WaitGroup) Add(delta int)
func (wg *WaitGroup) Done()
func (wg *WaitGroup) Wait()
type Mutex struct{ locked bool }
func (m *Mutex) Lock()
func (m *Mutex) Unlock()
type RWMutex struct{ locked bool }
func (m *RWMutex) Lock()
func (m *RWMutex) Unlock()
func (m *RWMutex) RLock()
func (m *RWMutex) RUnlock()
type Once struct{ done bool }
func (o *Once) Do(f func())
`,
}

var stubCache sync.Map // path → *types.Package (tanda tangan tidak pernah berubah)

// stubImporter memberi package tiruan dari stubSources, atau package kosong untuk
// package lain (tipenya jadi tidak diketahui, tapi pemeriksaan tetap cepat).
type stubImporter struct{}

func (stubImporter) Import(path string) (*types.Package, error) {
	if p, ok := stubCache.Load(path); ok {
		return p.(*types.Package), nil
	}
	pkg := buildStub(path)
	actual, _ := stubCache.LoadOrStore(path, pkg)
	return actual.(*types.Package), nil
}

func buildStub(path string) *types.Package {
	name := path[strings.LastIndex(path, "/")+1:]
	src, ok := stubSources[path]
	if !ok {
		p := types.NewPackage(path, name)
		p.MarkComplete()
		return p
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path+".go", src, parser.SkipObjectResolution)
	if err != nil {
		panic("stub " + path + " tidak valid: " + err.Error())
	}
	conf := types.Config{Importer: stubImporter{}, Error: func(error) {}}
	pkg, _ := conf.Check(path, fset, []*ast.File{f}, nil)
	pkg.MarkComplete()
	return pkg
}
