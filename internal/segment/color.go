// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package segment

import (
	"slices"

	"github.com/sergev/dejapdf/internal/jb2"
	"github.com/sergev/dejapdf/internal/raster"
)

// Color is an RGB colour; gray scans have R = G = B.
type Color [3]byte

// Foreground is the colour of each mark: an index into a small palette,
// by blit.
type Foreground struct {
	Palette []Color
	Index   []int
	Area    []int // pixels in each colour
}

// Quantisation: near-black marks are black; other colours closer than
// mergeDist are one; at most maxColors.
var (
	blackLevel  = 96 // luminance of the darkest ink taken for black
	blackChroma = 48
	mergeDist   = 40
	maxColors   = 8
	halo        = 2   // pixels around a mark its colour spreads to
	minShift    = 1.0 // least darkening of the paper around to take a hue from
)

// Colors gives the marks of j, made from p's mask, their colours. The
// lightness is the mean of a mark's pixels from the 25th to the 50th
// percentile: the lighter half is the edges, mixed with the paper; the
// darkest quarter is noise. The hue is that of the mark and its
// halo summed against the paper around: blur, and the chroma
// subsampling of JPEG, spread a thin stroke's colour, but keep its sum.
// The colours are then quantised.
func Colors(p *raster.Pixmap, j *jb2.Image) *Foreground {
	type mark struct {
		c      [3]float64
		weight int
	}
	// Which mark each pixel belongs to, +1.
	owner := make([]int32, p.W*p.H)
	for k, bl := range j.Blits {
		s := j.Shape(bl.Shape)
		top := j.H - bl.Y - s.H
		for y := range s.H {
			for x := range s.W {
				X, Y := bl.X+x, top+y
				if s.Pix[y*s.W+x] != 0 && X >= 0 && Y >= 0 && X < p.W && Y < p.H {
					owner[Y*p.W+X] = int32(k + 1)
				}
			}
		}
	}
	marks := make([]mark, len(j.Blits))
	var lum []int
	var px [][3]int
	var paper []int // luminance and pixel index of paper around
	for k, bl := range j.Blits {
		s := j.Shape(bl.Shape)
		top := j.H - bl.Y - s.H
		lum, px, paper = lum[:0], px[:0], paper[:0]
		var sum [3]float64 // over the mark and its halo
		x0, y0 := max(bl.X-halo, 0), max(top-halo, 0)
		x1, y1 := min(bl.X+s.W+halo, p.W), min(top+s.H+halo, p.H)
		for Y := y0; Y < y1; Y++ {
			for X := x0; X < x1; X++ {
				i := Y*p.W + X
				o := int(owner[i])
				if o != 0 && o != k+1 {
					continue
				}
				c := rgb(p, i)
				for n := range 3 {
					sum[n] += float64(c[n])
				}
				l := (299*c[0] + 587*c[1] + 114*c[2]) / 1000
				if o == k+1 {
					px = append(px, c)
					lum = append(lum, l)
				} else {
					paper = append(paper, l<<32|i)
				}
			}
		}
		if len(px) == 0 {
			continue
		}
		mid := slices.Clone(lum)
		slices.Sort(mid)
		lo, m := mid[len(mid)/4], mid[len(mid)/2]
		var core [3]float64
		n := 0
		for i, c := range px {
			if lum[i] >= lo && lum[i] <= m {
				for k := range 3 {
					core[k] += float64(c[k])
				}
				n++
			}
		}
		for k := range 3 {
			core[k] /= float64(n)
		}
		marks[k] = mark{core, len(px)}
		// Paper: the lighter half of the pixels around.
		if len(paper) < 4 {
			continue
		}
		slices.Sort(paper)
		var pp [3]float64
		half := paper[len(paper)/2:]
		for _, v := range half {
			c := rgb(p, v&(1<<32-1))
			for n := range 3 {
				pp[n] += float64(c[n])
			}
		}
		cnt := float64(len(px) + len(paper))
		var dir [3]float64
		for n := range 3 {
			pp[n] /= float64(len(half))
			dir[n] = sum[n]/cnt - pp[n]
		}
		dl := luma(dir)
		if dl > -minShift {
			continue // no darker than the paper: keep the core
		}
		t := (luma(core) - luma(pp)) / dl
		var c [3]float64
		for n := range 3 {
			c[n] = min(max(pp[n]+t*dir[n], 0), 255)
		}
		marks[k].c = c
	}

	// Black first; then seeds from the heaviest colours, far apart.
	f := &Foreground{Palette: []Color{{0, 0, 0}}, Index: make([]int, len(marks))}
	isBlack := func(c [3]float64) bool {
		lo, hi := min(c[0], c[1], c[2]), max(c[0], c[1], c[2])
		return (299*c[0]+587*c[1]+114*c[2])/1000 <= float64(blackLevel) && hi-lo <= float64(blackChroma)
	}
	var colored []int
	for i, m := range marks {
		if m.weight > 0 && !isBlack(m.c) {
			colored = append(colored, i)
		}
	}
	slices.SortFunc(colored, func(a, b int) int { return marks[b].weight - marks[a].weight })
	var centers [][3]float64
	for _, i := range colored {
		far := true
		for _, c := range centers {
			far = far && dist(c, marks[i].c) > float64(mergeDist)
		}
		if far && len(centers) < maxColors-1 {
			centers = append(centers, marks[i].c)
		}
	}
	// k-means, weighted by area; centres that come out close are merged,
	// and again.
	assign := make([]int, len(marks))
	for {
		var sums [][4]float64
		for range 10 {
			sums = make([][4]float64, len(centers))
			for _, i := range colored {
				best, bd := 0, dist(centers[0], marks[i].c)
				for k := 1; k < len(centers); k++ {
					if d := dist(centers[k], marks[i].c); d < bd {
						best, bd = k, d
					}
				}
				assign[i] = best
				w := float64(marks[i].weight)
				for k := range 3 {
					sums[best][k] += w * marks[i].c[k]
				}
				sums[best][3] += w
			}
			for k := range centers {
				if sums[k][3] > 0 {
					centers[k] = [3]float64{sums[k][0] / sums[k][3], sums[k][1] / sums[k][3], sums[k][2] / sums[k][3]}
				}
			}
		}
		a, b := -1, -1
		for k := range centers {
			for l := k + 1; l < len(centers); l++ {
				if dist(centers[k], centers[l]) <= float64(mergeDist) {
					a, b = k, l
				}
			}
		}
		if a < 0 {
			break
		}
		wa, wb := sums[a][3], sums[b][3]
		for k := range 3 {
			centers[a][k] = (wa*centers[a][k] + wb*centers[b][k]) / max(wa+wb, 1)
		}
		centers = slices.Delete(centers, b, b+1)
	}
	for _, c := range centers {
		f.Palette = append(f.Palette, Color{byte(c[0] + 0.5), byte(c[1] + 0.5), byte(c[2] + 0.5)})
	}
	// Each mark takes the nearest colour, black included: a dark mark
	// of a coloured line is that colour.
	for i, m := range marks {
		if m.weight == 0 || len(centers) == 0 {
			continue
		}
		best, bd := 0, dist([3]float64{}, m.c)
		for k, c := range centers {
			if d := dist(c, m.c); d < bd {
				best, bd = k+1, d
			}
		}
		f.Index[i] = best
	}
	f.Area = make([]int, len(f.Palette))
	for i, m := range marks {
		f.Area[f.Index[i]] += m.weight
	}
	return f
}

