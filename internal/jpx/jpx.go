// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

// Package jpx encodes 8-bit grey or RGB images as JPEG 2000 (JP2) files
// for PDF's JPXDecode, cut to a byte budget by PCRD-opt rate control.
// See docs/JPEG_2000_Image_Coding.md.
package jpx

import (
	"encoding/binary"
	"errors"
	"math"
	"runtime"
	"sort"
	"sync"
)

const (
	cbLog   = 6   // 64×64 code-blocks
	maxNL   = 5   // decomposition levels
	quantQ  = 1.0 // image-domain step: Δb = quantQ / ‖band synthesis‖
	cbSize  = 1 << cbLog
	maxSize = 1 << 16
)

// Inverse ICT column norms: how much a unit error in Y, Cb, Cr moves R,
// G, B (G.3).
var ictNorm = [3]float64{
	math.Sqrt(3),
	math.Sqrt(0.34413*0.34413 + 1.772*1.772),
	math.Sqrt(1.402*1.402 + 0.71414*0.71414),
}

// band is one sub-band of one component.
type band struct {
	orient    int
	x0, y0    int // in the transformed plane
	w, h      int
	eps, mu   int     // step size exponent and mantissa (E.1.1.1)
	step      float64 // Δb
	weight    float64 // squared error scale to image units
	ncx, ncy  int
	blocks    []block
	incl, zbp *tagTreeEnc
}

// block is a coded code-block.
type block struct {
	data   []byte
	passes []pass
	nbp    int   // magnitude bit-planes
	zbp    int   // missing (all-zero) top bit-planes
	keep   int   // passes kept
	hull   []int // pass indexes on the convex hull, + 1
	slope  []float64
}

// Encode codes the w×h image in planes (one grey or three RGB planes, row
// by row) as a JP2 file of at most budget bytes; budget 0 keeps every
// pass.
func Encode(w, h int, planes [][]byte, budget int) ([]byte, error) {
	nc := len(planes)
	if nc != 1 && nc != 3 {
		return nil, errors.New("jpx: need 1 or 3 planes")
	}
	if w <= 0 || h <= 0 || w > maxSize || h > maxSize {
		return nil, errors.New("jpx: bad size")
	}
	for _, p := range planes {
		if len(p) != w*h {
			return nil, errors.New("jpx: bad plane size")
		}
	}
	nl := maxNL
	for nl > 0 && min(w, h) < 1<<nl {
		nl--
	}
	comps := colour(w, h, planes)
	res := make([][][]*band, nl+1) // bands by resolution and component
	for r := range res {
		res[r] = make([][]*band, nc)
	}
	var all []*band
	var bands []*band
	for c := range nc {
		forward(comps[c], w, h, nl)
		bands = layout(w, h, nl)
		for i, b := range bands {
			b.weight *= b.step * b.step / (1 << (2 * fracBits))
			if nc == 3 {
				b.weight *= ictNorm[c] * ictNorm[c]
			}
			r := 0
			if i > 0 {
				r = (i-1)/3 + 1
			}
			res[r][c] = append(res[r][c], b)
			all = append(all, b)
		}
	}
	codeBlocks(all, comps, w, len(bands))
	// Guard bits so every block's bit-planes fit Mb = G + ε − 1 (E-2).
	guard := 2
	for _, b := range all {
		for i := range b.blocks {
			guard = max(guard, b.blocks[i].nbp-b.eps+1)
		}
	}
	if guard > 7 {
		return nil, errors.New("jpx: coefficients too large")
	}
	for _, b := range all {
		for i := range b.blocks {
			cb := &b.blocks[i]
			cb.zbp = guard + b.eps - 1 - cb.nbp
			cb.keep = len(cb.passes)
		}
	}
	head := header(w, h, nc, nl, guard, bands)
	var pw pktWriter
	size := func() int {
		n := len(head) + 2 + 2 + 85 // SOD, EOC, JP2 boxes
		for r := range res {
			for c := range nc {
				pw.buf = pw.buf[:0]
				n += packet(&pw, res[r][c]) + len(pw.buf)
			}
		}
		return n
	}
	if budget > 0 && size() > budget {
		allocate(all, budget, size)
	}
	cs := append(head, 0xff, 0x93) // SOD
	for r := range res {
		for c := range nc {
			pw.buf = pw.buf[:0]
			packet(&pw, res[r][c])
			cs = append(cs, pw.buf...)
			for _, b := range res[r][c] {
				for _, cb := range b.blocks {
					if cb.keep > 0 {
						cs = append(cs, cb.data[:cb.passes[cb.keep-1].rate]...)
					}
				}
			}
		}
	}
	// Psot: from SOT to the end of the tile data.
	binary.BigEndian.PutUint32(cs[len(head)-6:], uint32(len(cs)-len(head)+12))
	cs = append(cs, 0xff, 0xd9)
	return jp2(w, h, nc, cs), nil
}

