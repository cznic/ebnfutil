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
	"testing"

	"code.google.com/p/go.exp/ebnf"
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
		g, err := ebnf.Parse(fname, src)
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

		if g, e := Grammar(g).String(), string(ref); g != e {
			t.Errorf("%d/%d\n----\ngot:\n%s\n----\nexp:\n%s", i, len(testfiles), g, e)
			continue
		}

		t.Log(i, fname)
	}
}
