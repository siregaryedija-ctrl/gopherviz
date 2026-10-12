// Command gopherviz menampilkan cara kerja loop Go secara visual.
//
// Fase 1 baru menyediakan `list` dan `version`; perintah lain menyusul sesuai
// roadmap di CLAUDE.md.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"unicode/utf8"

	"gopherviz/internal/analyzer"
	"gopherviz/internal/ir"
	"gopherviz/internal/lang"
	"gopherviz/internal/project"
	"gopherviz/internal/protocol"
	"gopherviz/internal/scene"
	"gopherviz/internal/serve"
	"gopherviz/internal/sim"
	"gopherviz/internal/trace"
)

// version diisi build-bin.mjs (-X main.version) dari versi ekstensi.
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// Perintah yang sudah direncanakan tapi belum dibuat, beserta fasenya.
var planned = map[string]string{
	"show": "terminal view, after Phase 9",
	"on":   "terminal view, after Phase 9",
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return 2
	}
	switch cmd := args[0]; cmd {
	case "list":
		return cmdList(args[1:], stdin, stdout, stderr)
	case "trace":
		return cmdTrace(args[1:], stdin, stdout, stderr)
	case "serve":
		if err := serve.New(version).Run(stdin, stdout); err != nil {
			fmt.Fprintf(stderr, "gopherviz serve: %v\n", err)
			return 1
		}
		return 0
	case "ir": // debug: pohon IR untuk bahasa selain Go
		if len(args) != 2 {
			fmt.Fprintln(stderr, "usage: gopherviz ir <file>")
			return 2
		}
		src, err := os.ReadFile(args[1])
		if err != nil {
			fmt.Fprintf(stderr, "gopherviz: %v\n", err)
			return 1
		}
		language := lang.Detect(args[1])
		if language == "" || language == "go" {
			fmt.Fprintln(stderr, "gopherviz ir is only for languages other than Go")
			return 2
		}
		fmt.Fprint(stdout, ir.Dump(lang.Parse(language, string(src))))
		return 0
	case "version", "--version", "-v":
		fmt.Fprintf(stdout, "gopherviz %s (scene v%d, trace v%d, protocol v%d)\n",
			version, scene.Version, trace.Version, protocol.Version)
		return 0
	case "help", "--help", "-h":
		usage(stdout)
		return 0
	default:
		if phase, ok := planned[cmd]; ok {
			fmt.Fprintf(stderr, "gopherviz %s is not available yet (planned for %s).\n", cmd, phase)
			return 2
		}
		fmt.Fprintf(stderr, "unknown command: %q\n\n", cmd)
		usage(stderr)
		return 2
	}
}

func usage(w io.Writer) {
	fmt.Fprint(w, `gopherviz: see how your code works, step by step

Usage:
  gopherviz list [--json] <file>      list every block and its kind (- for stdin)
  gopherviz trace [--json] <file>:<line>
                                    step through one block (variable values, conditions, output)
  gopherviz serve                     JSON lines server over stdin/stdout (used by the IDE extension)
  gopherviz version                   binary and data schema versions
  gopherviz help                      show this help

Coming soon: on, show.
`)
}

func cmdList(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	var asJSON bool
	var files []string
	for _, a := range args {
		switch {
		case a == "--json" || a == "-json":
			asJSON = true
		case a == "-":
			files = append(files, a)
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "unknown flag: %s\n", a)
			return 2
		default:
			files = append(files, a)
		}
	}
	if len(files) != 1 {
		fmt.Fprintln(stderr, "usage: gopherviz list [--json] <file>")
		return 2
	}

	name, src, ok := readSource(files[0], stdin, stderr)
	if !ok {
		return 1
	}
	res := analyzer.Analyze(name, src)

	if asJSON {
		out, err := res.JSON()
		if err != nil {
			fmt.Fprintf(stderr, "failed to build JSON: %v\n", err)
			return 1
		}
		stdout.Write(out)
		return 0
	}
	printList(stdout, res)
	return 0
}

func readSource(name string, stdin io.Reader, stderr io.Writer) (string, []byte, bool) {
	var src []byte
	var err error
	if name == "-" {
		name = "stdin.go"
		src, err = io.ReadAll(stdin)
	} else {
		src, err = os.ReadFile(name)
	}
	if err != nil {
		fmt.Fprintf(stderr, "failed to read %s: %v\n", name, err)
		return name, nil, false
	}
	return name, bytes.ReplaceAll(src, []byte("\r\n"), []byte("\n")), true
}

func cmdTrace(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	var asJSON bool
	var targets []string
	for _, a := range args {
		switch {
		case a == "--json" || a == "-json":
			asJSON = true
		case strings.HasPrefix(a, "-") && a != "-":
			fmt.Fprintf(stderr, "unknown flag: %s\n", a)
			return 2
		default:
			targets = append(targets, a)
		}
	}
	const use = "usage: gopherviz trace [--json] <file>:<line>"
	if len(targets) != 1 {
		fmt.Fprintln(stderr, use)
		return 2
	}
	i := strings.LastIndex(targets[0], ":")
	line, err := strconv.Atoi(targets[0][i+1:])
	if i <= 0 || err != nil {
		fmt.Fprintln(stderr, use)
		return 2
	}
	name, src, ok := readSource(targets[0][:i], stdin, stderr)
	if !ok {
		return 1
	}
	prog := analyzer.Load(name, src)
	id, found := prog.Result.Scene.BlockAt(line)
	if !found {
		fmt.Fprintf(stderr, "no block (function, if, switch or loop) on line %d\n", line)
		return 1
	}
	tr := sim.Trace(prog, id, sim.Options{Files: project.Disk{}})

	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(tr); err != nil {
			fmt.Fprintf(stderr, "failed to build JSON: %v\n", err)
			return 1
		}
		return 0
	}
	printTrace(stdout, tr)
	return 0
}

