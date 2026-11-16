// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package segment

import "github.com/sergev/dejapdf/internal/raster"

// Background reduces p by k, filling what the mask m hides from the
// paper around it. Pixels within r0 of the mask (the blurred edges of
// the ink) are left out. Those within r1 (JPEG ringing, the shadow of
// the ink) count less the nearer they are, and not at all if darker
// than the paper around by ghost: the fill from the pixels beyond r1
// alone. Cells left with no weight are filled by push-pull: a pyramid
// of weighted means, each level filling the holes of the one below by
// bilinear interpolation. The text leaves no ghost, and the fill is
// smooth, so the wavelets spend nothing on it. The result is gray or
// RGB.
func Background(p *raster.Pixmap, m *raster.Bitmap, k, r0, r1 int) *raster.Pixmap {
	d := distance(m, p.W, p.H, r1)
	far := reduce(p, k, func(y int, wt []float64) {
		for x, di := range d[y*p.W : (y+1)*p.W] {
			wt[x] = float64(b2f(int(di) >= r1))
		}
	})
	pull(far)
	g := Gray(p)
	w := (p.W + k - 1) / k
	lv := reduce(p, k, func(y int, wt []float64) {
		i0 := y * p.W
		for x := range wt {
			di := int(d[i0+x])
			switch {
			case di >= r1:
				wt[x] = 1
			case di <= r0, float64(g[i0+x]) < lum(far, y/k*w+x/k)-ghost:
				wt[x] = 0
			default:
				wt[x] = float64(di-r0) / float64(r1-r0)
			}
		}
	})
	pull(lv)
	out := &raster.Pixmap{W: lv.w, H: lv.h, N: lv.n, Pix: make([]byte, len(lv.v))}
	for i, v := range lv.v {
		out.Pix[i] = byte(min(max(v+0.5, 0), 255))
	}
	return out
}

// ghost is how much darker than the paper around a pixel near the ink
// may be and still count as paper.
var ghost = 6.0

func b2f(b bool) int {
	if b {
		return 1
	}
	return 0
}

// lum is the luminance of cell c.
func lum(l *level, c int) float64 {
	if l.n == 1 {
		return l.v[c]
	}
	v := l.v[3*c:]
	return 0.299*v[0] + 0.587*v[1] + 0.114*v[2]
}

// reduce is level 0 of the pyramid: p reduced by k, each cell the mean
// of its pixels by weight, its weight their share of the cell. weights
// sets the weights of row y.
func reduce(p *raster.Pixmap, k int, weights func(y int, wt []float64)) *level {
	n := min(p.N, 3)
	w, h := (p.W+k-1)/k, (p.H+k-1)/k
	lv := &level{w: w, h: h, n: n, v: make([]float64, n*w*h), wt: make([]float64, w*h)}
	v, cwt, pix := lv.v, lv.wt, p.Pix
	wt := make([]float64, p.W)
	for y := range p.H {
		weights(y, wt)
		c, left := y/k*w, k // the cell, and its pixels left in this row
		for x, pw := range wt {
			if left == 0 {
				c, left = c+1, k
			}
			left--
			i := y*p.W + x
			if pw <= 0 {
				continue
			}
			cwt[c] += pw
			switch p.N {
			case 1:
				v[c] += pw * float64(pix[i])
			case 3:
				s, d := pix[3*i:3*i+3], v[3*c:3*c+3]
				d[0] += pw * float64(s[0])
				d[1] += pw * float64(s[1])
				d[2] += pw * float64(s[2])
			default:
				px := pixel(p, i)
				for j := range n {
					v[n*c+j] += pw * float64(px[j])
				}
			}
		}
	}
	for c, cw := range lv.wt {
		if cw == 0 {
			continue
		}
		for j := range n {
			lv.v[n*c+j] /= cw
		}
		lv.wt[c] = cw / float64(min(k, p.W-c%w*k)*min(k, p.H-c/w*k))
	}
	return lv
}

// level is one level of the pyramid: n channels, weights in 0..1.
type level struct {
	w, h, n int
	v, wt   []float64
}

