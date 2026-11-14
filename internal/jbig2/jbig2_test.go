// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package jbig2

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// testBitmap draws random boxes, lines and noise.
func testBitmap(w, h int, seed int64) *Bitmap {
	r := rand.New(rand.NewSource(seed))
	b := NewBitmap(w, h)
	for range 20 {
		x0, y0 := r.Intn(w), r.Intn(h)
		x1, y1 := min(w, x0+r.Intn(w/3+1)+1), min(h, y0+r.Intn(h/3+1)+1)
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				if (x+y)%7 != 0 {
					b.Set(x, y)
				}
			}
		}
	}
	for range w * h / 50 {
		b.Set(r.Intn(w), r.Intn(h))
	}
	return b
}

// pbm returns b as a P4 file.
func pbm(b *Bitmap) []byte {
	out := fmt.Appendf(nil, "P4\n%d %d\n", b.W, b.H)
	return append(out, b.Pix...)
}

// jbig2dec decodes a standalone file to PBM pixels.
func jbig2dec(t *testing.T, file []byte) []byte {
	t.Helper()
	dir := t.TempDir()
	in, out := filepath.Join(dir, "a.jb2"), filepath.Join(dir, "a.pbm")
	if err := os.WriteFile(in, file, 0o644); err != nil {
		t.Fatal(err)
	}
	if b, err := exec.Command("jbig2dec", "-q", "-t", "pbm", "-o", out, in).CombinedOutput(); err != nil {
		t.Fatalf("jbig2dec: %v\n%s", err, b)
	}
	d, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestGenericJbig2dec(t *testing.T) {
	if _, err := exec.LookPath("jbig2dec"); err != nil {
		t.Skip("no jbig2dec")
	}
	for _, sz := range [][2]int{{1, 1}, {7, 3}, {8, 8}, {9, 17}, {100, 1}, {333, 211}, {2550, 300}} {
		b := testBitmap(sz[0], sz[1], int64(sz[0]))
		segs := []Segment{
			{Number: 0, Type: TypePageInfo, Page: 1, Data: PageInfo(b.W, b.H, 300)},
			{Number: 1, Type: TypeGenericRegion, Page: 1, Data: GenericRegion(b, 0, 0)},
		}
		if got, want := jbig2dec(t, File(segs)), pbm(b); !bytes.Equal(got, want) {
			t.Errorf("%dx%d: jbig2dec output differs", b.W, b.H)
		}
	}
	// A blank bitmap is all typical rows.
	b := NewBitmap(64, 64)
	segs := []Segment{
		{Number: 0, Type: TypePageInfo, Page: 1, Data: PageInfo(64, 64, 0)},
		{Number: 1, Type: TypeGenericRegion, Page: 1, Data: GenericRegion(b, 0, 0)},
	}
	if got := jbig2dec(t, File(segs)); !bytes.Equal(got, pbm(b)) {
		t.Error("blank: jbig2dec output differs")
	}
}

func TestSegmentHeader(t *testing.T) {
	for _, c := range []struct {
		s    Segment
		want string
	}{
		{Segment{Number: 1, Type: 38, Page: 1, Data: []byte{9}}, "00000001 26 00 01 00000001 09"},
		{Segment{Number: 5, Type: 6, Refs: []uint32{1, 3}, Page: 1}, "00000005 06 40 0103 01 00000000"},
		{Segment{Number: 300, Type: 6, Refs: []uint32{2}, Page: 0}, "0000012c 06 20 0002 00 00000000"},
		{Segment{Number: 9, Type: 6, Refs: []uint32{1, 2, 3, 4, 5}, Page: 300}, "00000009 46 e0000005 00 0102030405 0000012c 00000000"},
	} {
		got := fmt.Sprintf("% x", c.s.Append(nil))
		want := ""
		for _, f := range bytes.Fields([]byte(c.want)) {
			for i := 0; i < len(f); i += 2 {
				if want != "" {
					want += " "
				}
				want += string(f[i : i+2])
			}
		}
		if got != want {
			t.Errorf("segment %d: %s, want %s", c.s.Number, got, want)
		}
	}
}

// TestTextJbig2dec builds pages from random symbols with a global and a
// page dictionary and checks jbig2dec's decode against direct drawing.
func TestTextJbig2dec(t *testing.T) {
	if _, err := exec.LookPath("jbig2dec"); err != nil {
		t.Skip("no jbig2dec")
	}
	r := rand.New(rand.NewSource(3))
	sym := func() *Bitmap {
		b := testBitmap(1+r.Intn(30), 1+r.Intn(40), r.Int63())
		b.Set(0, 0) // non-empty
		return b
	}
	for _, c := range []struct{ nglob, nloc, n, strips, dso int }{
		{0, 1, 1, 1, 0}, {1, 0, 5, 1, 0}, {3, 4, 60, 1, 0}, {50, 20, 400, 2, 0},
		{50, 20, 400, 4, 3}, {200, 70, 900, 8, -2}, {0, 300, 300, 1, 1},
	} {
		glob, loc := make([]*Bitmap, c.nglob), make([]*Bitmap, c.nloc)
		for i := range glob {
			glob[i] = sym()
		}
		for i := range loc {
			loc[i] = sym()
		}
		const w, h = 700, 500
		want := NewBitmap(w, h)
		var gd, ld []byte
		var gids, lids []int
		if c.nglob > 0 {
			gd, gids = SymbolDict(glob)
		}
		ld, lids = SymbolDict(loc)
		var ins []Instance
		for range c.n {
			var b *Bitmap
			var id int
			if k := r.Intn(c.nglob + c.nloc); k < c.nglob {
				b, id = glob[k], gids[k]
			} else {
				b, id = loc[k-c.nglob], c.nglob+lids[k-c.nglob]
			}
			x, y := r.Intn(w+20)-10, r.Intn(h+20)-10 // top-left, may stick out
			ins = append(ins, Instance{ID: id, S: x, T: y + b.H - 1, W: b.W})
			for yy := range b.H {
				for xx := range b.W {
					px, py := x+xx, y+yy
					if pixel(b.Row(yy), xx, b.W) == 1 && px >= 0 && py >= 0 && px < w && py < h {
						want.Set(px, py)
					}
				}
			}
		}
		var segs []Segment
		var refs []uint32
		n := uint32(0)
		if c.nglob > 0 {
			segs = append(segs, Segment{Number: 0, Type: TypeSymbolDict, Page: 0, Data: gd})
			refs = append(refs, 0)
			n = 1
		}
		segs = append(segs,
			Segment{Number: n, Type: TypePageInfo, Page: 1, Data: PageInfo(w, h, 300)},
			Segment{Number: n + 1, Type: TypeSymbolDict, Page: 1, Data: ld},
			Segment{Number: n + 2, Type: TypeTextRegion, Refs: append(refs, n+1), Page: 1,
				Data: TextRegion(w, h, ins, c.nglob+c.nloc, c.strips, c.dso)})
		if got := jbig2dec(t, File(segs)); !bytes.Equal(got, pbm(want)) {
			t.Errorf("%+v: jbig2dec output differs", c)
		}
	}
}

// TestRefineJbig2dec codes symbols as refinements of noisy copies, in a
// dictionary importing a direct one, and draws them with a text region.
func TestRefineJbig2dec(t *testing.T) {
	if _, err := exec.LookPath("jbig2dec"); err != nil {
		t.Skip("no jbig2dec")
	}
	r := rand.New(rand.NewSource(5))
	var base []*Bitmap
	for range 6 {
		b := testBitmap(3+r.Intn(30), 3+r.Intn(30), r.Int63())
		b.Set(0, 0)
		base = append(base, b)
	}
	baseData, baseIDs := SymbolDict(base)
	inputs := make([]*Bitmap, len(base))
	for i, b := range base {
		inputs[baseIDs[i]] = b
	}
	// Refined symbols: a reference's pixels moved and flipped; some
	// refine earlier refined ones.
	var syms []*Bitmap
	var refs []Ref
	for k := range 40 {
		id := r.Intn(len(inputs) + k)
		var src *Bitmap
		if id < len(inputs) {
			src = inputs[id]
		} else {
			src = syms[id-len(inputs)]
		}
		w, h := max(1, src.W+r.Intn(5)-2), max(1, src.H+r.Intn(5)-2)
		dx, dy := r.Intn(5)-2, r.Intn(5)-2
		s := NewBitmap(w, h)
		for y := range h {
			for x := range w {
				v := pixel(src.Row(y-dy), x-dx, src.W)
				if r.Intn(20) == 0 {
					v ^= 1
				}
				if v == 1 {
					s.Set(x, y)
				}
			}
		}
		syms = append(syms, s)
		refs = append(refs, Ref{id, dx, dy})
	}
	refData := RefineDict(syms, refs, inputs)
	const w, h = 400, 300
	want := NewBitmap(w, h)
	var ins []Instance
	all := append(append([]*Bitmap(nil), inputs...), syms...)
	for range 200 {
		id := r.Intn(len(all))
		b := all[id]
		x, y := r.Intn(w), r.Intn(h)
		ins = append(ins, Instance{ID: id, S: x, T: y + b.H - 1, W: b.W})
		for yy := range b.H {
			for xx := range b.W {
				if pixel(b.Row(yy), xx, b.W) == 1 && x+xx < w && y+yy < h {
					want.Set(x+xx, y+yy)
				}
			}
		}
	}
	segs := []Segment{
		{Number: 0, Type: TypeSymbolDict, Page: 0, Data: baseData},
		{Number: 1, Type: TypeSymbolDict, Refs: []uint32{0}, Page: 0, Data: refData},
		{Number: 2, Type: TypePageInfo, Page: 1, Data: PageInfo(w, h, 0)},
		{Number: 3, Type: TypeTextRegion, Refs: []uint32{0, 1}, Page: 1,
			Data: TextRegion(w, h, ins, len(all), 2, 0)},
	}
	if got := jbig2dec(t, File(segs)); !bytes.Equal(got, pbm(want)) {
		t.Error("jbig2dec output differs")
	}
}