// colour level-shifts the planes and applies the ICT to RGB (G.1, G.3).
func colour(w, h int, planes [][]byte) [][]float32 {
	n := w * h
	comps := make([][]float32, len(planes))
	for c := range comps {
		comps[c] = make([]float32, n)
	}
	if len(planes) == 1 {
		for i, v := range planes[0] {
			comps[0][i] = float32(v) - 128
		}
		return comps
	}
	for i := range n {
		r := float32(planes[0][i]) - 128
		g := float32(planes[1][i]) - 128
		b := float32(planes[2][i]) - 128
		comps[0][i] = 0.299*r + 0.587*g + 0.114*b
		comps[1][i] = -0.16875*r - 0.33126*g + 0.5*b
		comps[2][i] = 0.5*r - 0.41869*g - 0.08131*b
	}
	return comps
}

// layout returns the bands of one component in codestream order, with
// their step sizes.
func layout(w, h, nl int) []*band {
	var bs []*band
	add := func(o, x0, y0, x1, y1 int, norm float64) {
		gain := [4]int{0, 1, 1, 2}[o]
		eps, mu, step := quantStep(quantQ/norm, 8+gain)
		bw, bh := x1-x0, y1-y0
		b := &band{orient: o, x0: x0, y0: y0, w: bw, h: bh, eps: eps, mu: mu, step: step}
		b.weight = norm * norm
		b.ncx, b.ncy = (bw+cbSize-1)/cbSize, (bh+cbSize-1)/cbSize
		if bw == 0 || bh == 0 {
			b.ncx, b.ncy = 0, 0
		}
		b.incl = newTagTreeEnc(b.ncx, b.ncy)
		b.zbp = newTagTreeEnc(b.ncx, b.ncy)
		bs = append(bs, b)
	}
	lo := 1.0
	if nl > 0 {
		lo, _ = norms97(nl)
	}
	add(bandLL, 0, 0, ceilDiv(w, nl), ceilDiv(h, nl), lo*lo)
	for d := nl; d >= 1; d-- {
		// A level-d detail is one high-pass and d-1 low-pass syntheses.
		l, hi := norms97(d)
		pw, ph := ceilDiv(w, d-1), ceilDiv(h, d-1)
		lw, lh := ceilDiv(w, d), ceilDiv(h, d)
		add(bandHL, lw, 0, pw, lh, hi*l)
		add(bandLH, 0, lh, lw, ph, l*hi)
		add(bandHH, lw, lh, pw, ph, hi*hi)
	}
	return bs
}

// quantStep rounds step size s to the exponent and mantissa form for a
// band of nominal range rb bits.
func quantStep(s float64, rb int) (eps, mu int, step float64) {
	e := math.Floor(math.Log2(s))
	eps = rb - int(e)
	mu = int(math.Round((s/math.Exp2(e) - 1) * 2048))
	if mu == 2048 {
		mu = 0
		eps--
	}
	eps = max(0, min(31, eps))
	step = math.Exp2(float64(rb-eps)) * (1 + float64(mu)/2048)
	return
}

// codeBlocks quantizes and codes every code-block, in parallel.
func codeBlocks(all []*band, comps [][]float32, stride, nb int) {
	type job struct {
		b    *band
		c, i int
	}
	jobs := make(chan job, 64)
	var wg sync.WaitGroup
	for range runtime.GOMAXPROCS(0) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var t t1
			for j := range jobs {
				b := j.b
				cx, cy := j.i%b.ncx, j.i/b.ncx
				x0, y0 := cx*cbSize, cy*cbSize
				bw, bh := min(cbSize, b.w-x0), min(cbSize, b.h-y0)
				t.reset(bw, bh)
				src := comps[j.c]
				inv := (1 << fracBits) / b.step
				for y := range bh {
					row := src[(b.y0+y0+y)*stride+b.x0+x0:]
					for x := range bw {
						v := float64(row[x]) * inv
						if v < 0 {
							v = -v
							t.setNeg(x, y)
						}
						t.mag[y*bw+x] = uint32(v)
					}
				}
				nbp, data, ps := t.code(b.orient)
				cb := &b.blocks[j.i]
				cb.nbp = nbp
				cb.data = append([]byte(nil), data...)
				cb.passes = append([]pass(nil), ps...)
			}
		}()
	}
	for k, b := range all {
		b.blocks = make([]block, b.ncx*b.ncy)
		for i := range b.blocks {
			jobs <- job{b, k / nb, i}
		}
	}
	close(jobs)
	wg.Wait()
}