// rgb is pixel i of p as RGB.
func rgb(p *raster.Pixmap, i int) [3]int {
	switch p.N {
	case 1:
		v := int(p.Pix[i])
		return [3]int{v, v, v}
	case 3:
		c := p.Pix[3*i : 3*i+3]
		return [3]int{int(c[0]), int(c[1]), int(c[2])}
	}
	r, g, b := cmykRGB(p.Pix[4*i : 4*i+4])
	return [3]int{int(r), int(g), int(b)}
}

// dist compares colours by luma, at half weight (the lightness of
// thin marks varies), and chroma.
func dist(a, b [3]float64) float64 {
	d := [3]float64{a[0] - b[0], a[1] - b[1], a[2] - b[2]}
	y := luma(d)
	cb := -0.16875*d[0] - 0.33126*d[1] + 0.5*d[2]
	cr := 0.5*d[0] - 0.41869*d[1] - 0.08131*d[2]
	return max(abs(y)/2, abs(cb), abs(cr))
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func luma(c [3]float64) float64 { return 0.299*c[0] + 0.587*c[1] + 0.114*c[2] }

// inkRatio is the lightest the ink may be, as a fraction of the
// paper's luminance.
var inkRatio = 1.0 / 3

// Paper is the luminance of p's paper: the 90th percentile.
func Paper(p *raster.Pixmap) int { return paper(Gray(p)) }

// Enhance darkens faded ink: when the near-neutral ink (gray, brown,
// faded black; all such colours weighted by area) is lighter than
// inkRatio of the paper's luminance, those colours are scaled down by
// one factor to bring it there. Saturated colours are as printed, and
// the paper is left as it is.
func (f *Foreground) Enhance(paper int) {
	neutral := func(c Color) bool {
		return int(max(c[0], c[1], c[2]))-int(min(c[0], c[1], c[2])) <= blackChroma
	}
	sum, n := 0.0, 0
	for k, c := range f.Palette {
		if neutral(c) {
			sum += float64(f.Area[k]) * luma([3]float64{float64(c[0]), float64(c[1]), float64(c[2])})
			n += f.Area[k]
		}
	}
	if n == 0 {
		return
	}
	ink, want := sum/float64(n), inkRatio*float64(paper)
	if ink <= want {
		return
	}
	k := want / ink
	for i, c := range f.Palette {
		if neutral(c) {
			for j := range 3 {
				f.Palette[i][j] = byte(float64(c[j])*k + 0.5)
			}
		}
	}
}
