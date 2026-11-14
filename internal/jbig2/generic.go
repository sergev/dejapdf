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
	cx [1 << 16]mq.Context
}

// encodeGeneric codes a whole generic region with TPGDON.
func encodeGeneric(img *Bitmap) []byte {
	var g generic
	e := mq.New()
	g.code(e, img, true)
	return append([]byte(nil), e.FlushJBIG2()...)
}

// code codes img, with typical prediction if tpgdon.
func (g *generic) code(e *mq.Encoder, img *Bitmap, tpgdon bool) {
	cx := &g.cx
	ltp := 0
	for y := range img.H {
		row, prev := img.Row(y), img.Row(y-1)
		if tpgdon {
			typical := 0
			if equalRows(row, prev, img.W) {
				typical = 1
			}
			e.Encode(&cx[sltpContext], typical^ltp)
			ltp = typical
			if ltp != 0 {
				continue
			}
		}
		older := img.Row(y - 2)
		c := pixel(prev, 3, img.W)<<4 | pixel(prev, 2, img.W)<<5 | pixel(prev, 1, img.W)<<6 |
			pixel(prev, 0, img.W)<<7 |
			(pixel(older, 2, img.W)|pixel(older, 1, img.W)<<1|pixel(older, 0, img.W)<<2)<<11
		for x := range img.W {
			bit := int(row[x>>3] >> (7 - x&7) & 1)
			e.Encode(&cx[c], bit)
			c = (c<<1)&0xf7ee | uint32(bit) |
				pixel(prev, x+4, img.W)<<4 | pixel(older, x+3, img.W)<<11
		}
	}
}

// pixel returns pixel x of a packed row; 0 outside it.
func pixel(row []byte, x, w int) uint32 {
	if row == nil || x < 0 || x >= w {
		return 0
	}
	return uint32(row[x>>3] >> (7 - x&7) & 1)
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
