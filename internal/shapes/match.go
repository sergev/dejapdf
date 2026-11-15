// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package shapes

import (
	"math"
	"math/bits"
	"runtime"
	"sync"

	"github.com/sergev/dejapdf/internal/jb2"
)

// Shapes up to this size are matched; larger ones (rules, figures)
// stand for themselves.
const (
	maxMatchW = 128 - 2*margin
	maxMatchH = 256
	margin    = 4 // free bits each side of a row, for shifts
)

// Thresholds: sizes may differ by tolerance pixels; the weighted XOR
// (each differing pixel counted once per differing pixel around it)
// may reach xorTol times the perimeter of the bounding box.
var (
	sizeTol = 2
	xorTol  = 1.0
)

// tolerance is how much the size of a shape w×h may differ: none for
// marks (dots, commas), where a pixel is a different character.
func tolerance(w, h int) int {
	switch s := max(w, h); {
	case s <= tinySize:
		return 0
	case s <= smallSize:
		return min(1, sizeTol)
	}
	return sizeTol
}

var tinySize, smallSize = 10, 24

// narrow shapes (l, I, 1, |, !) differ in a serif or a flag a pixel
// wide: their sizes must agree.
func narrow(f *feature) bool { return 4*f.w <= f.h }

// tinyErr is the weighted XOR allowed for marks: a stray pixel or two.
var tinyErr = 2

// row is a row of a shape, x at bit x+margin of (hi, lo).
type row struct{ hi, lo uint64 }

func (r row) or(s row) row     { return row{r.hi | s.hi, r.lo | s.lo} }
func (r row) and(s row) row    { return row{r.hi & s.hi, r.lo & s.lo} }
func (r row) andNot(s row) row { return row{r.hi &^ s.hi, r.lo &^ s.lo} }
func (r row) xor(s row) row    { return row{r.hi ^ s.hi, r.lo ^ s.lo} }
func (r row) zero() bool       { return r.hi|r.lo == 0 }
func (r row) bit(p int) int {
	if p < 64 {
		return int(r.lo >> p & 1)
	}
	return int(r.hi >> (p - 64) & 1)
}
func (r row) count() int { return bits.OnesCount64(r.hi) + bits.OnesCount64(r.lo) }

// shift moves the bits n places up (x+n), or down if n < 0.
func (r row) shift(n int) row {
	switch {
	case n > 0:
		return row{r.hi<<n | r.lo>>(64-n), r.lo << n}
	case n < 0:
		n = -n
		return row{r.hi >> n, r.lo>>n | r.hi<<(64-n)}
	}
	return r
}

// spread is the row dilated by one pixel left and right.
func (r row) spread() row { return r.or(r.shift(1)).or(r.shift(-1)) }

// feature is what matching needs of a shape.
type feature struct {
	w, h   int
	area   int
	holes  int
	cx, cy float64 // centroid
	rows   []row   // nil if too large to match
	dil    []row   // rows dilated by one pixel, with a row of margin above and below
}

func features(b *jb2.Bitmap) *feature {
	f := &feature{w: b.W, h: b.H}
	var sx, sy int
	for y := range b.H {
		for x, v := range b.Pix[y*b.W : (y+1)*b.W] {
			if v != 0 {
				f.area++
				sx += x
				sy += y
			}
		}
	}
	if f.area > 0 {
		f.cx, f.cy = float64(sx)/float64(f.area), float64(sy)/float64(f.area)
	}
	if b.W > maxMatchW || b.H > maxMatchH {
		return f
	}
	f.holes = holes(b)
	f.rows = make([]row, b.H)
	for y := range b.H {
		var r row
		for x, v := range b.Pix[y*b.W : (y+1)*b.W] {
			if v != 0 {
				if p := x + margin; p < 64 {
					r.lo |= 1 << p
				} else {
					r.hi |= 1 << (p - 64)
				}
			}
		}
		f.rows[y] = r
	}
	f.dil = make([]row, b.H+2)
	for y, r := range f.rows {
		s := r.spread()
		for k := y; k < y+3; k++ {
			f.dil[k] = f.dil[k].or(s)
		}
	}
	return f
}

