// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package pdfread

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/sergev/dejapdf/internal/pdf"
)

func TestLexer(t *testing.T) {
	tests := []struct {
		in   string
		want any
	}{
		{"42", 42},
		{"-17", -17},
		{"+3", 3},
		{"3.5", 3.5},
		{"-.25", -0.25},
		{"--4", 4.0},
		{"1.2.3", 1.2},
		{"true", true},
		{"null", nil},
		{"/Name", pdf.Name("Name")},
		{"/A#20B#2", pdf.Name("A B#2")},
		{"/", pdf.Name("")},
		{"(a(b)c)", pdf.String("a(b)c")},
		{`(\n\r\t\b\f\(\)\\\101\7x\0123)`, pdf.String("\n\r\t\b\f()\\A\ax\n3")},
		{"(a\\\r\nb)", pdf.String("ab")},
		{"(a\r\nb\rc)", pdf.String("a\nb\nc")},
		{"<41 42 4>", pdf.String("AB@")},
		{"<>", pdf.String("")},
		{"[1 2 R 3]", pdf.Array{pdf.Ref(1), 3}},
		{"[1 2 3 R]", pdf.Array{1, pdf.Ref(2)}},
		{"[1 2]", pdf.Array{1, 2}},
		{"<</A 1 0 R/B[/C]/D null/E<<>>>>", pdf.Dict{"A": pdf.Ref(1), "B": pdf.Array{pdf.Name("C")}, "E": pdf.Dict{}}},
		{"<</A/B/C>>", pdf.Dict{"A": pdf.Name("B")}},
		{"% comment\n7", 7},
		{"obj", keyword("obj")},
	}
	for _, tt := range tests {
		l := &lexer{b: []byte(tt.in)}
		got, err := l.object(0)
		if err != nil || !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%q: got %#v, %v; want %#v", tt.in, got, err, tt.want)
		}
	}
	for _, in := range []string{"", "]", ")", "[1 2", "<</A 1", strings.Repeat("[", 1000)} {
		l := &lexer{b: []byte(in)}
		if _, err := l.object(0); err == nil {
			t.Errorf("%q: no error", in)
		}
	}
}

// build assembles a PDF from objects 1..n. kind is "table", "stream"
// or "hybrid"; stm lists objects to put into an object stream.
func build(objs []string, kind string, stm map[int]bool) []byte {
	var b bytes.Buffer
	b.WriteString("%PDF-1.5\n%\xe2\xe3\xcf\xd3\n")
	n := len(objs)
	offs := make([]int, n+3)
	var inStm []int
	for i, o := range objs {
		if stm[i+1] {
			inStm = append(inStm, i+1)
			continue
		}
		offs[i+1] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	stmNum := n + 1
	if len(inStm) > 0 {
		var head, body bytes.Buffer
		for _, k := range inStm {
			fmt.Fprintf(&head, "%d %d ", k, body.Len())
			body.WriteString(objs[k-1] + "\n")
		}
		data := zip(append(head.Bytes(), body.Bytes()...))
		offs[stmNum] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n<</Type/ObjStm/N %d/First %d/Filter/FlateDecode/Length %d>>stream\n",
			stmNum, len(inStm), head.Len(), len(data))
		b.Write(data)
		b.WriteString("\nendstream\nendobj\n")
	}
	xrefNum := n + 2
	var rows bytes.Buffer
	for k := 0; k <= xrefNum; k++ {
		switch {
		case k == 0 || k == stmNum && len(inStm) == 0:
			rows.Write([]byte{0, 0, 0, 0})
		case stm[k]:
			idx := 0
			for i, s := range inStm {
				if s == k {
					idx = i
				}
			}
			rows.Write([]byte{2, byte(stmNum >> 8), byte(stmNum), byte(idx)})
		case k == xrefNum:
			rows.Write([]byte{1, byte(b.Len() >> 8), byte(b.Len()), 0})
		default:
			rows.Write([]byte{1, byte(offs[k] >> 8), byte(offs[k]), 0})
		}
	}
	tr := fmt.Sprintf("/Size %d/Root 1 0 R", xrefNum+1)
	switch kind {
	case "stream":
		start := b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n<</Type/XRef/W[1 2 1]%s/Length %d>>stream\n", xrefNum, tr, rows.Len())
		b.Write(rows.Bytes())
		fmt.Fprintf(&b, "\nendstream\nendobj\nstartxref\n%d\n%%%%EOF\n", start)
	default:
		x := 0
		if kind == "hybrid" {
			x = b.Len()
			fmt.Fprintf(&b, "%d 0 obj\n<</Type/XRef/W[1 2 1]/Size %d/Length %d>>stream\n", xrefNum, xrefNum+1, rows.Len())
			b.Write(rows.Bytes())
			b.WriteString("\nendstream\nendobj\n")
			tr += fmt.Sprintf("/XRefStm %d", x)
		}
		start := b.Len()
		fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", n+1)
		for k := 1; k <= n; k++ {
			if stm[k] {
				// Hybrid: objects in streams are free in the table.
				b.WriteString("0000000000 00000 f \n")
			} else {
				fmt.Fprintf(&b, "%010d 00000 n \n", offs[k])
			}
		}
		fmt.Fprintf(&b, "trailer\n<<%s>>\nstartxref\n%d\n%%%%EOF\n", tr, start)
	}
	return b.Bytes()
}

func zip(b []byte) []byte {
	var z bytes.Buffer
	w := zlib.NewWriter(&z)
	w.Write(b)
	w.Close()
	return z.Bytes()
}