// pull fills l from a coarser level: a cell of weight w keeps w of its
// own value and takes the rest from the parents.
func pull(l *level) {
	full := true
	for _, x := range l.wt {
		full = full && x == 1
	}
	if full {
		return
	}
	if l.w == 1 && l.h == 1 {
		for i := range l.v {
			l.v[i] = l.wt[0]*l.v[i] + (1-l.wt[0])*255
		}
		return
	}
	pw, ph := (l.w+1)/2, (l.h+1)/2
	up := &level{w: pw, h: ph, n: l.n, v: make([]float64, l.n*pw*ph), wt: make([]float64, pw*ph)}
	for y := range l.h {
		for x := range l.w {
			c, pc := y*l.w+x, y/2*pw+x/2
			wt := l.wt[c]
			up.wt[pc] += wt
			for i := range l.n {
				up.v[l.n*pc+i] += wt * l.v[l.n*c+i]
			}
		}
	}
	for pc, wt := range up.wt {
		if wt > 0 {
			for i := range l.n {
				up.v[l.n*pc+i] /= wt
			}
		}
		up.wt[pc] = min(wt, 1)
	}
	pull(up)
	for y := range l.h {
		fy := max(float64(y)/2-0.25, 0)
		y0 := min(int(fy), ph-1)
		y1, ay := min(y0+1, ph-1), fy-float64(int(fy))
		for x := range l.w {
			c := y*l.w + x
			wt := l.wt[c]
			if wt == 1 {
				continue
			}
			fx := max(float64(x)/2-0.25, 0)
			x0 := min(int(fx), pw-1)
			x1, ax := min(x0+1, pw-1), fx-float64(int(fx))
			for i := range l.n {
				at := func(px, py int) float64 { return up.v[l.n*(py*pw+px)+i] }
				v := (1-ay)*((1-ax)*at(x0, y0)+ax*at(x1, y0)) + ay*((1-ax)*at(x0, y1)+ax*at(x1, y1))
				l.v[l.n*c+i] = wt*l.v[l.n*c+i] + (1-wt)*v
			}
		}
	}
}

// distance is each pixel's chessboard distance from the set pixels of
// m, up to max.
func distance(m *raster.Bitmap, w, h, limit int) []uint16 {
	lim := uint16(min(limit, 1<<15))
	d := make([]uint16, w*h)
	for y := range h {
		for x := range w {
			if y < m.H && x < m.W && m.Pix[y*m.Stride+x/8]>>(7-x%8)&1 != 0 {
				continue
			}
			v := lim
			if x > 0 {
				v = min(v, d[y*w+x-1]+1)
			}
			if y > 0 {
				for dx := max(x-1, 0); dx <= min(x+1, w-1); dx++ {
					v = min(v, d[(y-1)*w+dx]+1)
				}
			}
			d[y*w+x] = v
		}
	}
	for y := h - 1; y >= 0; y-- {
		for x := w - 1; x >= 0; x-- {
			v := d[y*w+x]
			if x < w-1 {
				v = min(v, d[y*w+x+1]+1)
			}
			if y < h-1 {
				for dx := max(x-1, 0); dx <= min(x+1, w-1); dx++ {
					v = min(v, d[(y+1)*w+dx]+1)
				}
			}
			d[y*w+x] = v
		}
	}
	return d
}

// pixel is pixel i of p, gray or RGB.
func pixel(p *raster.Pixmap, i int) [3]byte {
	switch p.N {
	case 1:
		return [3]byte{p.Pix[i]}
	case 3:
		return [3]byte(p.Pix[3*i : 3*i+3])
	}
	r, g, b := cmykRGB(p.Pix[4*i : 4*i+4])
	return [3]byte{r, g, b}
}

// Uniform reports whether p is one colour within ±2, and which.
func Uniform(p *raster.Pixmap) (Color, bool) {
	var lo, hi [3]byte
	for c := range p.N {
		lo[c], hi[c] = 255, 0
	}
	for i, v := range p.Pix {
		c := i % p.N
		lo[c], hi[c] = min(lo[c], v), max(hi[c], v)
		if hi[c]-lo[c] > 4 {
			return Color{}, false
		}
	}
	var mid Color
	for c := range p.N {
		mid[c] = byte((int(lo[c]) + int(hi[c]) + 1) / 2)
	}
	if p.N == 1 {
		mid[1], mid[2] = mid[0], mid[0]
	}
	return mid, true
}
