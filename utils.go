// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package ebnfutils (WIP:TODO) provides some utilities for messing with EBNF
// grammars.
//
// Positions attached to particular ebnf package types instances are ignored in
// most, if not all places. Positions make sense after Parse, but usually no
// more after mutating the grammar in any way.
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

var (
	altA   = map[bool]string{false: "%i\n", true: " "}
	altT   = map[bool]string{false: "%i\n", true: ""}
	altZ   = map[bool]string{false: "%u", true: ""}
	altBar = map[bool]string{false: "", true: " |"}
	grpL   = map[bool]string{false: " (", true: " (\n%i"}
	grpR   = map[bool]string{false: " )", true: "%u\n  )"}
	optL   = map[bool]string{false: " [", true: " [\n%i"}
	optR   = map[bool]string{false: " ]", true: "%u\n  ]"}
	repL   = map[bool]string{false: " {", true: " {\n%i"}
	repR   = map[bool]string{false: " }", true: "%u\n  }"}

	tests bool // Testing hook
)

// NormalizeExpression returns a normalized clone of expr. Positions are ignored.
func NormalizeExpression(expr ebnf.Expression) ebnf.Expression {
	switch x := expr.(type) {
	case nil:
		return nil
	case ebnf.Alternative:
		for stable := false; !stable; {
			stable = true
		loop:
			for i, v := range x {
				if a, ok := v.(*ebnf.Group); ok {
					var aa ebnf.Expression
					for {
						aa = NormalizeExpression(a.Body)
						if aaa, ok := aa.(*ebnf.Group); !ok {
							break
						} else {
							a = aaa
						}
					}
					switch a := aa.(type) {
					case ebnf.Alternative:
						y := ebnf.Alternative{}
						if i > 0 {
							y = append(y, x[:i]...)
						}
						y = append(y, a...)
						y = append(y, x[i+1:]...)
						x = y
						stable = false
						break loop
					case ebnf.Sequence:
						x[i] = a
						continue
					case nil:
						x[i] = nil
					}

				}
			}
		}
		y := ebnf.Alternative{}
		for _, v := range x {
			y = append(y, NormalizeExpression(v))
		}
		return y
	case ebnf.Sequence:
		if len(x) == 1 {
			switch x[0].(type) {
			case nil:
				return nil
			}
		}

		for stable := false; !stable; {
			stable = true
		loop2:
			for i, v := range x {
				switch a := v.(type) {
				case *ebnf.Group:
					var aa ebnf.Expression
					for {
						aa = NormalizeExpression(a.Body)
						if aaa, ok := aa.(*ebnf.Group); !ok {
							break
						} else {
							a = aaa
						}
					}
					switch a := aa.(type) {
					case ebnf.Sequence:
						y := ebnf.Sequence{}
						if i > 0 {
							y = append(y, x[:i]...)
						}
						y = append(y, a...)
						y = append(y, x[i+1:]...)
						x = y
						stable = false
						break loop2
					}

				case ebnf.Sequence:
					y := ebnf.Sequence{}
					if i > 0 {
						y = append(y, x[:i]...)
					}
					y = append(y, a...)
					y = append(y, x[i+1:]...)
					x = y
					stable = false
					break loop2
				}
			}
		}
		y := ebnf.Sequence{}
		for _, v := range x {
			if v != nil {
				y = append(y, NormalizeExpression(v))
			}
		}
		return y
	case *ebnf.Group:
		switch xx := x.Body.(type) {
		case *ebnf.Group:
			return NormalizeExpression(xx)
		default:
			switch {
			case prodLen(x) == 1:
				return NormalizeExpression(x.Body)
			default:
				switch x.Body.(type) {
				case *ebnf.Group, *ebnf.Option, *ebnf.Repetition:
					return NormalizeExpression(x.Body)
				default:
					return &ebnf.Group{Body: NormalizeExpression(x.Body)}
				}
			}
		}
	case *ebnf.Option:
		switch xx := x.Body.(type) {
		case *ebnf.Group:
			return &ebnf.Option{Body: NormalizeExpression(xx.Body)}
		default:
			return &ebnf.Option{Body: NormalizeExpression(x.Body)}
		}
	case *ebnf.Repetition:
		switch xx := x.Body.(type) {
		case *ebnf.Group:
			return &ebnf.Repetition{Body: NormalizeExpression(xx.Body)}
		default:
			return &ebnf.Repetition{Body: NormalizeExpression(x.Body)}
		}
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

// NormalizeProduction returns a normalized clone of prod. Positions are ignored.
func NormalizeProduction(prod *ebnf.Production) *ebnf.Production {
	return &ebnf.Production{Name: &ebnf.Name{String: prod.Name.String}, Expr: NormalizeExpression(prod.Expr)}
}

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
// Note: The grammar should be verified before invoking this method. Otherwise
// errors may occur.
func (g Grammar) Analyze() (r *Report, err error) {
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
			r.IsBNF = false
			f(name, x.Body)
		case *ebnf.Option:
			r.IsBNF = false
			f(name, x.Body)
		case *ebnf.Repetition:
			r.IsBNF = false
			f(name, x.Body)
		case *ebnf.Name:
			name2 := x.String
			r.Used[name2]++
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
		IsBNF:        true,
		Lexical:      map[string]bool{},
		Literals:     map[string]bool{},
		NonTerminals: map[string]bool{},
		Ranges:       map[struct{ Begin, End string }]bool{},
		Tokens:       map[string]bool{},
		Used:         map[string]int{},
		UsedBy:       map[string]map[string]bool{},
	}
	for name := range g {
		r.UsedBy[name] = map[string]bool{}
	}
	for name := range g {
		f(name, g[name])
	}
	return
}

// BNF returns g converted to a grammar without any of:
//
//	*ebnf.Group
//	*ebnf.Option
//	*ebnf.Repetition
//
// Removing the above items requires expanding them via adding new productions
// to the grammar. Names for such productions are obtained via nameInventor.
// The name of the production for which the item must be expanded is passed to
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
	if err != nil {
		return
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

// Inline attempts to remove some of the g's productions by inlining them into
// the places where they are used. For example this grammar:
//
//	Start = "0" Abc "9".
//	Abc = "abc" .
//
// becomes
//
//	Start = "0" "abc" "9" .
//
// Eligible productions are non self referential (eg. `P = P | P Q .`) non
// terminals used only once (or unlimited times when all == true).
//
// If g is a BNF grammar, it will still be a BNF grammar after Inline.
//
// Note: If no productions can be inlined, no error is reported.  Comparing the
// number of productions before and after calling Inline can reveal if any
// inlining was performed.
//
// 'start' is the name of the start production.
func (g Grammar) Inline(start string, all bool) (err error) {
	for a, b := -1, len(g); a != b; a, b = b, len(g) {
		s := []string{}
		for name := range g {
			s = append(s, name)
		}
		if tests {
			sort.Strings(s) // Reproducible result for testing
		}
		for _, name := range s {
			if name == start || !ast.IsExported(name) {
				continue // lexical
			}

			if err = g.InlineOne(name, all); err != nil {
				return
			}
		}
	}
	return
}

// InlineOne attempts to inline production 'name' into all places where it is
// used. For example, consider this grammar:
//
//	Start = "0" Abc Def "9".
//	Def = "X" | Abc .
//	Abc = "abc" .
//
// Performing InlineOne("Abc", true), it becomes:
//
//	Start = "0" "abc" Def "9" .
//	Def = "X" | "abc .
//
// Eligible productions are non self referential (eg. `P = P | P Q .`) non
// terminals used only once (or unlimited times when all == true).
//
// If g is a BNF grammar, it will still be a BNF grammar after InlineOne.
//
// Consider this EBNF grammar:
//
//	S = A B ( C | "5" ) .
//	A = "1" .
//	B = "2" | "3" .
//	C = "4" .
//
// Performing InlineOne("B", true) produces this EBNF grammar:
//
//	S = A ( "2" | "3" ) ( C | "5" ) .
//	A = "1" .
//	C = "4" .
//
// Consider this BNF grammar:
//
//	S = A B C .
//	A = "1" .
//	B = "2" | "3" .
//	C = "4" .
//
// Performing InlineOne("B", true) produces this BNF grammar:
//
//	S = A "2" C | A "3" C .
//	A = "1" .
//	C = "4" .
//
// Note: If the production cannot be inlined, no error is reported.  Comparing
// the number of productions before and after calling InlineOne can reveal if
// inlining was performed.
//
// Note: Invoking InlineOne for the start production may render the grammar
// unusable.
func (g Grammar) InlineOne(name string, all bool) (err error) {
	//TODO "Algorithm" used is naive, performance is poor.
	if !ast.IsExported(name) {
		return // lexical
	}

	rep, err := g.Analyze()
	if !all && rep.Used[name] > 1 || rep.Used[name] == 0 {
		return
	}

	for user := range rep.UsedBy[name] {
		if user == name {
			return // Self referential.
		}
	}

	switch rep.IsBNF {
	case true:
		g.inlineBNF(name, rep.UsedBy[name])
	default:
		g.inlineEBNF(name, rep.UsedBy[name])
	}
	return
}

func orthogonalBNF(expr0 ebnf.Expression) (yy ebnf.Expression) {
	var f func(int, ebnf.Expression) ebnf.Expression
	f = func(lvl int, expr ebnf.Expression) ebnf.Expression {
		lvl++
		switch x := expr.(type) {
		case nil, *ebnf.Token, *ebnf.Range, *ebnf.Name:
			switch lvl {
			case 2:
				return ebnf.Sequence{0: x}
			default:
				return ebnf.Alternative{0: ebnf.Sequence{0: expr}}
			}
		case ebnf.Alternative:
			switch lvl {
			case 1:
				y := ebnf.Alternative{}
				for _, v := range x {
					y = append(y, f(lvl, v))
				}
				return y
			default:
				panic(fmt.Sprintf("internal error %d %T(%v)", lvl, x, x))
			}
		case ebnf.Sequence:
			switch lvl {
			case 1:
				return ebnf.Alternative{0: f(lvl, x)}
			default:
				y := ebnf.Sequence{}
				for _, v := range x {
					switch vv := v.(type) {
					case *ebnf.Name, *ebnf.Token:
						y = append(y, v)
					default:
						panic(fmt.Sprintf("internal error %d %T(%v)", lvl, vv, vv))
					}
				}
				return y
			}
		default:
			panic(fmt.Sprintf("internal error %d %T(%v)", lvl, x, x))
		}
	}
	return f(0, expr0)
}

func (g Grammar) inlineBNF(what string, where map[string]bool) {
	inline := orthogonalBNF(g[what].Expr).(ebnf.Alternative)
	for name := range where {
		var f func(ebnf.Expression) ebnf.Expression
		f = func(expr ebnf.Expression) ebnf.Expression {
			switch x := expr.(type) {
			case nil:
				return nil
			case ebnf.Alternative:
				y := ebnf.Alternative{}
				for _, v := range x {
					fv := f(v)
					switch vv := fv.(type) {
					case ebnf.Alternative:
						y = append(y, vv...)
					case ebnf.Sequence:
						y = append(y, vv)
					default:
						panic(fmt.Sprintf("internal error %T(%v)", vv, vv))
					}
				}
				return y
			case ebnf.Sequence:
				if len(x) == 0 {
					return x
				}

				in := ebnf.Alternative{0: x}
				out := ebnf.Alternative{}
				for len(in) != 0 {
					seq := in[0].(ebnf.Sequence)
					in = in[1:]
					i := -1
				search:
					for j, v := range seq {
						if x, ok := v.(*ebnf.Name); ok && x.String == what {
							i = j
							break search
						}
					}
					switch {
					case i >= 0:
						switch {
						case len(inline) == 1: //TODO join w/ below
							y := ebnf.Sequence{}
							if i > 0 {
								y = append(y, seq[:i])
							}
							y = append(y, inline[0].(ebnf.Sequence)...)
							y = append(y, seq[i+1:]...)
							in = append(in, y)
						default:
							for _, v := range inline {
								y := ebnf.Sequence{}
								if i > 0 {
									y = append(y, seq[:i])
								}
								y = append(y, v.(ebnf.Sequence)...)
								y = append(y, seq[i+1:]...)
								in = append(in, y)
							}
						}
					default:
						out = append(out, seq)
					}
				}
				switch {
				case len(out) == 1:
					return out[0]
				default:
					return out
				}
			default:
				panic(fmt.Sprintf("internal error %T(%v)", x, x))
			}
		}
		g[name].Expr = NormalizeExpression(f(orthogonalBNF(g[name].Expr)))
	}
	delete(g, what)
}

func (g Grammar) inlineEBNF(what string, where map[string]bool) {
	for name := range where {
		var f func(*ebnf.Expression)
		f = func(expr *ebnf.Expression) {
			switch x := (*expr).(type) {
			case nil:
				// nop
			case ebnf.Alternative:
				for i := range x {
					f(&x[i])
				}
			case ebnf.Sequence:
				for i := range x {
					f(&x[i])
				}
			case *ebnf.Group:
				f(&x.Body)
			case *ebnf.Option:
				f(&x.Body)
			case *ebnf.Repetition:
				f(&x.Body)
			case *ebnf.Name:
				if x.String != what {
					break
				}

				*expr = &ebnf.Group{Body: NormalizeExpression(g[x.String].Expr)}
			case *ebnf.Token:
				// nop
			default:
				panic(fmt.Sprintf("internal error %T(%v)", x, x))
			}
		}
		prod := g[name]
		f(&prod.Expr)
		prod.Expr = NormalizeExpression(prod.Expr)
	}
	delete(g, what)
}

// Normalize returns a normalized clone of g. Positions are ignored.
func (g Grammar) Normalize() (r Grammar) {
	r = Grammar{}
	for name, prod := range g {
		r[name] = NormalizeProduction(prod)
	}
	return
}

func (g Grammar) str(expr ebnf.Expression) string {
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

	h(expr, false, false)
	return buf.String()
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

	a := []string{}
	for _, name := range term {
		a = append(a, g.str(g[name]))
	}

	if len(term) != 0 {
		a = append(a, "\n")
	}
	for _, name := range nterm {
		a = append(a, g.str(g[name]))
	}
	return strings.Join(a, "")
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
	// The grammar uses no groups (`( expr )`), options (`[ expr ]`) or repetitions (`{ expr }`).
	IsBNF bool
	// Set of lexical productions names.
	Lexical map[string]bool
	// Set of all ebnf.Token.String values
	Literals map[string]bool
	// Set of all non terminal productions names.
	NonTerminals map[string]bool
	// Set of all ebnf.Range.{Begin,End} pairs.
	Ranges map[struct{ Begin, End string }]bool
	// Set of all lexical production names referenced from within a
	// non-terminal production.
	Tokens map[string]bool
	// Used maps a production name to the count of its references.
	Used map[string]int
	// UsedBy maps a production name to its referencing production names
	// set ie. a cross-reference. For example a grammar:
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
	a := []string{}
	a = append(a, fmt.Sprintf("IsBNF: %t", r.IsBNF))
	a = append(a, fmt.Sprintf("Lexical: %s", str(r.Lexical)))
	a = append(a, fmt.Sprintf("Literals: %s", str(r.Literals)))
	a = append(a, fmt.Sprintf("NonTerminals: %s", str(r.NonTerminals)))
	aa := []string{}
	for v := range r.Ranges {
		aa = append(aa, fmt.Sprintf("%q … %q", v.Begin, v.End))
	}
	sort.Strings(aa)
	a = append(a, fmt.Sprintf("Ranges: %s", fmt.Sprintf("[%s]", strings.Join(aa, " "))))
	a = append(a, fmt.Sprintf("Tokens: %s", str(r.Tokens)))
	aa = []string{}
	for v := range r.UsedBy {
		aa = append(aa, v)
	}
	bb := []string{}
	for name, count := range r.Used {
		bb = append(bb, fmt.Sprintf("\t%q: %d", name, count))
	}
	sort.Strings(bb)
	a = append(a, fmt.Sprintf("Used:\n%s", strings.Join(bb, "\n")))
	bb = []string{}
	for _, v := range aa {
		aaa := []string{}
		for vv := range r.UsedBy[v] {
			aaa = append(aaa, fmt.Sprintf("%q", vv))
		}
		sort.Strings(aaa)
		bb = append(bb, fmt.Sprintf("\t%q: [%s]", v, strings.Join(aaa, " ")))
	}
	sort.Strings(bb)
	a = append(a, fmt.Sprintf("UsedBy:\n%s", strings.Join(bb, "\n")))
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
