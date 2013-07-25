// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ebnfutils

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"code.google.com/p/go.exp/ebnf"
	"github.com/cznic/fsm"
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
}

func TestString(t *testing.T) {
	for i, fname := range testfiles {
		fname = filepath.Join(testdata, fname)
		bsrc, err := ioutil.ReadFile(fname)
		if err != nil {
			t.Errorf("%d/%d %v", i, len(testfiles), err)
			continue
		}

		src := bytes.NewBuffer(bsrc)
		g, err := Parse(fname, src)
		if err != nil {
			t.Errorf("%d/%d %v", i, len(testfiles), err)
			continue
		}

		sname := fname[:len(fname)-len(".ebnf")] + ".string"
		ref, err := ioutil.ReadFile(sname)
		if err != nil {
			t.Errorf("%d/%d %v", i, len(testfiles), err)
			continue
		}

		if g, e := g.String(), string(ref); g != e {
			t.Errorf("%d/%d\n----\ngot:\n%s\n----\nexp:\n%s", i, len(testfiles), g, e)
			continue
		}

		t.Log(i, fname)
	}
}

func TestAnalyze(t *testing.T) {
	for i, fname := range testfiles {
		fname = filepath.Join(testdata, fname)
		bsrc, err := ioutil.ReadFile(fname)
		if err != nil {
			t.Errorf("%d/%d %v", i, len(testfiles), err)
			continue
		}

		src := bytes.NewBuffer(bsrc)
		g, err := Parse(fname, src)
		if err != nil {
			t.Errorf("%d/%d %v", i, len(testfiles), err)
			continue
		}

		sname := fname[:len(fname)-len(".ebnf")] + ".report"
		ref, err := ioutil.ReadFile(sname)
		if err != nil {
			t.Errorf("%d/%d %v", i, len(testfiles), err)
			continue
		}

		r, err := g.Analyze("Start")
		if err != nil {
			t.Errorf("%d/%d %v", i, len(testfiles), err)
			continue
		}

		if g, e := r.String(), string(ref); g != e {
			t.Errorf("%d/%d\n----\ngot:\n%s\n----\nexp:\n%s", i, len(testfiles), g, e)
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

func (g Grammar) nfa(start string) (r *fsm.NFA, err error) {
	rep, err := g.Analyze(start)
	if err != nil {
		return
	}

	a := []string{}
	for v := range rep.Literals {
		a = append(a, " "+v)
	}
	for v := range rep.Tokens {
		a = append(a, v)
	}
	for v := range rep.NonTerminals {
		a = append(a, v)
	}
	sort.Strings(a)
	toks := map[string]int{}
	for i, v := range a {
		switch v[0] {
		case ' ':
			switch len(v) {
			case 2:
				toks[v] = int(v[1])
			default:
				toks[v] = 0xe000 + i
			}
		default:
			toks[v] = 0xe000 + i
		}
	}

	r = fsm.NewNFA()
	a = []string{}
	for v := range rep.NonTerminals {
		a = append(a, v)
	}
	for v := range rep.Tokens {
		a = append(a, v)
	}
	sort.Strings(a)
	states := map[string]*fsm.State{}
	for _, v := range a {
		states[v] = r.NewState()
	}
	r.SetStart(states[start])
	dead := r.NewState()
	dead.NewEdge(fsm.Epsilon, dead)
	for v := range rep.Tokens {
		states[v].NewEdge(fsm.Epsilon, dead)
	}

	var f func(ebnf.Expression, *fsm.State) *fsm.State
	f = func(expr ebnf.Expression, in *fsm.State) (out *fsm.State) {
		switch x := expr.(type) {
		case nil:
			out = dead
			in.NewEdge(fsm.Epsilon, out)
		case ebnf.Alternative:
			out = in
			for _, v := range x {
				if out == in {
					out = r.NewState()
				}
				i := r.NewState()
				in.NewEdge(fsm.Epsilon, i)
				o := f(v, i)
				o.NewEdge(fsm.Epsilon, out)
			}
		case ebnf.Sequence:
			for _, v := range x {
				in = f(v, in)
			}
			out = in
		case *ebnf.Option:
			out = f(x.Body, in)
			in.NewEdge(fsm.Epsilon, out)
		case *ebnf.Repetition:
			out = f(x.Body, in)
			in.NewEdge(fsm.Epsilon, out)
			out.NewEdge(fsm.Epsilon, in)
		case *ebnf.Group:
			in2 := r.NewState()
			in.NewEdge(fsm.Epsilon, in2)
			out0 := f(x.Body, in2)
			out = r.NewState()
			out0.NewEdge(fsm.Epsilon, out)
		case *ebnf.Token:
			out = r.NewState()
			in.NewEdge(toks[" "+x.String], out)
		case *ebnf.Name:
			out = r.NewState()
			nm := x.String
			in.NewEdge(toks[nm], out)
			if s := states[nm]; s != nil {
				in.NewEdge(fsm.Epsilon, s)
			}
		default:
			panic(fmt.Sprintf("internal error %T(%v)", x, x))
		}
		return
	}

	for name, state := range states {
		if !rep.NonTerminals[name] {
			continue
		}

		out := f(g[name].Expr, state)
		out.IsAccepting = name == start
	}
	return
}

func TestNfa(t *testing.T) {
	table := []struct {
		src, exp string
	}{
		{
			`S = .`,
			`->[0]
				ε -> [1]
			[[1]]
				ε -> [1]`,
		},
		{
			`S = "for" .`,
			`->[0]
				57344 -> [2]
			[1]
				ε -> [1]
			[[2]]`,
		},
		{
			`S = "f" .`,
			`->[0]
				102 -> [2]
			[1]
				ε -> [1]
			[[2]]`,
		},
		{
			`S = a .
			a = .`,
			`->[0]
				ε -> [1]
				57345 -> [3]
			[1]
				ε -> [2]
			[2]
				ε -> [2]
			[[3]]`,
		},
		{
			`S = a .
			a = "@" .`,
			`->[0]
				ε -> [1]
				57346 -> [3]
			[1]
				ε -> [2]
			[2]
				ε -> [2]
			[[3]]`,
		},
		{
			`S = "A" | "B" .`,
			`->[0]
				ε -> [3] [5]
			[1]
				ε -> [1]
			[[2]]
			[3]
				65 -> [4]
			[4]
				ε -> [2]
			[5]
				66 -> [6]
			[6]
				ε -> [2]`,
		},
		{
			`S = a .
			a = "A" | "B" .`,
			`->[0]
			ε -> [1]
			57347 -> [3]
		[1]
			ε -> [2]
		[2]
			ε -> [2]
		[[3]]`,
		},
		{
			`S = "A" "B" .`,
			`->[0]
				65 -> [2]
			[1]
				ε -> [1]
			[2]
				66 -> [3]
			[[3]]`,
		},
		{
			`S = a .
			a = "A" "B" .`,
			`->[0]
				ε -> [1]
				57347 -> [3]
			[1]
				ε -> [2]
			[2]
				ε -> [2]
			[[3]]`,
		},
		{
			`S = e | S "A" .
			e = .`,
			`->[0]
				ε -> [4] [6]
			[1]
				ε -> [2]
			[2]
				ε -> [2]
			[[3]]
			[4]
				ε -> [1]
				57346 -> [5]
			[5]
				ε -> [3]
			[6]
				ε -> [0]
				57345 -> [7]
			[7]
				65 -> [8]
			[8]
				ε -> [3]`,
		},
		{
			`S = [ "A" ] .`,
			`->[0]
				ε -> [2]
				65 -> [2]
			[1]
				ε -> [1]
			[[2]]`,
		},
		{
			`S = { "A" } .`,
			`->[0]
				ε -> [2]
				65 -> [2]
			[1]
				ε -> [1]
			[[2]]
				ε -> [0]`,
		},
		{
			`S = { "A" } "B" .`,
			`->[0]
				ε -> [2]
				65 -> [2]
			[1]
				ε -> [1]
			[2]
				ε -> [0]
				66 -> [3]
			[[3]]`,
		},
		{
			`S = "@" { "A" } .`,
			`->[0]
				64 -> [2]
			[1]
				ε -> [1]
			[2]
				ε -> [3]
				65 -> [3]
			[[3]]
				ε -> [2]`,
		},
		{
			`S = "@" { "A" } "B" .`,
			`->[0]
				64 -> [2]
			[1]
				ε -> [1]
			[2]
				ε -> [3]
				65 -> [3]
			[3]
				ε -> [2]
				66 -> [4]
			[[4]]`,
		},
		{
			`S = ( "A" ) .`,
			`->[0]
				ε -> [2]
			[1]
				ε -> [1]
			[2]
				65 -> [3]
			[3]
				ε -> [4]
			[[4]]`,
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

		nfa, err := g.nfa("S")
		if err != nil {
			t.Error(i, err)
			continue
		}

		if g, e := nfa.String(), test.exp; trimx(g) != trimx(e) {
			t.Errorf("----\ng:\n%s\n----\ne:\n%s", g, e)
		}

	}
}

func TestNfa2(t *testing.T) {
	for i, fname := range testfiles {
		fname = filepath.Join(testdata, fname)
		bsrc, err := ioutil.ReadFile(fname)
		if err != nil {
			t.Errorf("%d/%d %v", i, len(testfiles), err)
			continue
		}

		src := bytes.NewBuffer(bsrc)
		g, err := Parse(fname, src)
		if err != nil {
			t.Errorf("%d/%d %v", i, len(testfiles), err)
			continue
		}

		if err = g.Verify("Start"); err != nil {
			t.Error(i, err)
			continue
		}

		_, err = g.nfa("Start")
		if err != nil {
			t.Error(i, err)
			continue
		}

		t.Log(i, fname)
	}
}
