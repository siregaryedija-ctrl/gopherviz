package irsim

import (
	"strconv"

	"gopherviz/internal/ir"
)

// pyTypeName: nama tipe Python untuk pesan error.
func (it *interp) pyTypeName(v Value) string {
	switch v.k {
	case kInt, kBig:
		return "int"
	case kFloat:
		return "float"
	case kStr:
		return "str"
	case kBool:
		return "bool"
	case kNull:
		return "NoneType"
	case kList:
		if v.l.kind == "tuple" {
			return "tuple"
		}
		return "list"
	case kDict:
		return "dict"
	case kSet:
		return "set"
	case kObj:
		return v.o.class.name
	case kFunc:
		return "function"
	case kClass:
		return "type"
	}
	return "object"
}

// phpIntKey: string berisi bilangan bulat desimal kanonik menjadi key integer di PHP.
func phpIntKey(s string) (int64, bool) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || strconv.FormatInt(n, 10) != s {
		return 0, false
	}
	return n, true
}

// pyFormat dan pyMatch diisi di python.go (Fase 9c).
var pyFormatImpl func(it *interp, v Value, spec string, at ir.Node) string
var pyMatchImpl func(it *interp, n *ir.Switch, e *Env, focus bool) flow

func (it *interp) pyFormat(v Value, spec string, at ir.Node) string {
	if pyFormatImpl != nil {
		return pyFormatImpl(it, v, spec, at)
	}
	return it.sem.str(it, v)
}

func (it *interp) pyMatch(n *ir.Switch, e *Env, focus bool) flow {
	if pyMatchImpl != nil {
		return pyMatchImpl(it, n, e, focus)
	}
	it.structural("match is not simulated yet")
	return flow{}
}
