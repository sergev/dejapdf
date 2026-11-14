// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package djvu

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// dsed holds the set-ant and set-txt blocks of `djvused -e output-all`
// by component id.
type dsed map[string]map[string]string

func readDsed(b []byte) dsed {
	r := dsed{}
	id, cmd := "", ""
	var buf strings.Builder
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(nil, 1<<24)
	for sc.Scan() {
		l := sc.Text()
		switch {
		case cmd != "" && l == ".":
			if r[id] == nil {
				r[id] = map[string]string{}
			}
			r[id][cmd] = buf.String()
			cmd = ""
			buf.Reset()
		case cmd != "":
			buf.WriteString(l + "\n")
		case strings.HasPrefix(l, `select "`):
			id = l[8:strings.LastIndexByte(l, '"')]
		case l == "set-ant" || l == "set-txt":
			cmd = l
		}
	}
	return r
}

func list(head string, items ...*Sexp) *Sexp {
	return &Sexp{IsList: true, List: append([]*Sexp{{Atom: head}}, items...)}
}

func str(s string) *Sexp { return &Sexp{Atom: s, Quoted: true} }

// zoneSexp prints a zone as djvused print-txt does: text on leaves only,
// without separators.
func zoneSexp(z *Zone) *Sexp {
	s := list(z.Type.String())
	for _, v := range []int{z.X0, z.Y0, z.X1, z.Y1} {
		s.List = append(s.List, &Sexp{Atom: strconv.Itoa(v)})
	}
	if len(z.Children) == 0 {
		s.List = append(s.List, str(strings.TrimRight(z.Text, " \n")))
	}
	for _, k := range z.Children {
		s.List = append(s.List, zoneSexp(k))
	}
	return s
}

func outlineSexp(bs []*Bookmark) []*Sexp {
	var r []*Sexp
	for _, b := range bs {
		s := &Sexp{IsList: true, List: []*Sexp{str(b.Title), str(b.URL)}}
		s.List = append(s.List, outlineSexp(b.Children)...)
		r = append(r, s)
	}
	return r
}

func TestTextAnnoOutlineCorpus(t *testing.T) {
	goldens, _ := filepath.Glob(filepath.Join(corpus, "*.dsed"))
	if len(goldens) == 0 {
		t.Fatal("no goldens")
	}
	for _, g := range goldens {
		base := strings.TrimSuffix(g, ".dsed")
		nt, na := checkDoc(t, base+".djvu", mustRead(t, g), mustRead(t, base+".outline"))
		if nt == 0 || na == 0 {
			t.Errorf("%s: %d text, %d annotations", g, nt, na)
		}
	}
}

// TestBooks checks the files in $DJVU_BOOKS (a glob) against djvused.
func TestBooks(t *testing.T) {
	files, _ := filepath.Glob(os.Getenv("DJVU_BOOKS"))
	if len(files) == 0 {
		t.Skip("DJVU_BOOKS not set")
	}
	for _, f := range files {
		ds, err := exec.Command("djvused", f, "-e", "output-all").Output()
		if err != nil {
			t.Fatal(err)
		}
		ol, err := exec.Command("djvused", f, "-e", "print-outline").Output()
		if err != nil {
			t.Fatal(err)
		}
		nt, na := checkDoc(t, f, ds, ol)
		t.Logf("%s: %d pages with text, %d annotations", filepath.Base(f), nt, na)
	}
}