var testObjs = []string{
	"<</Type/Catalog/Pages 2 0 R>>",
	"<</Type/Pages/Kids[3 0 R 4 0 R 5 0 R]/Count 3/MediaBox[0 0 100 200]/Rotate 90/Resources<</X 1>>>>",
	"<</Type/Page/Parent 2 0 R>>",
	"<</Type/Pages/Parent 2 0 R/Kids[6 0 R]/Count 1/Rotate -90/CropBox[10 20 0 0]>>",
	"<</Type/Page/Parent 2 0 R/Contents 7 0 R/Resources<</Y 2>>/MediaBox 8 0 R>>",
	"<</Type/Page/Parent 4 0 R>>",
	"<</Length 9 0 R>>stream\nq Q\nendstream",
	"[0 0 300 400]",
	"3",
}

func checkTestDoc(t *testing.T, name string, data []byte, repaired bool) {
	t.Helper()
	r, err := Open(data)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if r.Repaired != repaired {
		t.Errorf("%s: repaired %v", name, r.Repaired)
	}
	pages, err := r.Pages()
	if err != nil || len(pages) != 3 {
		t.Fatalf("%s: %d pages, %v", name, len(pages), err)
	}
	type summary struct {
		ref       pdf.Ref
		media     [4]float64
		crop      [4]float64
		rotate    int
		resources pdf.Dict
	}
	want := []summary{
		{3, [4]float64{0, 0, 100, 200}, [4]float64{0, 0, 100, 200}, 90, pdf.Dict{"X": 1}},
		{6, [4]float64{0, 0, 100, 200}, [4]float64{0, 0, 10, 20}, 270, pdf.Dict{"X": 1}},
		{5, [4]float64{0, 0, 300, 400}, [4]float64{0, 0, 300, 400}, 90, pdf.Dict{"Y": 2}},
	}
	for i, p := range pages {
		got := summary{p.Ref, p.MediaBox, p.CropBox, p.Rotate, p.Resources}
		if !reflect.DeepEqual(got, want[i]) {
			t.Errorf("%s: page %d: %+v; want %+v", name, i+1, got, want[i])
		}
	}
	s, ok := r.Get(7).(*Stream)
	if !ok || string(s.Raw) != "q Q" {
		t.Errorf("%s: stream %v", name, s)
	}
}

func TestStructure(t *testing.T) {
	stm := map[int]bool{2: true, 4: true, 8: true}
	for _, kind := range []string{"table", "stream", "hybrid"} {
		checkTestDoc(t, kind, build(testObjs, kind, nil), false)
		if kind != "table" {
			checkTestDoc(t, kind+"+objstm", build(testObjs, kind, stm), false)
		}
	}
}

func TestRepair(t *testing.T) {
	good := build(testObjs, "table", nil)
	// Junk before the header: offsets are off by its length.
	checkTestDoc(t, "prefix", append([]byte("junk\n"), good...), false)
	// No startxref.
	i := bytes.LastIndex(good, []byte("startxref"))
	checkTestDoc(t, "no startxref", good[:i], true)
	// startxref points nowhere.
	bad := append(append([]byte{}, good[:i]...), "startxref\n99999\n%%EOF\n"...)
	checkTestDoc(t, "bad startxref", bad, true)
	// Offsets in the table shifted: each object is found by scanning.
	shifted := bytes.Clone(good)
	x := bytes.Index(shifted, []byte("xref\n0 "))
	tbl := shifted[x:]
	for _, k := range []int{3, 5} {
		line := bytes.Index(tbl, []byte(fmt.Sprintf("%010d", bytes.Index(good, []byte(fmt.Sprintf("\n%d 0 obj", k)))+1)))
		if line >= 0 {
			copy(tbl[line:], "0000000020")
		}
	}
	checkTestDoc(t, "bad offsets", shifted, false)
	// Object streams, no xref at all.
	ostm := build(testObjs, "stream", map[int]bool{2: true, 4: true, 8: true})
	i = bytes.LastIndex(ostm, []byte("startxref"))
	checkTestDoc(t, "objstm no xref", ostm[:i], true)
	// No trailer: the catalog is found by type.
	i = bytes.Index(good, []byte("xref\n0"))
	checkTestDoc(t, "no trailer", good[:i], true)
	// Wrong /Length: the data ends at endstream.
	checkTestDoc(t, "bad length", bytes.Replace(good, []byte("\n3\nendobj"), []byte("\n5\nendobj"), 1), false)
	if _, err := Open([]byte("%PDF-1.4\nnothing")); err == nil {
		t.Error("no catalog: no error")
	}
	if _, err := Open([]byte("GIF89a")); err != ErrNotPDF {
		t.Errorf("not PDF: %v", err)
	}
}

func TestLoops(t *testing.T) {
	objs := []string{
		"<</Type/Catalog/Pages 2 0 R>>",
		"<</Type/Pages/Kids[3 0 R 2 0 R 3 0 R 4 0 R]/Count 2>>",
		"<</Type/Page/Parent 2 0 R>>",
		"<</Type/Pages/Kids[2 0 R]/Count 0>>",
		"5 0 R",
	}
	r, err := Open(build(objs, "table", nil))
	if err != nil {
		t.Fatal(err)
	}
	pages, err := r.Pages()
	if err != nil || len(pages) != 1 {
		t.Errorf("%d pages, %v", len(pages), err)
	}
	if v := r.Resolve(pdf.Ref(5)); v != nil {
		t.Errorf("self reference: %v", v)
	}
}