// allocate chooses each block's kept passes for the least distortion
// within budget (PCRD-opt, J.13): it keeps the hull points whose
// distortion-rate slope reaches a threshold, searched over all slopes.
func allocate(all []*band, budget int, size func() int) {
	var slopes []float64
	for _, b := range all {
		for i := range b.blocks {
			cb := &b.blocks[i]
			hullOf(cb, b.weight)
			slopes = append(slopes, cb.slope...)
		}
	}
	sort.Sort(sort.Reverse(sort.Float64Slice(slopes)))
	set := func(lambda float64) {
		for _, b := range all {
			for i := range b.blocks {
				cb := &b.blocks[i]
				cb.keep = 0
				for k, s := range cb.slope {
					if s >= lambda {
						cb.keep = cb.hull[k]
					}
				}
			}
		}
	}
	// Largest n such that keeping slopes[:n] fits.
	n := sort.Search(len(slopes), func(n int) bool {
		set(slopes[n])
		return size() > budget
	})
	if n == 0 {
		set(math.Inf(1))
	} else {
		set(slopes[n-1])
	}
}

// hullOf finds the passes on the lower convex hull of the block's
// rate-distortion curve and their slopes, decreasing.
func hullOf(cb *block, weight float64) {
	cb.hull, cb.slope = cb.hull[:0], cb.slope[:0]
	for k, p := range cb.passes {
		d := p.dist * weight
		for {
			n := len(cb.hull)
			r0, d0 := 0, 0.0
			if n > 0 {
				q := cb.passes[cb.hull[n-1]-1]
				r0, d0 = q.rate, q.dist*weight
			}
			if d <= d0 {
				break // no gain
			}
			if n > 0 && (p.rate <= r0 || (d-d0)/float64(p.rate-r0) >= cb.slope[n-1]) {
				cb.hull, cb.slope = cb.hull[:n-1], cb.slope[:n-1]
				continue // the previous point is not convex
			}
			if p.rate > r0 {
				cb.hull = append(cb.hull, k+1)
				cb.slope = append(cb.slope, (d-d0)/float64(p.rate-r0))
			}
			break
		}
	}
}

// header returns the main header and the SOT marker segment (A.4, A.5,
// A.6); Psot, 6 bytes from the end, is patched later.
func header(w, h, nc, nl, guard int, bs []*band) []byte {
	b := []byte{0xff, 0x4f}
	// SIZ
	b = be16(b, 0xff51, 38+3*nc, 0)
	b = be32(b, w, h, 0, 0, w, h, 0, 0)
	b = be16(b, nc)
	for range nc {
		b = append(b, 7, 1, 1)
	}
	// COD: LRCP, 1 layer, MCT, nl levels, 64×64, style 0, 9/7.
	b = be16(b, 0xff52, 12)
	b = append(b, 0, 0, 0, 1, b2u(nc == 3), byte(nl), cbLog-2, cbLog-2, 0, 0)
	// QCD: expounded.
	b = be16(b, 0xff5c, 3+2*len(bs))
	b = append(b, byte(guard<<5|2))
	for _, s := range bs {
		b = be16(b, s.eps<<11|s.mu)
	}
	// SOT: tile 0, Psot, part 0 of 1.
	b = be16(b, 0xff90, 10, 0)
	b = be32(b, 0)
	return append(b, 0, 1)
}

func be16(b []byte, v ...int) []byte {
	for _, x := range v {
		b = binary.BigEndian.AppendUint16(b, uint16(x))
	}
	return b
}

func be32(b []byte, v ...int) []byte {
	for _, x := range v {
		b = binary.BigEndian.AppendUint32(b, uint32(x))
	}
	return b
}

// jp2 wraps a codestream in the JP2 boxes (I.5).
func jp2(w, h, nc int, cs []byte) []byte {
	b := be32(nil, 12, 0x6a502020, 0x0d0a870a)
	b = be32(b, 20)
	b = append(b, "ftypjp2 \x00\x00\x00\x00jp2 "...)
	b = be32(b, 45)
	b = append(b, "jp2h"...)
	b = be32(b, 22)
	b = append(b, "ihdr"...)
	b = be32(b, h, w)
	b = be16(b, nc)
	b = append(b, 7, 7, 0, 0)
	cs4 := 16
	if nc == 1 {
		cs4 = 17
	}
	b = be32(b, 15)
	b = append(b, "colr\x01\x00\x00"...)
	b = be32(b, cs4, 8+len(cs))
	b = append(b, "jp2c"...)
	return append(b, cs...)
}
