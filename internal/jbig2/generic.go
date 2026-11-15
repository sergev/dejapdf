// Copyright 2026 肖其顿 (XIAO QI DUN)
// Copyright 2026 Serge Vakulenko
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Modified for dejapdf: taken from jbig2_grd_encoder.go; uses
// internal/mq and Bitmap, no limits or cancellation; segment data added.

package jbig2

import (
	"bytes"

	"github.com/sergev/dejapdf/internal/mq"
)

// GenericRegion returns the data of an immediate generic region segment
// (7.4.6) holding b at (x, y): template 0, nominal AT pixels, TPGDON.
func GenericRegion(b *Bitmap, x, y int) []byte {
	d := regionInfo(nil, b.W, b.H, x, y)
	d = append(d, 0<<1|1<<3) // MMR 0, GBTEMPLATE 0, TPGDON
	d = append(d, 3, 0xff, 0xfd, 0xff, 2, 0xfe, 0xfe, 0xfe)
	return append(d, encodeGeneric(b)...)
}

// sltpContext is the TPGDON context of template 0 (6.2.5.5, Figure 8).
const sltpContext = 0x9b25

// generic codes bitmaps with template 0 and nominal AT pixels (6.2.5).
// Its contexts persist across bitmaps, as in a symbol dictionary. The
// context holds, from bit 0: the 4 pixels left in this row, 7 of the row
// above (x+3 down to x-3) and 5 of the row above that (x+2 down to x-2),
// the spec's order with the AT pixels at their nominal places.
type generic struct {
	cx  [1 << 16]mq.Context
	buf []byte // unpacked rows, reused
}

// encodeGeneric codes a whole generic region with TPGDON.
func encodeGeneric(img *Bitmap) []byte {
	var g generic
	e := mq.New()
	g.code(e, img, true)
	return append([]byte(nil), e.FlushJBIG2()...)
}

// gpad is the white pixels on each side of an unpacked row.
const gpad = 4

// code codes img, with typical prediction if tpgdon.
func (g *generic) code(e *mq.Encoder, img *Bitmap, tpgdon bool) {
	cx := &g.cx
	w := img.W
	n := w + 2*gpad
	g.buf = grow(g.buf, 3*n)
	// Rows y−2, y−1 and y, one byte per pixel.
	older, prev, cur := g.buf[:n], g.buf[n:2*n], g.buf[2*n:3*n]
	ltp := 0
	for y := range img.H {
		row := img.Row(y)
		unpack(cur[gpad:gpad+w], row)
		if tpgdon {
			typical := 0
			if equalRows(row, img.Row(y-1), w) {
				typical = 1
			}
			e.Encode(&cx[sltpContext], typical^ltp)
			ltp = typical
			if ltp != 0 {
				older, prev, cur = prev, cur, older
				continue
			}
		}
		p, o, r := prev[gpad:gpad+w+4], older[gpad:gpad+w+3], cur[gpad:gpad+w]
		c := uint32(p[3])<<4 | uint32(p[2])<<5 | uint32(p[1])<<6 | uint32(p[0])<<7 |
			(uint32(o[2])|uint32(o[1])<<1|uint32(o[0])<<2)<<11
		for x, bit := range r {
			if !e.TryEncode(&cx[c], int(bit)) {
				e.Encode(&cx[c], int(bit))
			}
			c = (c<<1)&0xf7ee | uint32(bit) | uint32(p[x+4])<<4 | uint32(o[x+3])<<11
		}
		older, prev, cur = prev, cur, older
	}
}

// grow returns b resized to n zero bytes, reusing its storage.
func grow(b []byte, n int) []byte {
	if cap(b) < n {
		return make([]byte, n)
	}
	b = b[:n]
	clear(b)
	return b
}

// unpackTab spreads a byte's bits, MSB first, one per byte.
var unpackTab = func() (t [256][8]byte) {
	for v := range t {
		for i := range 8 {
			t[v][i] = byte(v >> (7 - i) & 1)
		}
	}
	return
}()

// unpack sets dst[i] to pixel i of the packed row src.
func unpack(dst, src []byte) {
	k := 0
	for ; k+8 <= len(dst); k += 8 {
		*(*[8]byte)(dst[k:]) = unpackTab[src[k>>3]]
	}
	for ; k < len(dst); k++ {
		dst[k] = src[k>>3] >> (7 - k&7) & 1
	}
}

// equalRows compares two rows' pixels; a missing previous row is white.
func equalRows(row, prev []byte, w int) bool {
	full := w >> 3
	tail := byte(0xff << (8 - uint(w&7)))
	if prev == nil {
		for _, v := range row[:full] {
			if v != 0 {
				return false
			}
		}
		return w&7 == 0 || row[full]&tail == 0
	}
	return bytes.Equal(row[:full], prev[:full]) &&
		(w&7 == 0 || (row[full]^prev[full])&tail == 0)
}
