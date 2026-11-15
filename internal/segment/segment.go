// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

// Package segment separates the text of a gray or colour scan from its
// background: a mask of the text (and line art), without photos and
// halftones, and the colour of each mark.
package segment

import (
	"math"

	"github.com/sergev/dejapdf/internal/raster"
	"github.com/sergev/dejapdf/internal/shapes"
)

// Sauvola's threshold: the local mean, lowered where the local
// deviation is small against sauvolaR.
var (
	sauvolaK = 0.34
	sauvolaR = 128.0
)

// Gray returns the luminance of a pixmap, 0 black.
func Gray(p *raster.Pixmap) []byte {
	g := make([]byte, p.W*p.H)
	switch p.N {
	case 1:
		copy(g, p.Pix)
	case 3:
		for i := range g {
			c := p.Pix[3*i : 3*i+3]
			g[i] = byte((299*int(c[0]) + 587*int(c[1]) + 114*int(c[2]) + 500) / 1000)
		}
	case 4:
		for i := range g {
			c := p.Pix[4*i : 4*i+4]
			r, gg, b := cmykRGB(c)
			g[i] = byte((299*int(r) + 587*int(gg) + 114*int(b) + 500) / 1000)
		}
	}
	return g
}

// cmykRGB is the naive conversion.
func cmykRGB(c []byte) (byte, byte, byte) {
	k := 255 - int(c[3])
	return byte((255 - int(c[0])) * k / 255), byte((255 - int(c[1])) * k / 255), byte((255 - int(c[2])) * k / 255)
}

// Threshold binarises gray (w×h, 0 black) with Sauvola's method in a
// window of (2r+1)² pixels: 1 where ink, and wherever darker than dark
// (the inside of strokes wider than the window).
func Threshold(gray []byte, w, h, r int, dark byte) *raster.Bitmap {
	b := &raster.Bitmap{W: w, H: h, Stride: (w + 7) / 8}
	b.Pix = make([]byte, b.Stride*h)
	// Column sums over rows y-r..y+r, updated row by row.
	col := make([]int64, w)
	colSq := make([]int64, w)
	addRow := func(y int, sign int64) {
		row := gray[y*w : (y+1)*w]
		for x, v := range row {
			col[x] += sign * int64(v)
			colSq[x] += sign * int64(v) * int64(v)
		}
	}
	for y := 0; y < min(r, h); y++ {
		addRow(y, 1)
	}
	for y := range h {
		if y+r < h {
			addRow(y+r, 1)
		}
		if y-r-1 >= 0 {
			addRow(y-r-1, -1)
		}
		rows := int64(min(y+r, h-1) - max(y-r, 0) + 1)
		var s, sq int64
		for x := 0; x < min(r, w); x++ {
			s += col[x]
			sq += colSq[x]
		}
		row := gray[y*w : (y+1)*w]
		out := b.Pix[y*b.Stride:]
		for x := range w {
			if x+r < w {
				s += col[x+r]
				sq += colSq[x+r]
			}
			if x-r-1 >= 0 {
				s -= col[x-r-1]
				sq -= colSq[x-r-1]
			}
			n := rows * int64(min(x+r, w-1)-max(x-r, 0)+1)
			m := float64(s) / float64(n)
			sd := math.Sqrt(max(float64(sq)/float64(n)-m*m, 0))
			t := m * (1 + sauvolaK*(sd/sauvolaR-1))
			if float64(row[x]) <= t || row[x] < dark {
				out[x/8] |= 0x80 >> (x % 8)
			}
		}
	}
	return b
}

// Mask returns the text and line art of a scan at dpi: thresholded,
// without specks, photos and halftones.
func Mask(p *raster.Pixmap, dpi float64) *raster.Bitmap {
	g := Gray(p)
	pap := paper(g)
	b := Threshold(g, p.W, p.H, max(4, int(dpi/12)), byte(pap/2))
	fillHoles(b, g, pap)
	clean(b, g, pap, dpi)
	return b
}

// paper is the gray of the paper: the 90th percentile.
func paper(g []byte) int {
	var hist [256]int
	for _, v := range g {
		hist[v]++
	}
	n := 0
	for v := 255; v >= 0; v-- {
		if n += hist[v]; n >= len(g)/10 {
			return v
		}
	}
	return 255
}

