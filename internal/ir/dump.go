package ir

import (
	"fmt"
	"strings"
)

// Dump menulis IR dalam bentuk S-expression ringkas (untuk test dan debugging).
func Dump(f *File) string {
	var b strings.Builder
	for _, s := range f.Body {
		dump(&b, s, 0)
	}
	for _, d := range f.Diags {
		fmt.Fprintf(&b, "!%d:%d %s\n", d.Pos.Line, d.Pos.Col, d.Msg)
	}
	return b.String()
}

func dump(b *strings.Builder, n Node, ind int) {
	pad := strings.Repeat("  ", ind)
	line := func(format string, args ...any) {
		fmt.Fprintf(b, "%s%d: %s\n", pad, n.Span().Start.Line, fmt.Sprintf(format, args...))
	}
	switch n := n.(type) {
	case *Block:
		closed := ""
		if !n.Closed {
			closed = " (belum ditutup)"
		}
		line("block%s", closed)
		for _, s := range n.List {
			dump(b, s, ind+1)
		}
	case *ExprStmt:
		line("expr %s", E(n.X))
	case *VarDecl:
		var parts []string
		for _, s := range n.Specs {
			x := s.Name
			if s.Pattern != nil {
				x = E(s.Pattern)
			}
			if len(s.Dims) > 0 {
				x += fmt.Sprintf("[%d dim]", len(s.Dims))
			}
			if s.Init != nil {
				x += " = " + E(s.Init)
			}
			if s.Ctor != nil {
				x += "(" + exprs(s.Ctor) + ")"
			}
			parts = append(parts, x)
		}
		line("var(%s) %s", n.Kind, strings.Join(parts, ", "))
	case *If:
		kw := "if"
		if n.Elif {
			kw = "elif"
		}
		line("%s %s", kw, E(n.Cond))
		dump(b, n.Then, ind+1)
		if n.Else != nil {
			fmt.Fprintf(b, "%s  else\n", pad)
			dump(b, n.Else, ind+1)
		}
	case *For:
		init := ""
		if n.Init != nil {
			init = strings.TrimSpace(strings.SplitN(dumpOne(n.Init), ": ", 2)[1])
		}
		line("for [%s] [%s] [%s]", init, E(n.Cond), E(n.Post))
		dump(b, n.Body, ind+1)
	case *ForEach:
		k := ""
		if n.Key != nil {
			k = E(n.Key) + " => "
		}
		line("foreach(%s %s) %s%s in %s", n.Kind, n.Decl, k, E(n.Value), E(n.X))
		dump(b, n.Body, ind+1)
		if n.Else != nil {
			fmt.Fprintf(b, "%s  else\n", pad)
			dump(b, n.Else, ind+1)
		}
	case *While:
		kw := "while"
		if n.Do {
			kw = "do-while"
		}
		line("%s %s", kw, E(n.Cond))
		dump(b, n.Body, ind+1)
		if n.Else != nil {
			fmt.Fprintf(b, "%s  else\n", pad)
			dump(b, n.Else, ind+1)
		}
	case *Switch:
		kw := "switch"
		if n.Match {
			kw = "match"
		}
		line("%s %s", kw, E(n.Tag))
		for _, c := range n.Cases {
			label := "default"
			if c.Exprs != nil {
				label = "case " + exprs(c.Exprs)
			}
			if c.Guard != nil {
				label += " if " + E(c.Guard)
			}
			fmt.Fprintf(b, "%s  %d: %s\n", pad, c.S.Start.Line, label)
			for _, s := range c.Body {
				dump(b, s, ind+2)
			}
		}
	case *Branch:
		line("%s %s", n.Tok, n.Label)
	case *Return:
		line("return %s", E(n.X))
	case *Throw:
		line("throw %s", E(n.X))
	case *Try:
		line("try")
		dump(b, n.Body, ind+1)
		for _, c := range n.Catches {
			fmt.Fprintf(b, "%s  catch %v %s\n", pad, c.Types, c.Name)
			dump(b, c.Body, ind+1)
		}
		if n.Finally != nil {
			fmt.Fprintf(b, "%s  finally\n", pad)
			dump(b, n.Finally, ind+1)
		}
	case *Func:
		line("func %s", funcSig(n))
		if n.Body != nil {
			dump(b, n.Body, ind+1)
		} else if n.ExprBody != nil {
			fmt.Fprintf(b, "%s  => %s\n", pad, E(n.ExprBody))
		}
	case *Class:
		line("%s %s %v", n.Kind, n.Name, n.Extends)
		for _, m := range n.Members {
			dump(b, m, ind+1)
		}
	case *Labeled:
		line("label %s", n.Label)
		dump(b, n.Stmt, ind+1)
	case *Simple:
		line("%s %v %s", n.Kind, n.Names, exprs(n.X))
	case *Bad:
		line("BAD %s", n.Msg)
	default:
		line("?%T", n)
	}
}

