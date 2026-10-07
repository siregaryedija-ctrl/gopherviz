// Package lang menghubungkan parser bahasa selain Go (internal/lang/clike, internal/lang/python)
// dengan pembangun Scene: deteksi bahasa dari nama file, parse ke IR, lalu IR → Scene v2.
package lang

import (
	"path/filepath"
	"strings"

	"gopherviz/internal/ir"
	"gopherviz/internal/lang/clike"
	"gopherviz/internal/lang/python"
)

// Names adalah nama tampilan tiap bahasa.
var Names = map[string]string{
	"go": "Go", "javascript": "JavaScript", "typescript": "TypeScript", "python": "Python",
	"php": "PHP", "cpp": "C++", "c": "C", "java": "Java",
}

// Detect menebak bahasa dari ekstensi file. "" bila tidak dikenal.
func Detect(filename string) string {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".go":
		return "go"
	case ".js", ".mjs", ".cjs", ".jsx":
		return "javascript"
	case ".ts", ".mts", ".cts", ".tsx":
		return "typescript"
	case ".py", ".pyw":
		return "python"
	case ".php", ".phtml":
		return "php"
	case ".cpp", ".cc", ".cxx", ".c++", ".hpp", ".hh", ".hxx":
		return "cpp"
	case ".c", ".h":
		return "c"
	case ".java":
		return "java"
	}
	return ""
}

// FromEditor memetakan languageId VS Code ke nama bahasa GopherViz.
func FromEditor(id string) string {
	switch id {
	case "go":
		return "go"
	case "javascript", "javascriptreact":
		return "javascript"
	case "typescript", "typescriptreact":
		return "typescript"
	case "python":
		return "python"
	case "php":
		return "php"
	case "cpp", "cuda-cpp":
		return "cpp"
	case "c":
		return "c"
	case "java":
		return "java"
	}
	return ""
}

// Parse mem-parse src dalam bahasa lang (bukan Go) menjadi IR.
func Parse(language, src string) *ir.File {
	if language == "python" {
		return python.Parse(src)
	}
	return clike.Parse(language, src)
}
