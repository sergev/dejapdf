// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package shapes

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/sergev/dejapdf/internal/jb2"
	"github.com/sergev/dejapdf/internal/pdfread"
	"github.com/sergev/dejapdf/internal/raster"
	"github.com/sergev/dejapdf/internal/scan"
)

// The rows of testdata/pdf/digits.pdf (testdata/mkpdf.sh): one
// character each, baselines 13 pt apart from 36 pt below the top.
var lookalikes = strings.Fields("0 O o D Q C G 1 l I i j ! | 7 T 3 8 6 9 5 S s 2 Z z B E F P R c e a n h u b d p q g , . : ; ' - r f t")

var digitsPages = []string{"Times 7", "Times 10", "Helvetica 9", "Courier 9"}

// TestLookalikes substitutes alike shapes across all pages of
// digits.pdf, as lossy re-encoding of one group does, and reads each
// glyph before and after: as the character of the nearest clean glyph
// of its font. No glyph read right before may read otherwise after.
func TestLookalikes(t *testing.T) {
	data, err := os.ReadFile("../../testdata/pdf/digits.pdf")
	if err != nil {
		t.Fatal(err)
	}
	r, err := pdfread.Open(data)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := scan.Find(r)
	if err != nil {
		t.Fatal(err)
	}
	var pages []*jb2.Bitmap // byte per pixel
	var ims []*jb2.Image
	for i := range doc.Pages {
		v, err := raster.Decode(r, doc.Pages[i].Uses[0].Image)
		if err != nil {
			t.Fatal(err)
		}
		im := Split(v.(*raster.Bitmap))
		ims = append(ims, im)
		pages = append(pages, im.Render())
	}
	after, classes, nshapes := substitute(ims)

	// Glyph cells, from the clean pages: components overlapping in x
	// are one glyph; each row's first glyph is its template.
	type cell struct{ x0, y0, x1, y1 int }
	cells := make([][][]cell, 4) // font, row
	for f := range 4 {
		cells[f] = make([][]cell, len(lookalikes))
		im := ims[f]
		for _, bl := range im.Blits {
			s := im.Shapes[bl.Shape]
			top := im.H - bl.Y - s.H
			cy := float64(top) + float64(s.H)/2
			row := int((cy*72/300 - 36 + 9.5) / 13)
			if row < 0 || row >= len(lookalikes) {
				continue
			}
			c := cell{bl.X, top, bl.X + s.W, top + s.H}
			cs := cells[f][row]
			merged := false
			for k := range cs {
				if c.x0 <= cs[k].x1 && cs[k].x0 <= c.x1 {
					cs[k] = cell{min(cs[k].x0, c.x0), min(cs[k].y0, c.y0), max(cs[k].x1, c.x1), max(cs[k].y1, c.y1)}
					merged = true
				}
			}
			if !merged {
				cells[f][row] = append(cs, c)
			}
		}
	}
	const pad = 3
	crop := func(b *jb2.Bitmap, c cell) *jb2.Bitmap {
		x0, y0, x1, y1 := c.x0-pad, c.y0-pad, c.x1+pad, c.y1+pad
		out := &jb2.Bitmap{W: x1 - x0, H: y1 - y0, Pix: make([]byte, (x1-x0)*(y1-y0))}
		for y := y0; y < y1; y++ {
			copy(out.Pix[(y-y0)*out.W:(y-y0+1)*out.W], b.Pix[y*b.W+x0:y*b.W+x1])
		}
		return out
	}
	// read gives the row of the nearest template, by XOR with shifts.
	templates := make([][]*jb2.Bitmap, 4)
	for f := range 4 {
		templates[f] = make([]*jb2.Bitmap, len(lookalikes))
		for k, cs := range cells[f] {
			if len(cs) > 0 {
				templates[f][k] = crop(pages[f], cs[0])
			}
		}
	}
	// read gives the row of the nearest template, and how much nearer
	// it is than the template of row want (or than the next nearest,
	// if it is want).
	read := func(f int, g *jb2.Bitmap, want int) (int, int) {
		best, next, row := 1<<30, 1<<30, -1
		dwant := 1 << 30
		for k, tp := range templates[f] {
			if tp == nil || abs(tp.W-g.W) > 6 || abs(tp.H-g.H) > 6 {
				continue
			}
			d := distance(g, tp)
			if k == want {
				dwant = d
			}
			if d < best {
				best, next, row = d, best, k
			} else if d < next {
				next = d
			}
		}
		if row == want {
			return row, next - best
		}
		return row, dwant - best
	}
	type result struct {
		n, misread int
		bad        map[string]int
	}
	res := make([]result, len(pages))
	parallelFor(len(pages), func(p int) {
		f := p % 4
		res[p].bad = map[string]int{}
		for row, cs := range cells[f] {
			for _, c := range cs {
				res[p].n++
				// Count only glyphs read right by a clear margin before, and
				// clearly as another character after.
				if was, m := read(f, crop(pages[p], c), row); was != row || m < readMargin {
					res[p].misread++
				} else if now, m := read(f, crop(after[p], c), row); now != row && m >= readMargin &&
					!(quote[lookalikes[row]] && quote[lookalikes[now]]) {
					k := fmt.Sprintf("%q reads %q", lookalikes[row], lookalikes[now])
					if res[p].bad[k]++; res[p].bad[k] == 1 && os.Getenv("DUMP") != "" {
						dump(fmt.Sprintf("%s/p%d-%d-%d-x%d-y%d", os.Getenv("DUMP"), p+1, row, now, c.x0, c.y0), crop(pages[p], c), crop(after[p], c), templates[f][now])
					}
				}
			}
		}
	})
	n, misread := 0, 0
	for p, x := range res {
		n += x.n
		misread += x.misread
		for k, v := range x.bad {
			t.Errorf("page %d (%s): %s (%d)", p+1, digitsPages[p%4], k, v)
		}
	}
	t.Logf("%d shapes in %d classes; %d glyphs, %d not read clearly before substitution", nshapes, classes, n, misread)
}