// holes counts the 4-connected white areas a shape encloses.
func holes(b *jb2.Bitmap) int {
	w, h := b.W+2, b.H+2
	seen := make([]bool, w*h)
	black := func(x, y int) bool {
		return x > 0 && y > 0 && x <= b.W && y <= b.H && b.Pix[(y-1)*b.W+x-1] != 0
	}
	var stack []int
	fill := func(p int) {
		stack = append(stack[:0], p)
		seen[p] = true
		for len(stack) > 0 {
			p := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			x, y := p%w, p/w
			for _, q := range [4][2]int{{x - 1, y}, {x + 1, y}, {x, y - 1}, {x, y + 1}} {
				if q[0] >= 0 && q[1] >= 0 && q[0] < w && q[1] < h {
					if i := q[1]*w + q[0]; !seen[i] && !black(q[0], q[1]) {
						seen[i] = true
						stack = append(stack, i)
					}
				}
			}
		}
	}
	fill(0)
	n := 0
	for y := range h {
		for x := range w {
			if p := y*w + x; !seen[p] && !black(x, y) {
				n++
				fill(p)
			}
		}
	}
	return n
}

// offset is where b's top left goes in a's frame to align centroids.
func offset(a, b *feature) (int, int) {
	return int(math.Round(a.cx - b.cx)), int(math.Round(a.cy - b.cy))
}

// match reports whether b may stand for a: sizes close, the same holes,
// each within a pixel of the other with centroids aligned, and a small
// weighted XOR. It returns the weighted XOR.
func match(a, b *feature) (int, bool) {
	err, ok := matchRaw(a, b)
	if max(a.w, a.h) <= tinySize {
		return err, ok && err <= tinyErr
	}
	return err, ok && float64(err) <= xorTol*float64(2*(a.w+a.h))
}

// matchRaw is match without the XOR limit.
func matchRaw(a, b *feature) (int, bool) {
	tol := tolerance(a.w, a.h)
	if narrow(a) || narrow(b) {
		tol = 0
	}
	if a.rows == nil || b.rows == nil || a.holes != b.holes ||
		abs(a.w-b.w) > tol || abs(a.h-b.h) > tol {
		return 0, false
	}
	// Each lies within the other dilated: the areas differ by less than
	// a perimeter.
	if abs(a.area-b.area) > a.w+a.h+b.w+b.h {
		return 0, false
	}
	dx, dy := offset(a, b)
	if abs(dx) > margin-1 || abs(dy) > tol+1 {
		return 0, false
	}
	// Rows of the union, in a's frame from y0 to y1.
	y0, y1 := min(0, dy), max(a.h, b.h+dy)
	at := func(rows []row, y int) row {
		if y < 0 || y >= len(rows) {
			return row{}
		}
		return rows[y]
	}
	// b's rows, shifted into a's frame.
	bRow := func(y int) row { return at(b.rows, y-dy).shift(dx) }
	bDil := func(y int) row { return at(b.dil, y-dy+1).shift(dx) }
	aDil := func(y int) row { return at(a.dil, y+1) }
	var buf [maxMatchH + 2*margin + 2]row
	xs := buf[:y1-y0+2] // XOR rows, one of margin above and below
	for y := y0; y < y1; y++ {
		ar, br := at(a.rows, y), bRow(y)
		if !ar.andNot(bDil(y)).zero() || !br.andNot(aDil(y)).zero() {
			return 0, false
		}
		xs[y-y0+1] = ar.xor(br)
	}
	patch := maxPatch
	if max(a.w, a.h) <= smallSize || narrow(a) || narrow(b) {
		patch = smallPatch
	}
	// Each XOR pixel counts the XOR pixels in its 3×3 neighbourhood.
	// Edge noise makes thin lines of them; a thick patch is a feature
	// one shape has and the other not (a serif, a flag, a tail).
	err := 0
	for i := 1; i < len(xs)-1; i++ {
		x := xs[i]
		if x.zero() {
			continue
		}
		err += x.and(xs[i-1].spread()).count() + x.and(xs[i].spread()).count() + x.and(xs[i+1].spread()).count()
		for h, w := range [2]uint64{x.lo, x.hi} {
			for ; w != 0; w &= w - 1 {
				p := 64*h + bits.TrailingZeros64(w)
				n := 0
				for _, r := range [3]row{xs[i-1], xs[i], xs[i+1]} {
					n += r.bit(p-1) + r.bit(p) + r.bit(p+1)
				}
				if n >= patch {
					return err, false
				}
			}
		}
	}
	return err, true
}

