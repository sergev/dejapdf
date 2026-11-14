// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package jbig2

import (
	"encoding/binary"
	"math/bits"
	"sort"

	"github.com/sergev/dejapdf/internal/mq"
)

// SymbolDict returns the data of a symbol dictionary segment (7.4.2)
// defining and exporting syms, none of them empty, and the number each
// symbol gets: symbols are coded by height class, then by width (6.5).
// Arithmetic coding, template 0, no refinement, no imports.
func SymbolDict(syms []*Bitmap) (data []byte, ids []int) {
	order := make([]int, len(syms))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		sa, sb := syms[order[a]], syms[order[b]]
		if sa.H != sb.H {
			return sa.H < sb.H
		}
		return sa.W < sb.W
	})
	ids = make([]int, len(syms))
	for k, i := range order {
		ids[i] = k
	}
	d := []byte{0, 0} // SDHUFF 0, SDREFAGG 0, SDTEMPLATE 0
	d = append(d, 3, 0xff, 0xfd, 0xff, 2, 0xfe, 0xfe, 0xfe)
	d = binary.BigEndian.AppendUint32(d, uint32(len(syms)))
	d = binary.BigEndian.AppendUint32(d, uint32(len(syms)))
	e := mq.New()
	var dh, dw, ex intCoder
	var g generic
	h, w := 0, 0
	for k, i := range order {
		s := syms[i]
		if k == 0 || s.H != h {
			if k > 0 {
				dw.oob(e) // end of the previous height class
			}
			dh.encode(e, s.H-h)
			h, w = s.H, 0
		}
		dw.encode(e, s.W-w)
		w = s.W
		g.code(e, s, false)
	}
	if len(order) > 0 {
		dw.oob(e)
	}
	// Export flags: none of the 0 inputs, then all new symbols.
	ex.encode(e, 0)
	ex.encode(e, len(syms))
	return append(d, e.FlushJBIG2()...), ids
}

// SDTemplate is the generic template of symbol bitmaps.
var SDTemplate = 3

// Instance places symbol ID with its bottom-left pixel at column S, row T
// (rows from the top); W is the symbol's width.
type Instance struct {
	ID, S, T, W int
}

// TextRegion returns the data of an immediate text region segment
// (7.4.3) of w×h at (0, 0) drawing the instances with OR (6.4).
// nsyms is the number of symbols available; strips is 1, 2, 4 or 8.
func TextRegion(w, h int, ins []Instance, nsyms, strips, dsOffset int) []byte {
	ins = append([]Instance(nil), ins...)
	strip := func(t int) int { return floorDiv(t, strips) * strips }
	sort.SliceStable(ins, func(a, b int) bool {
		sa, sb := strip(ins[a].T), strip(ins[b].T)
		if sa != sb {
			return sa < sb
		}
		return ins[a].S < ins[b].S
	})
	d := regionInfo(nil, w, h, 0, 0)
	flags := bits.TrailingZeros(uint(strips))<<2 | (dsOffset&0x1f)<<10
	d = binary.BigEndian.AppendUint16(d, uint16(flags))
	d = binary.BigEndian.AppendUint32(d, uint32(len(ins)))
	e := mq.New()
	var dt, fs, ds, it intCoder
	id := newIDCoder(bits.Len(uint(max(nsyms, 1) - 1)))
	dt.encode(e, 0) // STRIPT = 0
	stript, firsts := 0, 0
	for i := 0; i < len(ins); {
		st := strip(ins[i].T)
		dt.encode(e, (st-stript)/strips)
		stript = st
		curs := 0
		for first := true; i < len(ins) && strip(ins[i].T) == st; i++ {
			in := ins[i]
			if first {
				fs.encode(e, in.S-firsts)
				firsts, first = in.S, false
			} else {
				ds.encode(e, in.S-curs-dsOffset)
			}
			if strips > 1 {
				it.encode(e, in.T-st)
			}
			id.encode(e, in.ID)
			curs = in.S + in.W - 1
		}
		ds.oob(e)
	}
	return append(d, e.FlushJBIG2()...)
}

func floorDiv(a, b int) int {
	q := a / b
	if a%b != 0 && a < 0 {
		q--
	}
	return q
}
