// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package pdf

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestObjects(t *testing.T) {
	cases := []struct {
		v    any
		want string
	}{
		{nil, "null"},
		{true, "true"},
		{-3, "-3"},
		{612.0, "612"},
		{0.5, "0.5"},
		{-0.00001, "0"},
		{2.0 / 3, "0.6667"},
		{Name("A B#/"), "/A#20B#23#2F"},
		{String("a(b)\\"), `(a\(b\)\\)`},
		{String("\x00\xff"), "<00FF>"},
		{TextString("Ж"), "<FEFF0416>"},
		{Array{1, Ref(3), Name("X")}, "[1 3 0 R /X]"},
		{Dict{"b": 1, "a": Name("Z"), "c": nil}, "<</a /Z/b 1>>"},
	}
	for _, c := range cases {
		if got := string(appendObject(nil, c.v)); got != c.want {
			t.Errorf("%#v: got %s, want %s", c.v, got, c.want)
		}
	}
}

func TestWriter(t *testing.T) {
	var b bytes.Buffer
	p := NewWriter(&b)
	img := p.AddStream(Dict{"Type": Name("XObject"), "Subtype": Name("Image"), "Width": 8, "Height": 1,
		"ImageMask": true, "BitsPerComponent": 1}, []byte{0x55})
	p.AddPage(Page{Width: 612, Height: 792, Rotate: 90,
		Resources: Dict{"XObject": Dict{"Im": img}}, Content: []byte("q 612 0 0 792 0 0 cm /Im Do Q")})
	if err := p.Close(nil, Dict{"Producer": String("test")}); err != nil {
		t.Fatal(err)
	}
	s := b.String()
	if !strings.HasPrefix(s, "%PDF-1.5\n") || !strings.HasSuffix(s, "%%EOF\n") {
		t.Errorf("bad header or trailer")
	}
	q := NewWriter(&bytes.Buffer{})
	if q.Alloc(); q.Close(nil, nil) == nil {
		t.Error("unwritten object accepted")
	}
	if _, err := exec.LookPath("qpdf"); err != nil {
		t.Skip("no qpdf")
	}
	f := filepath.Join(t.TempDir(), "t.pdf")
	os.WriteFile(f, []byte(s), 0o644)
	if out, err := exec.Command("qpdf", "--check", f).CombinedOutput(); err != nil {
		t.Errorf("qpdf: %v\n%s", err, out)
	}
}

func TestPageTree(t *testing.T) {
	if _, err := exec.LookPath("qpdf"); err != nil {
		t.Skip("no qpdf")
	}
	for _, n := range []int{0, 1, 64, 65, 200} {
		var b bytes.Buffer
		p := NewWriter(&b)
		for range n {
			p.AddPage(Page{Width: 100, Height: 100})
		}
		if err := p.Close(nil, nil); err != nil {
			t.Fatal(err)
		}
		f := filepath.Join(t.TempDir(), "t.pdf")
		os.WriteFile(f, b.Bytes(), 0o644)
		out, err := exec.Command("qpdf", "--show-npages", f).CombinedOutput()
		if want := fmt.Sprint(n); err != nil || strings.TrimSpace(string(out)) != want {
			t.Errorf("%d pages: qpdf says %s %v", n, out, err)
		}
		if n > 0 {
			if out, err := exec.Command("qpdf", "--check", f).CombinedOutput(); err != nil {
				t.Errorf("%d pages: %s", n, out)
			}
		}
	}
}
