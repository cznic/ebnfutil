// Copyright 2014 The ebnfutil Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ebnfutil

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"code.google.com/p/go.exp/ebnf"
	"github.com/cznic/strutil"
)

func dbg(s string, va ...interface{}) {
	_, fn, fl, _ := runtime.Caller(1)
	fmt.Printf("%s:%d: ", path.Base(fn), fl)
	fmt.Printf(s, va...)
	fmt.Println()
}

const testdata = "testdata"

var testfiles []string

func init() {
	f, err := os.Open(testdata)
	if err != nil {
		panic(err)
	}

	names, err := f.Readdirnames(0)
	if err != nil {
		panic(err)
	}

	for _, name := range names {
		if filepath.Ext(name) == ".ebnf" {
			testfiles = append(testfiles, name)
		}
	}

	if len(testfiles) == 0 {
		panic("internal error: missing testdata")
	}

	tests = true
}

func (g Grammar) dstr(expr ebnf.Expression) string {
	var buf bytes.Buffer
	f := strutil.IndentFormatter(&buf, "\t")

	var h func(ebnf.Expression)
	h = func(expr ebnf.Expression) {
		switch x := expr.(type) {
		case nil:
			f.Format(" <nil>")
		case *ebnf.Production:
			name := x.Name.String
			f.Format("%s =%i", name)
			h(g[name].Expr)
			f.Format(" .%u\n")
		case ebnf.Alternative:
			for i, v := range x {
				switch {
				case i == 0:
					f.Format(" <A>%i\n")
				default:
					f.Format("\n|")
				}
				h(v)
			}
			f.Format("</A>%u")
		case ebnf.Sequence:
			f.Format(" <S>")
			for _, v := range x {
				h(v)
			}
			f.Format("</S>")
		case *ebnf.Group:
			f.Format(" (%i\n")
			h(x.Body)
			f.Format("%u\n)")
		case *ebnf.Option:
			f.Format(" [%i\n")
			h(x.Body)
			f.Format("%u\n]")
		case *ebnf.Repetition:
			f.Format(" {%i\n")
			h(x.Body)
			f.Format("%u\n}")
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

	h(expr)
	return buf.String()
}

func TestString(t *testing.T) {
	for i, fname := range testfiles {
		fname = filepath.Join(testdata, fname)
		bsrc, err := ioutil.ReadFile(fname)
		if err != nil {
			t.Error(err)
			continue
		}

		src := bytes.NewBuffer(bsrc)
		g, err := Parse(fname, src)
		if err != nil {
			t.Error(err)
			continue
		}

		sname := fname[:len(fname)-len(".ebnf")] + ".string"
		ref, err := ioutil.ReadFile(sname)
		if err != nil {
			t.Error(err)
			continue
		}

		if g, e := g.String(), string(ref); g != e {
			t.Errorf("----\ngot:\n%s\n----\nexp:\n%s", g, e)
			continue
		}

		t.Log(i, fname)
	}
}

func TestIsBNF(t *testing.T) {
	table := []struct {
		src string
		exp bool
	}{
		{
			`S = .`,
			true,
		},
		{
			`S = a .
			a = .`,
			true,
		},
		{
			`S = a | b .
			a = .
			b = .`,
			true,
		},
		{
			`S = a | b [ c ] .
			a = .
			b = .
			c = .`,
			false,
		},
		{
			`S = a | b ( c ) .
			a = .
			b = .
			c = .`,
			false,
		},
		{
			`S = a | b { c } .
			a = .
			b = .
			c = .`,
			false,
		},
	}

	for i, test := range table {
		g, err := Parse(fmt.Sprintf("f%d", i), strings.NewReader(test.src))
		if err != nil {
			t.Error(i, err)
			continue
		}

		if err = g.Verify("S"); err != nil {
			t.Error(i, err)
			continue
		}

		rep, err := g.Analyze("")
		if err != nil {
			t.Error(i, err)
			continue
		}

		if g, e := rep.IsBNF, test.exp; g != e {
			t.Error(i)
		}
	}
}

func TestAnalyze(t *testing.T) {
	for i, fname := range testfiles {
		fname = filepath.Join(testdata, fname)
		bsrc, err := ioutil.ReadFile(fname)
		if err != nil {
			t.Error(err)
			continue
		}

		src := bytes.NewBuffer(bsrc)
		g, err := Parse(fname, src)
		if err != nil {
			t.Error(err)
			continue
		}

		sname := fname[:len(fname)-len(".ebnf")] + ".report"
		ref, err := ioutil.ReadFile(sname)
		if err != nil {
			t.Error(err)
			continue
		}

		r, err := g.Analyze("")
		if err != nil {
			t.Error(err)
			continue
		}

		if g, e := r.String(), string(ref); g != e {
			t.Errorf("----\ngot:\n%s\n----\nexp:\n%s", g, e)
			continue
		}

		t.Log(i, fname)
	}
}

func trimx(s string) string {
	a := strings.Split(s, "\n")
	b := []string{}
	for _, v := range a {
		if s = strings.TrimSpace(v); s != "" {
			b = append(b, s)
		}
	}
	return strings.Join(b, "\n")
}

func TestBNF0(t *testing.T) {
	table := []struct {
		src, exp string
	}{
		{
			`S = .`,
			`S = .`,
		},
		{
			`S = "a" .`,
			`S = "a" .`,
		},
		{
			`S = [ "a" ] .`,
			`S = S_1 .
			S_1 = 
				| "a" .`,
		},
		{
			`S = A .
			A = "A" .`,
			`A = "A" .
			S = A .`,
		},
		{
			`S = { "a" } .`,
			`S = S_1 .
			S_1 = 
				| S_1 "a" .`,
		},
		{
			`S = "0" … "9" .`,
			`S = "0" … "9" .`,
		},
		{
			`S = ( "A" ) .`,
			`S = S_1 .
			S_1 = "A" .`,
		},
	}
	for i, test := range table {
		g, err := Parse(fmt.Sprintf("f%d", i), strings.NewReader(test.src))
		if err != nil {
			t.Error(i, err)
			continue
		}

		if err = g.Verify("S"); err != nil {
			t.Error(i, err)
			continue
		}

		var i int
		g2, _, err := g.BNF("S", nil)
		if err != nil {
			t.Error(i, err)
			continue
		}

		if g, e := g2.String(), test.exp; trimx(g) != trimx(e) {
			t.Errorf("----\ng:\n%s\n----\ne:\n%s", g, e)
		}

	}
}

func TestBNF1(t *testing.T) {
	for i, fname := range testfiles {
		fname = filepath.Join(testdata, fname)
		bsrc, err := ioutil.ReadFile(fname)
		if err != nil {
			t.Error(err)
			continue
		}

		src := bytes.NewBuffer(bsrc)
		g, err := Parse(fname, src)
		if err != nil {
			t.Error(err)
			continue
		}

		if err = g.Verify("Start"); err != nil {
			t.Error(i, err)
			continue
		}

		g, _, err = g.BNF("Start", nil)
		if err != nil {
			t.Error(i, err)
			continue
		}

		sname := fname + ".bnf"
		ref, err := ioutil.ReadFile(sname)
		if err != nil {
			t.Error(err)
			continue
		}

		if g, e := g.String(), string(ref); g != e {
			t.Errorf("n----\ngot:\n%s\n----\nexp:\n%s", g, e)
			continue
		}

		t.Log(i, fname)
	}
}

func TestNormalize(t *testing.T) {
	for i, fname := range testfiles {
		fname = filepath.Join(testdata, fname)
		bsrc, err := ioutil.ReadFile(fname)
		if err != nil {
			t.Error(err)
			continue
		}

		src := bytes.NewBuffer(bsrc)
		g, err := Parse(fname, src)
		if err != nil {
			t.Error(err)
			continue
		}

		g2 := g.Normalize()
		if g, e := g2.String(), g.String(); g != e {
			t.Log(g)
			t.Log(e)
			t.Error(fname)
			continue
		}

		t.Log(i, fname)
	}
}

func TestInlineEBNF0(t *testing.T) {
	table := []struct {
		src string
		all bool
		exp string
	}{
		{
			`S = R | "0" | "1" .
			R = "A" | "Z" .
			Ebnf = ( "E" | "F" ) .`,
			false,
			`Ebnf = ( "E" | "F" ) .
			S = "A"
				| "Z"
				| "0"
				| "1" .`,
		},
		{
			`S = "0" | R | "1" .
			R = "A" | "Z" .
			Ebnf = ( "E" | "F" ) .`,
			false,
			`Ebnf = ( "E" | "F" ) .
			S = "0"
				| "A"
				| "Z"
				| "1" .`,
		},
		{
			`S = "0" | "1" | R .
			R = "A" | "Z" .
			Ebnf = ( "E" | "F" ) .`,
			false,
			`Ebnf = ( "E" | "F" ) .
			S = "0"
				| "1"
				| "A"
				| "Z" .`,
		},
		{
			`S = "0" | One | "2" .
			One = ( "f" "function" ) .
			Ebnf = ( "E" | "F" ) .`,
			false,
			`Ebnf = ( "E" | "F" ) .
			S = "0"
				| "f" "function"
				| "2" .`,
		},
		{
			`Empty = .
			S = Empty | { "1" "2" } .`,
			false,
			`S =
				| { "1" "2" } .`,
		},
		{
			`S = "A" B .
			B = "B" { "C" } .`,
			false,
			`S = "A" "B" { "C" } .`,
		},
	}
	for i, test := range table {
		g, err := Parse(fmt.Sprintf("f%d", i), strings.NewReader(test.src))
		if err != nil {
			t.Error(i, err)
			continue
		}

		g2 := g.Normalize()
		if err := g2.Inline("S", test.all); err != nil {
			t.Error(i, err)
			continue
		}

		if g, e := g2.String(), test.exp; trimx(g) != trimx(e) {
			t.Errorf("----\ng:\n%s\n----\ne:\n%s", g, e)
		}

	}
}

func TestInlineEBNF1(t *testing.T) {
	for i, fname := range testfiles {
		fname = filepath.Join(testdata, fname)
		bsrc, err := ioutil.ReadFile(fname)
		if err != nil {
			t.Error(err)
			continue
		}

		src := bytes.NewBuffer(bsrc)
		g, err := Parse(fname, src)
		if err != nil {
			t.Error(err)
			continue
		}

		g2 := g.Normalize()
		if err = g2.Inline("Start", false); err != nil {
			t.Error(i, err)
			continue
		}

		sname := fname + ".reduced"
		ref, err := ioutil.ReadFile(sname)
		if err != nil {
			t.Error(err)
			continue
		}

		if g, e := g2.String(), string(ref); g != e {
			t.Errorf("----\ngot:\n%s\n----\nexp:\n%s", g, e)
			continue
		}

		t.Log(i, fname)
	}
}

func TestInlineEBNF2(t *testing.T) {
	for i, fname := range testfiles {
		fname = filepath.Join(testdata, fname)
		bsrc, err := ioutil.ReadFile(fname)
		if err != nil {
			t.Error(err)
			continue
		}

		src := bytes.NewBuffer(bsrc)
		g, err := Parse(fname, src)
		if err != nil {
			t.Error(err)
			continue
		}

		g2 := g.Normalize()
		if err = g2.Inline("Start", true); err != nil {
			t.Error(i, err)
			continue
		}

		sname := fname + ".inlined"
		ref, err := ioutil.ReadFile(sname)
		if err != nil {
			t.Error(err)
			continue
		}

		if g, e := g2.String(), string(ref); g != e {
			t.Errorf("----\ngot:\n%s\n----\nexp:\n%s", g, e)
			continue
		}

		t.Log(i, fname)
	}
}

func TestInlineBNF0(t *testing.T) {
	table := []struct {
		src string
		all bool
		exp string
	}{
		{
			`S = R | "0" | "1" .
			R = "A" | "Z" .`,
			false,
			`S = "A"
				| "Z"
				| "0"
				| "1" .`,
		},
		{
			`S = "0" | R | "1" .
			R = "A" | "Z" .`,
			false,
			`S = "0"
				| "A"
				| "Z"
				| "1" .`,
		},
		{
			`S = "0" | "1" | R .
			R = "A" | "Z" .`,
			false,
			`S = "0"
				| "1"
				| "A"
				| "Z" .`,
		},
		{
			`S = "0" | One | "2" .
			One = "f" "function" .`,
			false,
			`S = "0"
				| "f" "function"
				| "2" .`,
		},
		{
			`Empty = .
			S = Empty | "1" "2" .`,
			false,
			`S =
				| "1" "2" .`,
		},
		{
			`S = "A" B .
			B = "B" "C" .`,
			false,
			`S = "A" "B" "C" .`,
		},
	}
	for i, test := range table {
		g, err := Parse(fmt.Sprintf("f%d", i), strings.NewReader(test.src))
		if err != nil {
			t.Error(i, err)
			continue
		}

		g2 := g.Normalize()
		if err := g2.Inline("S", test.all); err != nil {
			t.Error(i, err)
			continue
		}

		if g, e := g2.String(), test.exp; trimx(g) != trimx(e) {
			t.Errorf("----\ng:\n%s\n----\ne:\n%s", g, e)
		}

	}
}

func TestInlineBNF1(t *testing.T) {
	for i, fname := range testfiles {
		fname = filepath.Join(testdata, fname)
		bsrc, err := ioutil.ReadFile(fname)
		if err != nil {
			t.Error(err)
			continue
		}

		src := bytes.NewBuffer(bsrc)
		g0, err := Parse(fname, src)
		if err != nil {
			t.Error(err)
			continue
		}

		g1, _, err := g0.BNF("Start", nil)
		if err != nil {
			t.Error(err)
			continue
		}

		g2 := g1.Normalize()
		if err = g2.Inline("Start", false); err != nil {
			t.Error(i, err)
			continue
		}

		sname := fname + ".bnf.reduced"
		ref, err := ioutil.ReadFile(sname)
		if err != nil {
			t.Error(err)
			continue
		}

		if g, e := g2.String(), string(ref); g != e {
			t.Errorf("got:\n%s\n----\nexp:\n%s", g2, e)
			continue
		}

		t.Log(i, fname)
	}
}

func TestInlineBNF2(t *testing.T) {
	for i, fname := range testfiles {
		fname = filepath.Join(testdata, fname)
		bsrc, err := ioutil.ReadFile(fname)
		if err != nil {
			t.Error(err)
			continue
		}

		src := bytes.NewBuffer(bsrc)
		g0, err := Parse(fname, src)
		if err != nil {
			t.Error(err)
			continue
		}

		g1, _, err := g0.BNF("Start", nil)
		if err != nil {
			t.Error(err)
			continue
		}

		g2 := g1.Normalize()
		if err = g2.Inline("Start", true); err != nil {
			t.Error(i, err)
			continue
		}

		sname := fname + ".bnf.inlined"
		ref, err := ioutil.ReadFile(sname)
		if err != nil {
			t.Error(err)
			continue
		}

		if g, e := g2.String(), string(ref); g != e {
			t.Errorf("got:\n%s\n----\nexp:\n%s", g2, e)
			continue
		}

		t.Log(i, fname)
	}
}

func TestBug(t *testing.T) {
	const src = `// Copyright (c) 2013 ebnfutil Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

float		= . // http://golang.org/ref/spec#float_lit
identifier	= . // ASCII letters, digits, "_". No front digit.
imaginary	= . // http://golang.org/ref/spec#imaginary_lit
integer		= . // http://golang.org/ref/spec#int_lit
str		= . // http://golang.org/ref/spec#string_lit
boolean		= "true" | "false" .

andnot 	= "&^" .
lsh 	= "<<" .
rsh 	= ">>" .

Expression = Term  { ( "^" | "|" | "-" | "+" ) Term } .
ExpressionList = Expression { "," Expression } .
Factor = [ "^" | "!" | "-" | "+" ] Operand .
Literal = boolean
	| float
	| QualifiedIdent
	| imaginary
	| integer
	| str .
Term = Factor { ( andnot | "&" | lsh  | rsh | "%" | "/" | "*" ) Factor } .
Operand = Literal
        | QualifiedIdent "(" [ ExpressionList ] ")"
        | "(" Expression ")" .
QualifiedIdent = identifier [ "." identifier ] .`

	for ie := 0; ie < 3; ie++ {
		for iy := 0; iy < 3; iy++ {
			r := bytes.NewBufferString(src)
			g0, err := Parse("bug", r)
			if err != nil {
				t.Fatal(err)
			}

			if err = g0.Verify("Expression"); err != nil {
				t.Fatal(err)
			}

			switch ie {
			case 0:
				// nop
			case 1:
				if err = g0.Inline("Expression", false); err != nil {
					t.Error(err)
					continue
				}
			case 2:
				if err = g0.Inline("Expression", true); err != nil {
					t.Error(err)
					continue
				}
			default:
				t.Fatal(ie, "internal error")
			}

			g1, _, err := g0.BNF("Expression", nil)
			if err != nil {
				t.Error(err)
				continue
			}

			switch iy {
			case 0:
				// nop
			case 1:
				if err = g1.Inline("Expression", false); err != nil {
					t.Error(err)
					continue
				}
			case 2:
				if err = g1.Inline("Expression", true); err != nil {
					t.Error(err)
					continue
				}
			default:
				t.Fatal(iy, "internal error")
			}
		}
	}
}
