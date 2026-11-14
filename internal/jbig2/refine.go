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

// refiner codes bitmaps by refinement (6.3), no typical prediction; its
// contexts persist across bitmaps.
type refiner struct {
	cx [1 << 13]mq.Context
}

// code codes img against ref, whose pixel (x−dx, y−dy) lies over img's
// (x, y).
func (g *refiner) code(e *mq.Encoder, img, ref *Bitmap, dx, dy int) {
	t := &refTemplates[GRTemplate]
	for y := range img.H {
		row := img.Row(y)
		for x := range img.W {
			c := 0
			for _, p := range t.own {
				c = c<<1 | int(pixel(img.Row(y+p[1]), x+p[0], img.W))
			}
			for _, p := range t.ref {
				c = c<<1 | int(pixel(ref.Row(y-dy+p[1]), x-dx+p[0], ref.W))
			}
			e.Encode(&g.cx[c], int(row[x>>3]>>(7-x&7)&1))
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
