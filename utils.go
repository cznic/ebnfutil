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
	"io"
	"sort"
	"strings"

	"code.google.com/p/go.exp/ebnf"
	"github.com/cznic/strutil"
)

//TODO Reduce

var (
	altA   = map[bool]string{false: "%i\n", true: " "}
	altT   = map[bool]string{false: "%i\n", true: ""}
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

// Parse parses a set of EBNF productions from source src. It returns a set of
// productions. Errors are reported for incorrect syntax and if a production is
// declared more than once; the filename is used only for error positions.
func Parse(filename string, src io.Reader) (g Grammar, err error) {
	g0, err := ebnf.Parse(filename, src)
	g = Grammar(g0)
	return
}

// Analyze analyzes g with starting production 'start' and returns a Report
// about it.
//
// 'start' is the name of the start production.
//
// Note: The grammar should be verified before invoking this method. Otherwise
// errors may occur.
func (g Grammar) Analyze(start string) (r *Report, err error) {
	seen := map[string]bool{}
	var f func(string, ebnf.Expression)
	f = func(name string, expr ebnf.Expression) {
		if err != nil {
			return
		}

		switch x := expr.(type) {
		case nil:
			// nop
		case *ebnf.Production:
			if x == nil {
				err = fmt.Errorf("Missing production %q", name)
				return
			}

			switch ast.IsExported(name) {
			case false:
				r.Lexical[name] = true
			default:
				r.NonTerminals[name] = true
			}
			if seen[name] {
				return
			}

			seen[name] = true
			f(name, x.Expr)
		case ebnf.Alternative:
			for _, v := range x {
				f(name, v)
			}
		case ebnf.Sequence:
			for _, v := range x {
				f(name, v)
			}
		case *ebnf.Group:
			f(name, x.Body)
		case *ebnf.Option:
			f(name, x.Body)
		case *ebnf.Repetition:
			f(name, x.Body)
		case *ebnf.Name:
			name2 := x.String
			r.UsedBy[name2][name] = true
			if ast.IsExported(name) && !ast.IsExported(name2) {
				r.Tokens[name2] = true
			}
			f(name2, g[name2])
		case *ebnf.Token:
			r.Literals[x.String] = true
		case *ebnf.Range:
			r.Ranges[struct{ Begin, End string }{x.Begin.String, x.End.String}] = true
		default:
			panic(fmt.Sprintf("internal error %T(%v)", x, x))
		}
	}

	r = &Report{
		Lexical:      map[string]bool{},
		Literals:     map[string]bool{},
		NonTerminals: map[string]bool{},
		Ranges:       map[struct{ Begin, End string }]bool{},
		Tokens:       map[string]bool{},
		UsedBy:       map[string]map[string]bool{},
	}
	for name := range g {
		r.UsedBy[name] = map[string]bool{}
	}
	f(start, g[start])
	return
}

// BNF returns g converted to a Grammar without any of:
//
//	*ebnf.Group
//	*ebnf.Option
//	*ebnf.Repetition
//
// Removing the above items requires expanding them via adding new productions
// to the grammar. Names for such productions are obtained via nameInventor.
// The name of the production for which the item must be expaned is passed to
// the nameInventor function. nameInventor must not return a name already
// existing in the grammar nor it may return any name more than once.  Nil
// nameInventor can be passed to use a default implementation.
//
// 'start' is the name of the start production.
func (g Grammar) BNF(start string, nameInventor func(name string) string) (r Grammar, repetitions map[string]bool, err error) {
	if nameInventor == nil {
		names := map[string]bool{}
		for _, name := range []string{
			"break", "default", "func", "interface", "select",
			"case", "defer", "go", "map", "struct",
			"chan", "else", "goto", "package", "switch",
			"const", "fallthrough", "if", "range", "type",
			"continue", "for", "import", "return", "var",
		} {
			names[name] = true
		}
		for name := range g {
			if names[name] {
				err = fmt.Errorf("Reserved word %q cannot be used as a production name.", name)
				break
			}

			names[name] = true
		}
		nameInventor = func(name string) (s string) {
			const sep = "_"
			for i := 0; ; i++ {
				switch {
				case i == 0 && sep == "":
					s = fmt.Sprintf("%s%s", name, sep)
				case i == 0:
					continue
				case i != 0:
					s = fmt.Sprintf("%s%s%d", name, sep, i)
				}
				if _, ok := names[s]; !ok {
					names[s] = true
					return s
				}
			}
		}
	}

	var f func(string, int, ebnf.Expression) ebnf.Expression

	add := func(name string, expr ebnf.Expression) (nm *ebnf.Name) {
		nm = &ebnf.Name{String: name}
		r[name] = &ebnf.Production{Name: nm, Expr: f(name, 0, expr)}
		return
	}

	f = func(name string, nest int, expr ebnf.Expression) (r ebnf.Expression) {
		nest++
		switch x := expr.(type) {
		case nil:
			return nil
		case ebnf.Alternative:
			switch nest {
			case 1:
				y := ebnf.Alternative{}
				for _, v := range x {
					y = append(y, f(name, nest, v))
				}
				return y
			default:
				return add(nameInventor(name), x)
			}
		case *ebnf.Option:
			return add(nameInventor(name), ebnf.Alternative{
				0: nil,
				1: x.Body,
			})
		case *ebnf.Repetition:
			newName := nameInventor(name)
			repetitions[newName] = true
			return add(newName, ebnf.Alternative{
				0: nil,
				1: ebnf.Sequence{
					0: &ebnf.Name{String: newName},
					1: x.Body,
				},
			})
		case *ebnf.Group:
			return add(nameInventor(name), x.Body)
		case ebnf.Sequence:
			y := ebnf.Sequence{}
			for _, v := range x {
				y = append(y, f(name, nest, v))
			}
			return y
		case *ebnf.Name:
			return &ebnf.Name{String: x.String}
		case *ebnf.Token:
			return &ebnf.Token{String: x.String}
		case *ebnf.Range:
			return &ebnf.Range{
				Begin: &ebnf.Token{String: x.Begin.String},
				End:   &ebnf.Token{String: x.End.String},
			}
		default:
			panic(fmt.Sprintf("internal error %T(%v)", x, x))
		}
	}

	r = Grammar{}
	repetitions = map[string]bool{}
	for name, prod := range g {
		r[name] = &ebnf.Production{Name: &ebnf.Name{String: name}, Expr: f(name, 0, prod.Expr)}
	}
	return
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

	var h func(ebnf.Expression, bool, bool)
	h = func(expr ebnf.Expression, newLine, tld bool) {
		switch x := expr.(type) {
		case nil:
			// nop
		case *ebnf.Production:
			name := x.Name.String
			f.Format("%s =%i", name)
			h(g[name].Expr, true, true)
			f.Format(" .%u\n")
		case ebnf.Alternative:
			hasNil := false
			for _, v := range x {
				if hasNil = v == nil; hasNil {
					break
				}
			}
			switch {
			case isShort(x) && !hasNil:
				for i, v := range x {
					f.Format(altBar[i != 0])
					h(v, false, false)
				}
			default:
				for i, v := range x {
					switch {
					case i == 0 && !tld:
						f.Format(altA[newLine])
					case i == 0 && tld:
						f.Format(altT[newLine])
					default:
						f.Format("\n|")
					}
					h(v, false, false)
				}
				f.Format(altZ[newLine])
			}
		case ebnf.Sequence:
			for _, v := range x {
				h(v, false, false)
			}
		case *ebnf.Group:
			long := !isShort(x.Body)
			f.Format(grpL[long])
			h(x.Body, long, false)
			f.Format(grpR[long])
		case *ebnf.Option:
			long := !isShort(x.Body)
			f.Format(optL[long])
			h(x.Body, long, false)
			f.Format(optR[long])
		case *ebnf.Repetition:
			long := !isShort(x.Body)
			f.Format(repL[long])
			h(x.Body, long, false)
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
		h(g[name], false, false)
	}

	f.Format("\n")
	for _, name := range nterm {
		h(g[name], false, false)
	}
	return buf.String()
}

// Verify checks that:
//
//	- all productions used are defined
//	- all productions defined are used when beginning at start
//	- lexical productions refer only to other lexical productions
func (g Grammar) Verify(start string) error {
	return ebnf.Verify(ebnf.Grammar(g), start)
}

// Report is returned from Analyze.
type Report struct {
	// Set of names of all lexical productions.
	Lexical map[string]bool
	// Set of all ebnf.Token.String values
	Literals map[string]bool
	// Set of names of all non terminal productions.
	NonTerminals map[string]bool
	// Set of all ebnf.Range.{Begin,End} pairs.
	Ranges map[struct{ Begin, End string }]bool
	// Set of all lexical production names referenced from within a
	// non-terminal production.
	Tokens map[string]bool
	// UsedBy is map of production names to a set of production names which
	// refers to them, ie. a cross-reference. For example a grammar:
	//
	//        Start = number | Start number .
	//        number = "0" … "9" .
	//
	// produces:
	//
	//        map[string]map[string]bool{
	//                "Start":map[string]bool{"Start": true},
	//                "number":map[string]bool{"Start": true},
	//        }
	UsedBy map[string]map[string]bool
}

// String implements fmt.Stringer.
func (r *Report) String() string {
	a := [6]string{}
	a[0] = fmt.Sprintf("Lexical %s", str(r.Lexical))
	a[1] = fmt.Sprintf("Literals %s", str(r.Literals))
	a[2] = fmt.Sprintf("NonTerminals %s", str(r.NonTerminals))
	aa := []string{}
	for v := range r.Ranges {
		aa = append(aa, fmt.Sprintf("%q … %q", v.Begin, v.End))
	}
	sort.Strings(aa)
	a[3] = fmt.Sprintf("Ranges %s", fmt.Sprintf("[%s]", strings.Join(aa, " ")))
	a[4] = fmt.Sprintf("Tokens %s", str(r.Tokens))
	aa = []string{}
	for v := range r.UsedBy {
		aa = append(aa, v)
	}
	sort.Strings(aa)
	bb := []string{}
	for _, v := range aa {
		aaa := []string{}
		for vv := range r.UsedBy[v] {
			aaa = append(aaa, fmt.Sprintf("%q", vv))
		}
		sort.Strings(aaa)
		bb = append(bb, fmt.Sprintf("\t%q: [%s]", v, strings.Join(aaa, " ")))
	}
	a[5] = fmt.Sprintf("UsedBy:\n%s", strings.Join(bb, "\n"))
	return strings.Join(a[:], "\n") + "\n"
}

func isShort(expr ebnf.Expression) bool {
	return prodLen(expr) <= 2
}

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

func str(m map[string]bool) string {
	a := []string{}
	for s := range m {
		a = append(a, fmt.Sprintf("%q", s))
	}
	sort.Strings(a)
	return fmt.Sprintf("[%s]", strings.Join(a, " "))
}
