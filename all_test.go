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
	"strings"
	"testing"
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

		rep, err := g.Analyze()
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

		r, err := g.Analyze()
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
				| "a"  .`,
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
				| S_1 "a"  .`,
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

		var i int
		g, _, err = g.BNF("Start", nil)

		if err != nil {
			t.Error(i, err)
			continue
		}

		t.Log(i, fname)
	}
}

func TestClone(t *testing.T) {
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

		g2 := g.Clone()
		if g, e := g2.String(), g.String(); g != e {
			t.Log(g)
			t.Log(e)
			t.Error(fname)
			continue
		}

		t.Log(i, fname)
	}
}

func _TestReduceEBNF0(t *testing.T) { //TODO
	table := []struct {
		src string
		all bool
		exp string
	}{
		{
			`S = R | "0"  | "1" .
			R = "A" | "Z" .
			Ebnf = ( "E" | "F" ) .`,
			false,
			`S = "A"
				| "Z"
				| "0"
				| "1"  .`,
		},
		{
			`S = "0" | R | "1" .
			R = "A" | "Z" .
			Ebnf = ( "E" | "F" ) .`,
			false,
			`S = "0"
				| "A"
				| "Z"
				| "1"  .`,
		},
		{
			`S = "0" | "1" | R .
			R = "A" | "Z" .
			Ebnf = ( "E" | "F" ) .`,
			false,
			`S = "0"
				| "1"
				| "A"
				| "Z"  .`,
		},
	}
	for i, test := range table {
		g, err := Parse(fmt.Sprintf("f%d", i), strings.NewReader(test.src))
		if err != nil {
			t.Error(i, err)
			continue
		}

		g2 := g.Clone()
		if err := g2._Reduce("S", test.all); err != nil {
			t.Error(i, err)
			continue
		}

		if g, e := g2.String(), test.exp; trimx(g) != trimx(e) {
			t.Errorf("----\ng:\n%s\n----\ne:\n%s", g, e)
		}

	}
}

func _TestReduceEBNF(t *testing.T) { //TODO
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

		g2 := g.Clone()
		if err = g2._Reduce("Start" /*TODOfalse*/, true); err != nil {
			t.Error(i, err)
			continue
		}

		sname := fname + ".reduced"
		ref, err := ioutil.ReadFile(sname)
		if err != nil {
			t.Errorf("%d/%d %v", i, len(testfiles), err)
			continue
		}

		if g, e := g2.String(), string(ref); g != e {
			t.Errorf("%d/%d\n----\ngot:\n%s\n----\nexp:\n%s", i, len(testfiles), g, e)
			continue
		}

		t.Log(i, fname)
	}
}
