// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package ebnfutils (WIP:TODO) provides some utilities for messing with EBNF
// grammars.
package ebnfutils

import (
	"bytes"
	"fmt"
	"go/ast"
	"sort"

	"code.google.com/p/go.exp/ebnf"
	"github.com/cznic/strutil"
)

//TODO Reduce
//TODO ToBNF
//TODO ToNFA
//TODO Equals?

var (
	altA   = map[bool]string{false: "%i\n ", true: " "}
	altZ   = map[bool]string{false: " %u", true: " "}
	altBar = map[bool]string{false: "", true: " |"}
	grpL   = map[bool]string{false: " (", true: " (\n%i"}
	grpR   = map[bool]string{false: " )", true: "%u\n  )"}
	optL   = map[bool]string{false: " [", true: " [\n%i"}
	optR   = map[bool]string{false: " ]", true: "%u\n  ]"}
	repL   = map[bool]string{false: " {", true: " {\n%i"}
	repR   = map[bool]string{false: " }", true: "%u\n  }"}
)

// Grammar is ebnf.Grammar extended with utility methods.
type Grammar ebnf.Grammar

func prodLen(expr ebnf.Expression) (y int) {
	var f func(ebnf.Expression)
	f = func(expr ebnf.Expression) {
		switch x := expr.(type) {
		case nil:
			// nop
		case ebnf.Sequence:
			for _, v := range x {
				f(v)
			}
		case ebnf.Alternative:
			for _, v := range x {
				f(v)
			}
		case *ebnf.Option:
			f(x.Body)
		case *ebnf.Group:
			f(x.Body)
		case *ebnf.Repetition:
			f(x.Body)
		case *ebnf.Name, *ebnf.Token, *ebnf.Range:
			y++
		default:
			panic(fmt.Sprintf("internal error %T(%v)", x, x))
		}
	}
	f(expr)
	return
}

func isShort(expr ebnf.Expression) bool {
	return prodLen(expr) <= 2
}

// String implements fmt.Stringer.
func (g Grammar) String() string {
	term, nterm := []string{}, []string{}
	for name := range g {
		if ast.IsExported(name) {
			nterm = append(nterm, name)
			continue
		}

		term = append(term, name)
	}
	sort.Strings(nterm)
	sort.Strings(term)

	var buf bytes.Buffer
	f := strutil.IndentFormatter(&buf, "\t")

	var h func(ebnf.Expression, bool)
	h = func(expr ebnf.Expression, newLine bool) {
		switch x := expr.(type) {
		case nil:
			// nop
		case *ebnf.Production:
			name := x.Name.String
			f.Format("%s =%i", name)
			h(g[name].Expr, true)
			f.Format(" .%u\n")
		case ebnf.Alternative:
			switch isShort(x) {
			case true:
				for i, v := range x {
					f.Format(altBar[i != 0])
					h(v, false)
				}
			default:
				for i, v := range x {
					switch i {
					case 0:
						f.Format(altA[newLine])
					default:
						f.Format("\n|")
					}
					h(v, false)
				}
				f.Format(altZ[newLine])
			}
		case ebnf.Sequence:
			for _, v := range x {
				h(v, false)
			}
		case *ebnf.Group:
			long := !isShort(x.Body)
			f.Format(grpL[long])
			h(x.Body, long)
			f.Format(grpR[long])
		case *ebnf.Option:
			long := !isShort(x.Body)
			f.Format(optL[long])
			h(x.Body, long)
			f.Format(optR[long])
		case *ebnf.Repetition:
			long := !isShort(x.Body)
			f.Format(repL[long])
			h(x.Body, long)
			f.Format(repR[long])
		case *ebnf.Token:
			f.Format(" %q", x.String)
		case *ebnf.Name:
			f.Format(" %s", x.String)
		case *ebnf.Range:
			f.Format(" %q … %q", x.Begin.String, x.End.String)
		default:
			panic(fmt.Sprintf("internal error %T(%v)", x, x))
		}
	}

	for _, name := range term {
		h(g[name], false)
	}

	f.Format("\n")
	for _, name := range nterm {
		h(g[name], false)
	}
	return buf.String()
}