func printTrace(w io.Writer, tr trace.Trace) {
	kind := blockKindNames[tr.Block.Kind]
	if tr.Block.Kind == scene.BlockLoop {
		kind = kindNames[tr.Loop.Kind]
	} else if tr.Block.Name != "" {
		kind += " " + tr.Block.Name
	}
	fmt.Fprintf(w, "%s %s lines %d-%d · mode %s · %d steps\n", tr.Block.ID, kind, tr.Block.Line, tr.Block.EndLine, tr.Mode, len(tr.Steps))
	for _, warn := range tr.Warnings {
		fmt.Fprintf(w, "  ! %s\n", warn)
	}
	if tr.Collection != nil {
		fmt.Fprintf(w, "  collection: %v\n", tr.Collection.Items)
	}
	fmt.Fprintln(w)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, s := range tr.Steps {
		out := ""
		if s.Output != "" {
			out = "  ⇒ " + strings.TrimSuffix(s.Output, "\n")
		}
		// kedalaman call stack ditampilkan sebagai indentasi catatan
		fmt.Fprintf(tw, "%d\t%s\t%s\tline %d\t%s%s%s\n", s.N, s.Block, s.Phase, s.Line, strings.Repeat("· ", s.Depth), s.Note, out)
	}
	tw.Flush()
	if tr.Truncated {
		fmt.Fprintln(w, "… (cut)")
	}
}

var kindNames = map[scene.Kind]string{
	scene.KindFor3:         "classic for",
	scene.KindCond:         "condition for",
	scene.KindInfinite:     "endless for",
	scene.KindRangeInt:     "range number",
	scene.KindRangeSlice:   "range slice",
	scene.KindRangeArray:   "range array",
	scene.KindRangeString:  "range string",
	scene.KindRangeMap:     "range map",
	scene.KindRangeChan:    "range channel",
	scene.KindRangeFunc:    "range iterator",
	scene.KindRangeUnknown: "range (unknown type)",
	scene.KindDoWhile:      "do-while",
	scene.KindRangeKeys:    "range key",
}

var blockKindNames = map[scene.BlockKind]string{
	scene.BlockFunc:   "function",
	scene.BlockIf:     "if",
	scene.BlockSwitch: "switch",
	scene.BlockLoop:   "loop",
	scene.BlockClass:  "class",
}

const maxDiagnostics = 5

func printList(w io.Writer, res analyzer.Result) {
	blocks := res.Scene.Blocks
	loops := map[string]scene.Loop{}
	for _, l := range res.Scene.Loops {
		loops[l.ID] = l
	}
	if len(blocks) == 0 {
		fmt.Fprintf(w, "%s: no blocks\n", res.Scene.File)
	} else {
		fmt.Fprintf(w, "%s: %d blocks (%s)\n\n", res.Scene.File, len(blocks), res.Scene.Language)
		depth := map[string]int{}
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "ID\tLINES\tKIND\tHEADER\tBODY\tSTATUS")
		for _, b := range blocks {
			if b.Parent != "" {
				depth[b.ID] = depth[b.Parent] + 1
			}
			lines := fmt.Sprint(b.Line)
			if b.EndLine != b.Line {
				lines = fmt.Sprintf("%d-%d", b.Line, b.EndLine)
			}
			kind, head, body := blockKindNames[b.Kind], b.Header, ""
			switch b.Kind {
			case scene.BlockLoop:
				l := loops[b.ID]
				kind = kindNames[l.Kind]
				if l.Label != "" {
					head = l.Label + ": " + head
				}
				body = parts(len(l.Body))
			case scene.BlockFunc:
				body = parts(len(b.Body))
			case scene.BlockClass:
				n := 0
				for _, c := range b.Children {
					if cb, ok := res.Scene.Block(c); ok && cb.Kind == scene.BlockFunc {
						n++
					}
				}
				body = fmt.Sprintf("%d method", n)
			default:
				body = plural(len(b.Branches), "branch", "branches")
			}
			state := "complete"
			if b.State == scene.StateIncomplete {
				state = "incomplete"
			}
			fmt.Fprintf(tw, "%s%s\t%s\t%s\t%s\t%s\t%s\n",
				strings.Repeat("  ", depth[b.ID]), b.ID, lines, kind, clip(head, 44), body, state)
		}
		tw.Flush()
	}
	if n := len(res.Diagnostics); n > 0 {
		fmt.Fprintf(w, "\nParser notes (%d):\n", n)
		for i, d := range res.Diagnostics {
			if i == maxDiagnostics {
				fmt.Fprintf(w, "  … and %d more\n", n-maxDiagnostics)
				break
			}
			fmt.Fprintf(w, "  line %d:%d  %s\n", d.Line, d.Col, d.Message)
		}
	}
}

func parts(n int) string {
	if n == 0 {
		return "empty"
	}
	return plural(n, "part", "parts")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}