// clean erases specks, photos and halftones from b, given the gray g
// and the gray of the paper. Pictures are found by tiles: a tile whose
// paper (its pixels not ink) is dark; one a thick blob covers (print is
// strokes; a photo, a shadow, the edge of the page are areas); one
// crowded with tiny marks (halftone, photo texture). Marks centred in
// a picture tile, or next to one, are erased.
func clean(b *raster.Bitmap, g []byte, pap int, dpi float64) {
	speck := max(2, int(dpi*dpi/20000)) // pixels; 4 at 300 dpi
	ts := max(8, int(dpi/4))            // tile size
	tw, th := (b.W+ts-1)/ts, (b.H+ts-1)/ts
	tile := func(x, y int) int { return (y/ts)*tw + x/ts }
	picture := make([]bool, tw*th)

	cs := shapes.Components(b)
	// The paper of each tile: percentiles of what is not ink nor its
	// halo (not the mean: the inside of hollow strokes is dark).
	near := dilate(b, cs, 1)
	hist := make([][256]int32, tw*th)
	n := make([]int, tw*th)
	for y := range b.H {
		row := near.Pix[y*b.Stride:]
		for x := range b.W {
			if row[x/8]&(0x80>>(x%8)) == 0 {
				i := tile(x, y)
				hist[i][g[y*b.W+x]]++
				n[i]++
			}
		}
	}
	// The paper's level: the 75th percentile.
	level := make([]int, tw*th)
	midtone := make([]bool, tw*th) // over 1% of the tile
	for i := range picture {
		k, hi := 0, 255
		for ; hi > 0; hi-- {
			if k += int(hist[i][hi]); 4*k >= n[i] {
				break
			}
		}
		level[i] = max(hi, 0)
		// Mid-tones away from ink: a photo in continuous tone.
		mid := 0
		for v := range hi * 4 / 5 {
			mid += int(hist[i][v])
		}
		midtone[i] = 100*mid > ts*ts
		picture[i] = n[i] > 0 && (hi < pap*2/3 || 50*mid > ts*ts)
	}

	for _, c := range cs {
		w, h := c.X1-c.X0, c.Y1-c.Y0
		if c.Area < int(dpi*dpi) || float64(c.Area) < 0.2*float64(w*h) {
			continue
		}
		if thick(c, int(dpi/10)) {
			for _, r := range c.Runs {
				for x := r.X0; x < r.X1; x += ts {
					picture[tile(x, r.Y)] = true
				}
				picture[tile(r.X1-1, r.Y)] = true
			}
		}
	}
	dense := halftones(b, dpi)
	// Pictures are regions of picture tiles, 2×2 tiles at least: their
	// boxes, grown by half a tile, cover the parts not found dark.
	type box struct{ x0, y0, x1, y1 int }
	var boxes []box
	seen := make([]bool, len(picture))
	for start := range picture {
		if !picture[start] || seen[start] {
			continue
		}
		bx := box{tw, th, 0, 0}
		stack := []int{start}
		seen[start] = true
		for len(stack) > 0 {
			p := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			x, y := p%tw, p/tw
			bx = box{min(bx.x0, x), min(bx.y0, y), max(bx.x1, x+1), max(bx.y1, y+1)}
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					q, qy := x+dx, y+dy
					if q >= 0 && qy >= 0 && q < tw && qy < th && picture[qy*tw+q] && !seen[qy*tw+q] {
						seen[qy*tw+q] = true
						stack = append(stack, qy*tw+q)
					}
				}
			}
		}
		if bx.x1-bx.x0 >= 2 && bx.y1-bx.y0 >= 2 {
			boxes = append(boxes, box{bx.x0*ts - ts/2, bx.y0*ts - ts/2, bx.x1*ts + ts/2, bx.y1*ts + ts/2})
		}
	}
	// A mark in a picture's box goes with it, unless it is small, near
	// the edge of the box, and its tile clean paper: text beside the
	// picture.
	inPicture := func(c shapes.Component) bool {
		x, y := (c.X0+c.X1)/2, (c.Y0+c.Y1)/2
		i := tile(x, y)
		text := !picture[i] && !midtone[i] && c.X1-c.X0 < ts && c.Y1-c.Y0 < ts
		for _, b := range boxes {
			if x < b.x0 || x >= b.x1 || y < b.y0 || y >= b.y1 {
				continue
			}
			edge := x < b.x0+3*ts/2 || x >= b.x1-3*ts/2 || y < b.y0+3*ts/2 || y >= b.y1-3*ts/2
			if !text || !edge {
				return true
			}
		}
		return false
	}
	for _, c := range cs {
		i := tile((c.X0+c.X1)/2, (c.Y0+c.Y1)/2)
		if c.Area < speck || inPicture(c) || dense(c) || !standsOut(c, b.W, g, level[i]) {
			for _, r := range c.Runs {
				row := b.Pix[r.Y*b.Stride:]
				for x := r.X0; x < r.X1; x++ {
					row[x/8] &^= 0x80 >> (x % 8)
				}
			}
		}
	}
}