// maxPatch is the most XOR pixels a 3×3 neighbourhood of an XOR pixel
// may have.
var maxPatch, smallPatch = 5, 3

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// Class tells which shape stands for a shape, and where it goes: its
// top left at (DX, DY) from the shape's.
type Class struct {
	Rep    int
	DX, DY int
}

// Classify groups similar shapes. Each shape joins the earlier
// representative it matches best, or represents itself. Shapes are
// compared in parallel with the representatives before their chunk,
// then in order with those of the chunk: the result does not depend on
// the number of CPUs.
func Classify(bms []*jb2.Bitmap) []Class {
	fs := make([]*feature, len(bms))
	parallelFor(len(bms), func(i int) { fs[i] = features(bms[i]) })
	out := make([]Class, len(bms))
	type key struct{ w, h, holes int }
	reps := map[key][]int{}
	// best finds the best match of shape i among the representatives
	// with index below end (or all, if end < 0) from start.
	best := func(i, start, end int) (int, int) {
		f := fs[i]
		b, be := -1, 1<<30
		for w := f.w - sizeTol; w <= f.w+sizeTol; w++ {
			for h := f.h - sizeTol; h <= f.h+sizeTol; h++ {
				for _, r := range reps[key{w, h, f.holes}] {
					if r < start || end >= 0 && r >= end {
						continue
					}
					if e, ok := match(fs[r], f); ok && e < be {
						b, be = r, e
					}
				}
			}
		}
		return b, be
	}
	const chunk = 1024
	type found struct{ rep, err int }
	pre := make([]found, chunk)
	for c0 := 0; c0 < len(fs); c0 += chunk {
		c1 := min(c0+chunk, len(fs))
		parallelFor(c1-c0, func(k int) {
			pre[k] = found{-1, 0}
			if fs[c0+k].rows != nil {
				r, e := best(c0+k, 0, c0)
				pre[k] = found{r, e}
			}
		})
		for i := c0; i < c1; i++ {
			out[i] = Class{Rep: i}
			f := fs[i]
			if f.rows == nil {
				continue
			}
			r, e := pre[i-c0].rep, pre[i-c0].err
			if r2, e2 := best(i, c0, i); r2 >= 0 && (r < 0 || e2 < e) {
				r = r2
			}
			if r >= 0 {
				// r's top left in f's frame.
				dx, dy := offset(f, fs[r])
				out[i] = Class{Rep: r, DX: dx, DY: dy}
			} else {
				reps[key{f.w, f.h, f.holes}] = append(reps[key{f.w, f.h, f.holes}], i)
			}
		}
	}
	return out
}

// parallelFor runs f(0..n-1) on all CPUs.
func parallelFor(n int, f func(int)) {
	var wg sync.WaitGroup
	next := make(chan int, 256)
	for range min(runtime.GOMAXPROCS(0), n) {
		wg.Go(func() {
			for i := range next {
				f(i)
			}
		})
	}
	for i := range n {
		next <- i
	}
	close(next)
	wg.Wait()
}
