// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package jbig2

import (
	"encoding/binary"
	"math/bits"

	"github.com/sergev/dejapdf/internal/mq"
)

// GRTemplate is the refinement template: 0 has 13 pixels, 1 has 10
// (6.3.5.3, Figures 12 and 13).
const GRTemplate = 0

// refTemplates lists, for each GRTEMPLATE, the pixels (dx, dy) taken from
// the bitmap being coded and from the reference, AT pixels nominal.
var refTemplates = [2]struct{ own, ref [][2]int }{
	{
		own: [][2]int{{-1, -1}, {0, -1}, {1, -1}, {-1, 0}},
		ref: [][2]int{{-1, -1}, {0, -1}, {1, -1}, {-1, 0}, {0, 0}, {1, 0}, {-1, 1}, {0, 1}, {1, 1}},
	},
	{
		own: [][2]int{{-1, -1}, {0, -1}, {1, -1}, {-1, 0}},
		ref: [][2]int{{0, -1}, {-1, 0}, {0, 0}, {1, 0}, {0, 1}, {1, 1}},
	},
}

// refiner codes bitmaps by refinement (6.3) with template 0, no
// typical prediction; its contexts persist across bitmaps.
type refiner struct {
	cx       [1 << 13]mq.Context
	own, ref []byte // unpacked bitmaps, reused
}

// code codes img against ref, whose pixel (x−dx, y−dy) lies over img's
// (x, y).
func (g *refiner) code(e *mq.Encoder, img, ref *Bitmap, dx, dy int) {
	w, h := img.W, img.H
	n := w + 2 // columns −1 to w
	// img's rows −1 to h−1, and ref's rows over img's −1 to h, one byte
	// per pixel, white outside.
	g.own = grow(g.own, (h+1)*n)
	for y := range h {
		unpack(g.own[(y+1)*n+1:(y+1)*n+1+w], img.Row(y))
	}
	g.ref = grow(g.ref, (h+2)*n)
	lo, hi := max(-1, dx), min(w, dx+ref.W-1) // columns over ref
	for y := -1; y <= h; y++ {
		src := ref.Row(y - dy)
		if src == nil || lo > hi {
			continue
		}
		dst := g.ref[(y+1)*n:]
		for x := lo; x <= hi; x++ {
			c := x - dx
			dst[x+1] = src[c>>3] >> (7 - c&7) & 1
		}
	}
	for y := range h {
		up, cur := g.own[y*n:(y+1)*n], g.own[(y+1)*n+1:(y+1)*n+1+w]
		m0, m1, m2 := g.ref[y*n:(y+1)*n], g.ref[(y+1)*n:(y+2)*n], g.ref[(y+2)*n:(y+3)*n]
		// Windows of 3 pixels, columns x−1 to x+1.
		t := int(up[0])<<1 | int(up[1])
		a := int(m0[0])<<1 | int(m0[1])
		b := int(m1[0])<<1 | int(m1[1])
		c := int(m2[0])<<1 | int(m2[1])
		left := 0
		for x, bit := range cur {
			t = (t<<1)&7 | int(up[x+2])
			a = (a<<1)&7 | int(m0[x+2])
			b = (b<<1)&7 | int(m1[x+2])
			c = (c<<1)&7 | int(m2[x+2])
			if cx := &g.cx[t<<10|left<<9|a<<6|b<<3|c]; !e.TryEncode(cx, int(bit)) {
				e.Encode(cx, int(bit))
			}
			left = int(bit)
		}
	}
}

// Ref says a symbol refines symbol ID (an input symbol, or an earlier
// new one numbered after the inputs), with the reference's pixel
// (x−DX, y−DY) over the symbol's (x, y).
type Ref struct {
	ID, DX, DY int
}

// RefineDict returns the data of a symbol dictionary segment whose
// symbols are each a refinement of one earlier symbol (6.5.8.2 with
// REFAGGNINST = 1). inputs are the symbols it imports, which it does
// not export; syms are coded in the order given.
func RefineDict(syms []*Bitmap, refs []Ref, inputs []*Bitmap) []byte {
	all := append(append([]*Bitmap(nil), inputs...), syms...)
	flags := 1<<1 | GRTemplate<<12 // SDREFAGG, SDRTEMPLATE
	d := binary.BigEndian.AppendUint16(nil, uint16(flags))
	d = append(d, 3, 0xff, 0xfd, 0xff, 2, 0xfe, 0xfe, 0xfe)
	if GRTemplate == 0 {
		d = append(d, 0xff, 0xff, 0xff, 0xff) // RA1, RA2 at (−1, −1)
	}
	d = binary.BigEndian.AppendUint32(d, uint32(len(syms)))
	d = binary.BigEndian.AppendUint32(d, uint32(len(syms)))
	e := mq.New()
	var dh, dw, ai, rdx, rdy, ex intCoder
	id := newIDCoder(bits.Len(uint(max(len(all), 1) - 1)))
	var g refiner
	h, w := 0, 0
	for k, s := range syms {
		if k == 0 || s.H != h {
			if k > 0 {
				dw.oob(e)
			}
			dh.encode(e, s.H-h)
			h, w = s.H, 0
		}
		dw.encode(e, s.W-w)
		w = s.W
		r := refs[k]
		ai.encode(e, 1)
		id.encode(e, r.ID)
		rdx.encode(e, r.DX)
		rdy.encode(e, r.DY)
		g.code(e, s, all[r.ID], r.DX, r.DY)
	}
	if len(syms) > 0 {
		dw.oob(e)
	}
	ex.encode(e, len(inputs))
	ex.encode(e, len(syms))
	return append(d, e.FlushJBIG2()...)
}