// fillHoles fills the holes of the marks of b that are darker than
// halfway from their ink to the paper: the inside of strokes wider than
// the threshold window.
func fillHoles(b *raster.Bitmap, g []byte, pap int) {
	for _, c := range shapes.Components(b) {
		w, h := c.X1-c.X0, c.Y1-c.Y0
		if w < 3 || h < 3 {
			continue
		}
		ink := 0
		on := make([]bool, w*h)
		for _, r := range c.Runs {
			for x := r.X0; x < r.X1; x++ {
				on[(r.Y-c.Y0)*w+x-c.X0] = true
				ink += int(g[r.Y*b.W+x])
			}
		}
		mid := (ink/c.Area + pap) / 2
		// Label the white areas inside the box; those touching its edge
		// are outside.
		seen := make([]bool, w*h)
		var stack, area []int
		for start := range on {
			if on[start] || seen[start] {
				continue
			}
			stack, area = append(stack[:0], start), area[:0]
			seen[start] = true
			edge, sum := false, 0
			for len(stack) > 0 {
				p := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				area = append(area, p)
				x, y := p%w, p/w
				sum += int(g[(c.Y0+y)*b.W+c.X0+x])
				edge = edge || x == 0 || y == 0 || x == w-1 || y == h-1
				for _, q := range [4][2]int{{x - 1, y}, {x + 1, y}, {x, y - 1}, {x, y + 1}} {
					if q[0] >= 0 && q[1] >= 0 && q[0] < w && q[1] < h {
						if i := q[1]*w + q[0]; !on[i] && !seen[i] {
							seen[i] = true
							stack = append(stack, i)
						}
					}
				}
			}
			if !edge && sum < mid*len(area) {
				for _, p := range area {
					x, y := c.X0+p%w, c.Y0+p/w
					b.Pix[y*b.Stride+x/8] |= 0x80 >> (x % 8)
				}
			}
		}
	}
}

// minContrast is how much darker than the paper of its tile print is.
var minContrast = 30.0

// dilate returns b's ink grown by r pixels each way.
func dilate(b *raster.Bitmap, cs []shapes.Component, r int) *raster.Bitmap {
	out := &raster.Bitmap{W: b.W, H: b.H, Stride: b.Stride, Pix: make([]byte, len(b.Pix))}
	for _, c := range cs {
		for _, run := range c.Runs {
			x0, x1 := max(run.X0-r, 0), min(run.X1+r, b.W)
			for y := max(run.Y-r, 0); y < min(run.Y+r+1, b.H); y++ {
				row := out.Pix[y*b.Stride:]
				for x := x0; x < x1; x++ {
					row[x/8] |= 0x80 >> (x % 8)
				}
			}
		}
	}
	return out
}

// standsOut reports whether c is darker than its paper by minContrast:
// grain and stains are not.
func standsOut(c shapes.Component, w int, g []byte, level int) bool {
	ink := 0
	for _, r := range c.Runs {
		for x := r.X0; x < r.X1; x++ {
			ink += int(g[r.Y*w+x])
		}
	}
	return float64(level)-float64(ink)/float64(c.Area) >= minContrast
}