// checkDoc compares the text, annotations and outline of a bundled or
// indirect document with djvused output-all and print-outline output.
// It returns the number of text layers and annotations seen.
func checkDoc(t *testing.T, path string, dsed, outline []byte) (nt, na int) {
	t.Helper()
	want := readDsed(dsed)
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	out, err := d.Outline()
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	got, w := "", ""
	if out != nil {
		got = list("bookmarks", outlineSexp(out)...).String()
	}
	if e := ParseSexps(outline); len(e) > 0 {
		w = e[0].String()
	}
	if got != w {
		t.Errorf("%s outline:\n got %s\nwant %s", path, got, w)
	}
	for _, c := range d.Dir.Components {
		f, err := d.component(c)
		if err != nil {
			t.Fatal(err)
		}
		w := want[c.ID]
		z, err := textOf(f)
		if err != nil {
			t.Fatalf("%s %s: %v", path, c.ID, err)
		}
		got, wt := "", ""
		if z != nil {
			got = zoneSexp(z).String()
			nt++
		}
		if e := ParseSexps([]byte(w["set-txt"])); len(e) > 0 {
			wt = e[0].String()
		}
		if got != wt {
			t.Errorf("%s %s text:\n got %s\nwant %s", path, c.ID, got, wt)
		}
		a, err := annoOf(f)
		if err != nil {
			t.Fatalf("%s %s: %v", path, c.ID, err)
		}
		got, wa := "", ""
		for _, s := range a {
			got += s.String() + "\n"
			na++
		}
		for _, s := range ParseSexps([]byte(w["set-ant"])) {
			wa += s.String() + "\n"
		}
		if got != wa {
			t.Errorf("%s %s annotations:\n got %s\nwant %s", path, c.ID, got, wa)
		}
	}
	return nt, na
}

func TestParseSexps(t *testing.T) {
	in := `(maparea "#3" "a \"b\"\n\101\\" (rect 1 2 3 4) (none)) ) (zoom page) (broken "x"`
	e := ParseSexps([]byte(in))
	if len(e) != 2 || e[0].Head() != "maparea" || e[1].String() != "(zoom page)" {
		t.Fatalf("got %v", e)
	}
	if s := e[0].List[2].Atom; s != "a \"b\"\nA\\" {
		t.Errorf("string %q", s)
	}
	if s := e[0].String(); s != `(maparea "#3" "a \"b\"\nA\\" (rect 1 2 3 4) (none))` {
		t.Errorf("format %s", s)
	}
}

func TestParseTextErrors(t *testing.T) {
	ok := []byte{0, 0, 2, 'h', 'i', 1,
		1, 0x80, 0, 0x80, 0, 0x80, 10, 0x80, 10, 0x80, 0, 0, 0, 2, 0, 0, 1,
		6, 0x80, 1, 0x80, 2, 0x80, 3, 0x80, 4, 0x80, 0, 0, 0, 2, 0, 0, 0}
	z, err := ParseText(ok)
	if err != nil {
		t.Fatal(err)
	}
	w := z.Children[0]
	if w.Type != ZWord || w.Text != "hi" || w.X0 != 1 || w.Y0 != 10-2-4 || w.X1 != 4 || w.Y1 != 8 {
		t.Errorf("word %+v", *w)
	}
	for _, n := range []int{2, 5, 10, len(ok) - 1} {
		if _, err := ParseText(ok[:n]); err == nil {
			t.Errorf("truncated at %d accepted", n)
		}
	}
	bad := bytes.Clone(ok)
	bad[len(bad)-4] = 3 // word text longer than the page text
	if _, err := ParseText(bad); err == nil {
		t.Error("text overflow accepted")
	}
}

func TestParseOutlineErrors(t *testing.T) {
	ok := []byte{0, 2, 1, 0, 0, 1, 'A', 0, 0, 2, '#', '1', 0, 0, 0, 1, 'B', 0, 0, 0}
	o, err := ParseOutline(ok)
	if err != nil || len(o) != 1 || o[0].URL != "#1" || o[0].Children[0].Title != "B" {
		t.Fatalf("got %v, %v", o, err)
	}
	for n := range len(ok) - 1 {
		if _, err := ParseOutline(ok[:n]); err == nil {
			t.Errorf("truncated at %d accepted", n)
		}
	}
	bad := bytes.Clone(ok)
	bad[1] = 1 // child beyond the total
	if _, err := ParseOutline(bad); err == nil {
		t.Error("bad count accepted")
	}
}