func dumpOne(n Node) string {
	var b strings.Builder
	dump(&b, n, 0)
	return strings.TrimSpace(strings.SplitN(b.String(), "\n", 2)[0])
}

func funcSig(f *Func) string {
	var ps []string
	for _, p := range f.Params {
		s := p.Name
		if p.Pattern != nil {
			s = E(p.Pattern)
		}
		if p.Rest {
			s = "..." + s
		}
		if p.Default != nil {
			s += "=" + E(p.Default)
		}
		ps = append(ps, s)
	}
	name := f.Name
	if f.Class != "" {
		name = f.Class + "." + name
	}
	if f.Arrow {
		name += "=>"
	}
	return fmt.Sprintf("%s(%s)", name, strings.Join(ps, ", "))
}

func exprs(l []Expr) string {
	var out []string
	for _, x := range l {
		out = append(out, E(x))
	}
	return strings.Join(out, ", ")
}

// E menulis ekspresi dalam bentuk S-expression.
func E(x Expr) string {
	if x == nil {
		return ""
	}
	switch x := x.(type) {
	case *Ident:
		return x.Name
	case *Lit:
		switch x.Kind {
		case LitString:
			return fmt.Sprintf("%q", x.Value)
		case LitChar:
			return fmt.Sprintf("'%s'", x.Value)
		}
		return x.Value
	case *Template:
		var b strings.Builder
		b.WriteString("`")
		for i, p := range x.Parts {
			b.WriteString(p)
			if i < len(x.Exprs) {
				b.WriteString("${" + E(x.Exprs[i]) + "}")
			}
		}
		b.WriteString("`")
		return b.String()
	case *Unary:
		if x.Postfix {
			return "(" + E(x.X) + x.Op + ")"
		}
		return "(" + x.Op + " " + E(x.X) + ")"
	case *Binary:
		return "(" + E(x.X) + " " + x.Op + " " + E(x.Y) + ")"
	case *Assign:
		return "(" + E(x.L) + " " + x.Op + " " + E(x.R) + ")"
	case *Cond:
		return "(" + E(x.C) + " ? " + E(x.T) + " : " + E(x.F) + ")"
	case *Call:
		return E(x.Fun) + "(" + exprs(x.Args) + ")"
	case *KwArg:
		return x.Name + "=" + E(x.Value)
	case *New:
		s := "new " + x.Type
		if len(x.Dims) > 0 {
			s += fmt.Sprintf("[%s]", exprs(x.Dims))
		}
		if x.Init != nil {
			s += E(x.Init)
		}
		return s + "(" + exprs(x.Args) + ")"
	case *Member:
		return E(x.X) + x.Op + x.Name
	case *Index:
		return E(x.X) + "[" + E(x.I) + "]"
	case *Slice:
		return E(x.X) + "[" + E(x.Lo) + ":" + E(x.Hi) + ":" + E(x.Step) + "]"
	case *ArrayLit:
		return x.Kind + "[" + exprs(x.Elems) + "]"
	case *ObjectLit:
		var ps []string
		for _, p := range x.Props {
			switch {
			case p.Spread:
				ps = append(ps, "..."+E(p.Value))
			case p.Key == nil:
				ps = append(ps, E(p.Value))
			default:
				ps = append(ps, E(p.Key)+": "+E(p.Value))
			}
		}
		return x.Kind + "{" + strings.Join(ps, ", ") + "}"
	case *Func:
		if x.ExprBody != nil {
			return "fn " + funcSig(x) + " " + E(x.ExprBody)
		}
		return "fn " + funcSig(x) + " {...}"
	case *Class:
		return x.Kind + " " + x.Name + "{...}"
	case *Spread:
		return "..." + E(x.X)
	case *Cast:
		return "(" + x.Type + ")" + E(x.X)
	case *Comp:
		var fs []string
		for _, f := range x.Fors {
			s := "for " + E(f.Target) + " in " + E(f.Iter)
			for _, c := range f.Ifs {
				s += " if " + E(c)
			}
			fs = append(fs, s)
		}
		elt := E(x.Elt)
		if x.Kind == "dict" {
			elt = E(x.Key) + ": " + E(x.Value)
		}
		return x.Kind + "comp[" + elt + " " + strings.Join(fs, " ") + "]"
	case *This:
		return x.Name
	case *Seq:
		return "(seq " + exprs(x.List) + ")"
	case *TypeRef:
		return "type:" + x.Name
	case *BadExpr:
		return "BAD"
	}
	return fmt.Sprintf("?%T", x)
}