// halftones finds halftones and textures (cloth, photos) as Bloomberg's
// halftone mask in Leptonica does (pageseg.c, written anew): at about
// 150 ppi, areas solid at 1/8 scale over a 5×5 brick seed a fill of the
// image closed by 4×4. It returns whether a mark touches the result.
func halftones(b *raster.Bitmap, dpi float64) func(shapes.Component) bool {
	r := 1 // to about 150 ppi
	if dpi >= 200 {
		r = 2
	}
	w, h := (b.W+r-1)/r, (b.H+r-1)/r
	img := make([]bool, w*h) // OR reduced
	for y := range b.H {
		row := b.Pix[y*b.Stride:]
		for x := range b.W {
			if row[x/8]&(0x80>>(x%8)) != 0 {
				img[(y/r)*w+x/r] = true
			}
		}
	}
	// Seed: 4×4 blocks all ink, opened by 5×5.
	sw, sh := w/4, h/4
	seed := make([]bool, sw*sh)
	for y := range sh {
		for x := range sw {
			all := true
			for dy := 0; dy < 4 && all; dy++ {
				for dx := 0; dx < 4 && all; dx++ {
					all = img[(4*y+dy)*w+4*x+dx]
				}
			}
			seed[y*sw+x] = all
		}
	}
	seed = dilateGrid(erodeGrid(seed, sw, sh, 5), sw, sh, 5)
	// Fill the seed, back at the reduced scale, through the closed image.
	mask := erodeGrid(dilateGrid(img, w, h, 4), w, h, 4)
	ht := make([]bool, w*h)
	var stack []int
	for y := range sh {
		for x := range sw {
			if !seed[y*sw+x] {
				continue
			}
			for dy := range 4 {
				for dx := range 4 {
					if p := (4*y+dy)*w + 4*x + dx; mask[p] && !ht[p] {
						ht[p] = true
						stack = append(stack, p)
					}
				}
			}
		}
	}
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		x, y := p%w, p/w
		for _, q := range [4][2]int{{x - 1, y}, {x + 1, y}, {x, y - 1}, {x, y + 1}} {
			if q[0] >= 0 && q[1] >= 0 && q[0] < w && q[1] < h {
				if i := q[1]*w + q[0]; mask[i] && !ht[i] {
					ht[i] = true
					stack = append(stack, i)
				}
			}
		}
	}
	return func(c shapes.Component) bool {
		for _, run := range c.Runs {
			for x := run.X0; x < run.X1; x += r {
				if ht[(run.Y/r)*w+x/r] {
					return true
				}
			}
		}
		return false
	}
}

// erodeGrid keeps the cells whose k×k brick, centred, is all on.
func erodeGrid(on []bool, w, h, k int) []bool {
	return brick(on, w, h, k, true)
}

// dilateGrid sets the cells whose k×k brick, centred, has any on.
func dilateGrid(on []bool, w, h, k int) []bool {
	return brick(on, w, h, k, false)
}

// brick applies a k×k brick, rows then columns: all (erode) or any.
func brick(on []bool, w, h, k int, all bool) []bool {
	r0, r1 := k/2, k-1-k/2
	pass := func(src []bool, n, stride, lines, lstride int) []bool {
		out := make([]bool, len(src))
		for l := range lines {
			// Running count of on cells in the window.
			at := func(i int) bool { return src[l*lstride+i*stride] }
			cnt := 0
			for i := 0; i < min(r1, n); i++ {
				if at(i) {
					cnt++
				}
			}
			for i := range n {
				if j := i + r1; j < n && at(j) {
					cnt++
				}
				if j := i - r0 - 1; j >= 0 && at(j) {
					cnt--
				}
				win := min(i+r1, n-1) - max(i-r0, 0) + 1
				if all {
					out[l*lstride+i*stride] = cnt == win
				} else {
					out[l*lstride+i*stride] = cnt > 0
				}
			}
		}
		return out
	}
	rows := pass(on, w, 1, h, w)
	return pass(rows, h, w, w, 1)
}

// thick reports whether over 30% of c lies in runs at least k long
// both across and down.
func thick(c shapes.Component, k int) bool {
	w, h := c.X1-c.X0, c.Y1-c.Y0
	// Pixels in long horizontal runs.
	across := make([]bool, w*h)
	for _, r := range c.Runs {
		if r.X1-r.X0 >= k {
			for x := r.X0; x < r.X1; x++ {
				across[(r.Y-c.Y0)*w+x-c.X0] = true
			}
		}
	}
	n := 0
	for x := range w {
		for y := 0; y < h; {
			if !across[y*w+x] {
				y++
				continue
			}
			e := y
			for e < h && across[e*w+x] {
				e++
			}
			if e-y >= k {
				n += e - y
			}
			y = e
		}
	}
	return 10*n > 3*c.Area
}
