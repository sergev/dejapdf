// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package shapes

import (
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/sergev/dejapdf/internal/jb2"
	"github.com/sergev/dejapdf/internal/pdf"
	"github.com/sergev/dejapdf/internal/pdfread"
	"github.com/sergev/dejapdf/internal/raster"
	"github.com/sergev/dejapdf/internal/scan"
)

func newBitmap(w, h int) *raster.Bitmap {
	s := (w + 7) / 8
	return &raster.Bitmap{W: w, H: h, Stride: s, Pix: make([]byte, s*h)}
}

func set(b *raster.Bitmap, x, y int) { b.Pix[y*b.Stride+x/8] |= 0x80 >> (x % 8) }

func parse(rows ...string) *raster.Bitmap {
	b := newBitmap(len(rows[0]), len(rows))
	for y, r := range rows {
		for x, c := range r {
			if c == '#' {
				set(b, x, y)
			}
		}
	}
	return b
}

// check splits b and verifies the result: it renders to b, shapes are
// tight, no larger than MaxSide, and distinct.
func check(t *testing.T, b *raster.Bitmap) *jb2.Image {
	t.Helper()
	im := Split(b)
	got := im.Render()
	for y := range b.H {
		for x := range b.W {
			want := b.Pix[y*b.Stride+x/8] >> (7 - x%8) & 1
			if got.Pix[y*b.W+x] != want {
				t.Fatalf("%dx%d: pixel (%d, %d) = %d, want %d", b.W, b.H, x, y, got.Pix[y*b.W+x], want)
			}
		}
	}
	seen := map[string]bool{}
	for i, s := range im.Shapes {
		if s.W > MaxSide || s.H > MaxSide {
			t.Fatalf("shape %d: %dx%d", i, s.W, s.H)
		}
		if t2, _, _ := trim(s); t2 != s {
			t.Fatalf("shape %d: %dx%d has empty edges", i, s.W, s.H)
		}
		k := string(rune(s.W)) + string(rune(s.H)) + string(s.Pix)
		if seen[k] {
			t.Fatalf("shape %d: duplicate", i)
		}
		seen[k] = true
	}
	return im
}

func TestComponents(t *testing.T) {
	for _, c := range []struct {
		rows          []string
		blits, shapes int
	}{
		{[]string{"...", "...", "..."}, 0, 0},
		{[]string{"#.#", ".#.", "#.#"}, 1, 1},    // diagonals connect
		{[]string{"#..#", "#..#", "####"}, 1, 1}, // a U is one
		{[]string{"##.##", ".....", "##.##"}, 4, 1},
		{[]string{"#.#.#.#.#"}, 5, 1},
		{[]string{"#....", "..#..", "....#"}, 3, 1},
		{[]string{"###.#", "#...#", "#.###"}, 2, 2},  // a spiral's arms
		{[]string{"#.#", "###", "..#", "#.#"}, 2, 2}, // merges from below
		{[]string{"..#", ".#.", "#..", ".#.", "..#"}, 1, 1},
	} {
		im := check(t, parse(c.rows...))
		if len(im.Blits) != c.blits || len(im.Shapes) != c.shapes {
			t.Errorf("%q: %d blits, %d shapes; want %d, %d", c.rows, len(im.Blits), len(im.Shapes), c.blits, c.shapes)
		}
	}
}

func TestRandom(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for range 300 {
		w, h := 1+r.Intn(70), 1+r.Intn(40)
		b := newBitmap(w, h)
		p := r.Float64()
		for y := range h {
			for x := range w {
				if r.Float64() < p {
					set(b, x, y)
				}
			}
			// Ink in the padding must be ignored.
			if w%8 != 0 {
				b.Pix[y*b.Stride+b.Stride-1] |= 0xff >> (w % 8)
			}
		}
		check(t, b)
	}
}

func TestTiles(t *testing.T) {
	defer func(n int) { MaxSide = n }(MaxSide)
	MaxSide = 16
	b := newBitmap(100, 60)
	// A frame, a diagonal and a solid block, all larger than MaxSide.
	for x := range 100 {
		set(b, x, 0)
		set(b, x, 59)
	}
	for y := range 60 {
		set(b, 0, y)
		set(b, 99, y)
		set(b, 10+y, y)
	}
	for y := 20; y < 50; y++ {
		for x := 70; x < 95; x++ {
			set(b, x, y)
		}
	}
	im := check(t, b)
	t.Logf("%d blits, %d shapes", len(im.Blits), len(im.Shapes))
}

// TestScans splits the bilevel scan images of the test files, and of
// $PDF_BOOKS, and checks that they render back exactly.
func TestScans(t *testing.T) {
	files, _ := filepath.Glob("../../testdata/pdf/*.pdf")
	books, _ := filepath.Glob(os.Getenv("PDF_BOOKS"))
	for _, path := range append(files, books...) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()
			data, err := os.ReadFile(path)
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
			seen := map[pdf.Ref]bool{}
			n, blits, shapes := 0, 0, 0
			for i := range doc.Pages {
				p := &doc.Pages[i]
				for _, u := range p.Uses {
					if a, _ := doc.Plan(r, p, u); a != scan.JBIG2 || seen[u.Image.Ref] {
						continue
					}
					seen[u.Image.Ref] = true
					got, err := raster.Decode(r, u.Image)
					if err != nil {
						t.Errorf("object %d: %v", u.Image.Ref, err)
						continue
					}
					im := check(t, got.(*raster.Bitmap))
					n++
					blits += len(im.Blits)
					shapes += len(im.Shapes)
				}
			}
			if n == 0 {
				t.Skip("no bilevel scans")
			}
			t.Logf("%d images, %d blits, %d shapes", n, blits, shapes)
		})
	}
}

func FuzzSplit(f *testing.F) {
	f.Add(uint8(9), []byte{0xa5, 0x5a, 0xff, 0x01, 0x80})
	f.Fuzz(func(t *testing.T, w uint8, pix []byte) {
		if w == 0 || len(pix) > 4096 {
			return
		}
		defer func(n int) { MaxSide = n }(MaxSide)
		MaxSide = 1 + int(w)%13
		s := (int(w) + 7) / 8
		h := len(pix) / s
		check(t, &raster.Bitmap{W: int(w), H: h, Stride: s, Pix: pix[:s*h]})
	})
}
