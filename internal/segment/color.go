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
}

// Quantisation: near-black marks are black; other colours closer than
// mergeDist are one; at most maxColors.
var (
	blackLevel  = 96 // luminance of the darkest ink taken for black
	blackChroma = 48
	mergeDist   = 40
	maxColors   = 8
)

// Colors gives the marks of j, made from p's mask, their colours: the
// mean of the darker half of their pixels (the edges are mixed with
// the paper), quantised.
func Colors(p *raster.Pixmap, j *jb2.Image) *Foreground {
	type mark struct {
		c      [3]float64
		weight int
	}
	marks := make([]mark, len(j.Blits))
	var lum []int
	var px [][3]int
	for k, bl := range j.Blits {
		s := j.Shape(bl.Shape)
		top := j.H - bl.Y - s.H
		lum, px = lum[:0], px[:0]
		for y := range s.H {
			for x := range s.W {
				if s.Pix[y*s.W+x] == 0 {
					continue
				}
				X, Y := bl.X+x, top+y
				if X < 0 || Y < 0 || X >= p.W || Y >= p.H {
					continue
				}
				c := rgb(p, Y*p.W+X)
				px = append(px, c)
				lum = append(lum, (299*c[0]+587*c[1]+114*c[2])/1000)
			}
		}
		if len(px) == 0 {
			continue
		}
		mid := slices.Clone(lum)
		slices.Sort(mid)
		m := mid[len(mid)/2]
		var sum [3]float64
		n := 0
		for i, c := range px {
			if lum[i] <= m {
				for k := range 3 {
					sum[k] += float64(c[k])
				}
				n++
			}
		}
		for k := range 3 {
			sum[k] /= float64(n)
		}
		marks[k] = mark{sum, len(px)}
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
	// k-means, weighted by area.
	assign := make([]int, len(marks))
	for range 10 {
		sums := make([][4]float64, len(centers))
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
	for _, c := range centers {
		f.Palette = append(f.Palette, Color{byte(c[0] + 0.5), byte(c[1] + 0.5), byte(c[2] + 0.5)})
	}
	for _, i := range colored {
		f.Index[i] = 1 + assign[i]
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

func dist(a, b [3]float64) float64 {
	d := 0.0
	for k := range 3 {
		d = max(d, abs(a[k]-b[k]))
	}
	return d
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