// readMargin is how many pixels nearer one character's template must be
// than another's for a glyph to read clearly as it.
const readMargin = 3

// Apostrophe and comma are one glyph in these fonts, told apart by
// where they stand, which substitution keeps.
var quote = map[string]bool{"'": true, ",": true}

// substitute classifies the shapes of the images together and renders
// each with members replaced by their representatives.
func substitute(ims []*jb2.Image) ([]*jb2.Bitmap, int, int) {
	var bms []*jb2.Bitmap
	byKey := map[string]int{}
	ids := make([][]int, len(ims))
	for k, im := range ims {
		ids[k] = make([]int, len(im.Shapes))
		for i, s := range im.Shapes {
			key := fmt.Sprintf("%d %d %s", s.W, s.H, s.Pix)
			id, ok := byKey[key]
			if !ok {
				id = len(bms)
				byKey[key] = id
				bms = append(bms, s)
			}
			ids[k][i] = id
		}
	}
	cl := Classify(bms)
	classes := 0
	for i, c := range cl {
		if c.Rep == i {
			classes++
		}
	}
	var out []*jb2.Bitmap
	for k, im := range ims {
		j := &jb2.Image{W: im.W, H: im.H, Shapes: bms, Refs: make([]jb2.Ref, len(bms))}
		for _, bl := range im.Blits {
			id := ids[k][bl.Shape]
			c := cl[id]
			s, rep := bms[id], bms[c.Rep]
			j.Blits = append(j.Blits, jb2.Blit{Shape: c.Rep, X: bl.X + c.DX, Y: bl.Y + s.H - c.DY - rep.H})
		}
		out = append(out, j.Render())
	}
	return out, classes, len(bms)
}

// distance is the least XOR count of a and b, b shifted a pixel.
func distance(a, b *jb2.Bitmap) int {
	best := 1 << 30
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			d := 0
			for y := range max(a.H, b.H) {
				for x := range max(a.W, b.W) {
					if at(a, x, y) != at(b, x-dx, y-dy) {
						d++
					}
				}
			}
			best = min(best, d)
		}
	}
	return best
}

func at(b *jb2.Bitmap, x, y int) byte {
	if x < 0 || y < 0 || x >= b.W || y >= b.H {
		return 0
	}
	return b.Pix[y*b.W+x]
}

// dump writes glyphs side by side as a PBM, for looking at failures.
func dump(name string, gs ...*jb2.Bitmap) {
	w, h := 0, 0
	for _, g := range gs {
		w += g.W + 2
		h = max(h, g.H)
	}
	out := fmt.Appendf(nil, "P1\n%d %d\n", w, h)
	for y := range h {
		x0 := 0
		for _, g := range gs {
			for x := range g.W + 2 {
				out = append(out, '0'+at(g, x, y), ' ')
			}
			x0 += g.W + 2
		}
		out = append(out, '\n')
	}
	os.WriteFile(name+".pbm", out, 0o644)
}
