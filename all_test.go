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
	sort.Strings(a)
	states := map[string]*fsm.State{}
	for _, v := range a {
		states[v] = r.NewState()
		dbg("%d", states[v].Id()) //TODO bug in fsm
	}
	r.SetStart(states[start])

	var f func(ebnf.Expression, *fsm.State) *fsm.State
	f = func(expr ebnf.Expression, in *fsm.State) (out *fsm.State) {
		switch x := expr.(type) {
		case nil:
			// nop
		case *ebnf.Token:
			out := r.NewState()
			in.NewEdge(toks[" "+x.String], out)
		case *ebnf.Name:
			out := r.NewState()
			in.NewEdge(toks[x.String], out)
		default:
			panic(fmt.Sprintf("internal error %T(%v)", x, x))
		}
		return
	}

	for name, state := range states {
		f(g[name].Expr, state)
	}
	return
}

func TestNfa(t *testing.T) {
	return //TODO-
	table := []struct {
		src, exp string
	}{
		{`S = .`,
			`->[0]
`},
		{`S = "for" .`,
			`->[0]
	57344 -> [1]
[1]`},
		{`S = "f" .`,
			`->[0]
	102 -> [1]
[1]`},
		{
			`S = a .
a = "@" .`,
			`->[0]
	103 -> [1]
[1]`},
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

		if g, e := strings.TrimSpace(nfa.String()), strings.TrimSpace(test.exp); g != e {
			t.Errorf("----\ng:\n%s\n----\ne:\n%s", g, e)
		}

	}
}

func TestNfa2(t *testing.T) {
	//TODO from testdata
}
