// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

// Package shapes splits a bilevel scan into shapes and their positions,
// as a jb2.Image for internal/stencil.
package shapes

import (
	"bytes"
	"hash/maphash"
	"math/bits"
	"slices"

	"github.com/sergev/dejapdf/internal/jb2"
	"github.com/sergev/dejapdf/internal/raster"
)

// MaxSide caps a shape's width and height: larger components (rules,
// figures) are cut into tiles of at most MaxSide×MaxSide.
var MaxSide = 1024

// run is a horizontal run of ink [x0, x1) in row y.
type run struct {
	x0, x1, y int32
}

// Split returns b's 8-connected components as shapes, each blitted
// once; equal shapes are merged. Rendering the image gives b.
func Split(b *raster.Bitmap) *jb2.Image {
	rs, rows := findRuns(b)
	comp, n := label(rs, rows)

	// Group the runs by component, in order of their first run.
	start := make([]int32, n+1)
	for _, c := range comp {
		start[c+1]++
	}
	for i := range n {
		start[i+1] += start[i]
	}
	byComp := make([]int32, len(rs))
	next := slices.Clone(start[:n])
	for i, c := range comp {
		byComp[next[c]] = int32(i)
		next[c]++
	}

	im := &jb2.Image{W: b.W, H: b.H}
	seen := map[uint64][]int{}
	seed := maphash.MakeSeed()
	add := func(s *jb2.Bitmap, x, top int) {
		h := maphash.Bytes(seed, s.Pix) ^ uint64(s.W)<<32 ^ uint64(s.H)
		k := -1
		for _, i := range seen[h] {
			if t := im.Shapes[i]; t.W == s.W && t.H == s.H && bytes.Equal(t.Pix, s.Pix) {
				k = i
				break
			}
		}
		if k < 0 {
			k = len(im.Shapes)
			im.Shapes = append(im.Shapes, s)
			im.Refs = append(im.Refs, jb2.Ref{Shape: -1})
			seen[h] = append(seen[h], k)
		}
		im.Blits = append(im.Blits, jb2.Blit{Shape: k, X: x, Y: b.H - top - s.H})
	}
	for c := range n {
		cr := byComp[start[c]:start[c+1]]
		x0, x1 := rs[cr[0]].x0, rs[cr[0]].x1
		y0, y1 := rs[cr[0]].y, rs[cr[len(cr)-1]].y+1
		for _, i := range cr {
			x0, x1 = min(x0, rs[i].x0), max(x1, rs[i].x1)
		}
		ms := int32(MaxSide)
		for ty := y0; ty < y1; ty += ms {
			for tx := x0; tx < x1; tx += ms {
				bx0, by0 := tx, ty
				bx1, by1 := min(tx+ms, x1), min(ty+ms, y1)
				s := paint(rs, cr, bx0, by0, bx1, by1)
				if s, dx, dy := trim(s); s != nil {
					add(s, int(bx0)+dx, int(by0)+dy)
				}
			}
		}
	}
	return im
}

// findRuns returns the runs of ink in raster order, and where each
// row's runs start (rows[y]..rows[y+1]).
func findRuns(b *raster.Bitmap) ([]run, []int32) {
	var rs []run
	rows := make([]int32, b.H+1)
	w := b.W
	for y := range b.H {
		row := b.Pix[y*b.Stride : y*b.Stride+(w+7)/8]
		for x := 0; ; {
			x = scanTo(row, x, w, 0)
			if x >= w {
				break
			}
			e := scanTo(row, x, w, 0xff)
			rs = append(rs, run{int32(x), int32(e), int32(y)})
			x = e
		}
		rows[y+1] = int32(len(rs))
	}
	return rs, rows
}

// scanTo returns the first column from x whose bit differs from inv's
// (0: the next 1, 0xff: the next 0), or w.
func scanTo(row []byte, x, w int, inv byte) int {
	for x < w {
		v := (row[x>>3] ^ inv) & (0xff >> (x & 7))
		if v != 0 {
			return min(x&^7+bits.LeadingZeros8(v), w)
		}
		x = x&^7 + 8
	}
	return w
}

// label gives each run its component, numbered in order of the
// component's first run, and the number of components.
func label(rs []run, rows []int32) ([]int32, int) {
	parent := make([]int32, len(rs))
	for i := range parent {
		parent[i] = int32(i)
	}
	find := func(i int32) int32 {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	for y := 1; y+1 < len(rows); y++ {
		j, pe := rows[y-1], rows[y]
		for i := rows[y]; i < rows[y+1]; i++ {
			a := rs[i]
			for j < pe && rs[j].x1 < a.x0 {
				j++
			}
			// Diagonal neighbours touch: [x0-1, x1] overlaps.
			for k := j; k < pe && rs[k].x0 <= a.x1; k++ {
				ra, rb := find(i), find(k)
				if ra != rb {
					// The earlier run stays the root.
					if ra < rb {
						parent[rb] = ra
					} else {
						parent[ra] = rb
					}
				}
			}
		}
	}
	comp := make([]int32, len(rs))
	n := int32(0)
	for i := range rs {
		r := find(int32(i))
		if r == int32(i) {
			comp[i] = n
			n++
		} else {
			comp[i] = comp[r]
		}
	}
	return comp, int(n)
}

// paint draws the runs cr clipped to [x0, x1)×[y0, y1).
func paint(rs []run, cr []int32, x0, y0, x1, y1 int32) *jb2.Bitmap {
	w, h := int(x1-x0), int(y1-y0)
	s := &jb2.Bitmap{W: w, H: h, Pix: make([]byte, w*h)}
	for _, i := range cr {
		r := rs[i]
		if r.y < y0 || r.y >= y1 {
			continue
		}
		a, e := max(r.x0, x0), min(r.x1, x1)
		row := s.Pix[int(r.y-y0)*w:]
		for x := a; x < e; x++ {
			row[x-x0] = 1
		}
	}
	return s
}

// trim cuts s's empty edges, returning the offset of what is left, or
// nil if s is empty.
func trim(s *jb2.Bitmap) (*jb2.Bitmap, int, int) {
	x0, y0, x1, y1 := s.W, s.H, 0, 0
	for y := range s.H {
		row := s.Pix[y*s.W : (y+1)*s.W]
		a := bytes.IndexByte(row, 1)
		if a < 0 {
			continue
		}
		e := bytes.LastIndexByte(row, 1) + 1
		x0, x1 = min(x0, a), max(x1, e)
		y0, y1 = min(y0, y), y+1
	}
	if x1 == 0 {
		return nil, 0, 0
	}
	if x0 == 0 && y0 == 0 && x1 == s.W && y1 == s.H {
		return s, 0, 0
	}
	w, h := x1-x0, y1-y0
	t := &jb2.Bitmap{W: w, H: h, Pix: make([]byte, w*h)}
	for y := range h {
		copy(t.Pix[y*w:(y+1)*w], s.Pix[(y+y0)*s.W+x0:])
	}
	return t, x0, y0
}
