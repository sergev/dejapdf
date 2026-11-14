// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package djvu

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/sergev/dejapdf/internal/ccitt"
	"github.com/sergev/dejapdf/internal/jb2"
)

// maxMMRPixels limits a decoded Smmr mask, like the JB2 page limit.
const maxMMRPixels = 1 << 27

// DecodeMMR decodes an Smmr chunk (spec 8.3.15): "MMR", flags (bit 0
// reverse video, bit 1 striped), width and height, then one G4 stream,
// or rows per stripe and, for each stripe from the top, a 4-byte length
// and an independent G4 stream. The result has 1 for black.
func DecodeMMR(b []byte) (*jb2.Bitmap, error) {
	if len(b) < 8 || string(b[:3]) != "MMR" {
		return nil, errors.New("djvu: Smmr: bad header")
	}
	invert, striped := b[3]&1 != 0, b[3]&2 != 0
	w, h := int(b[4])<<8|int(b[5]), int(b[6])<<8|int(b[7])
	if w == 0 || h == 0 || w*h > maxMMRPixels {
		return nil, fmt.Errorf("djvu: Smmr: size %dx%d", w, h)
	}
	b = b[8:]
	bm := &jb2.Bitmap{W: w, H: h, Pix: make([]byte, w*h)}
	rps := h
	if striped {
		if len(b) < 2 {
			return nil, errors.New("djvu: Smmr: no rows per stripe")
		}
		rps = int(b[0])<<8 | int(b[1])
		b = b[2:]
		if rps == 0 {
			return nil, errors.New("djvu: Smmr: zero rows per stripe")
		}
	}
	row := make([]byte, (w+7)/8)
	for y0 := 0; y0 < h; y0 += rps {
		data := b
		if striped {
			if len(b) < 4 {
				return nil, errors.New("djvu: Smmr: stripe truncated")
			}
			n := int(b[0])<<24 | int(b[1])<<16 | int(b[2])<<8 | int(b[3])
			if n < 0 || n > len(b)-4 {
				return nil, errors.New("djvu: Smmr: stripe truncated")
			}
			data, b = b[4:4+n], b[4+n:]
		}
		rows := min(rps, h-y0)
		r := ccitt.NewReader(bytes.NewReader(data), ccitt.MSB, ccitt.Group4, w, rows, nil)
		for y := y0; y < y0+rows; y++ {
			if _, err := io.ReadFull(r, row); err != nil {
				return nil, fmt.Errorf("djvu: Smmr: row %d: %w", y, err)
			}
			px := bm.Pix[y*w : (y+1)*w]
			for x := range px {
				// The decoder gives 1 for white.
				white := row[x>>3]>>(7-x&7)&1 != 0
				if white == invert {
					px[x] = 1
				}
			}
		}
	}
	return bm, nil
}

// mmrMask wraps an Smmr bitmap as a mask image of one blit.
func mmrMask(data []byte) (*jb2.Image, error) {
	bm, err := DecodeMMR(data)
	if err != nil {
		return nil, err
	}
	return &jb2.Image{W: bm.W, H: bm.H, Shapes: []*jb2.Bitmap{bm},
		Refs: []jb2.Ref{{Shape: -1}}, Blits: []jb2.Blit{{}}}, nil
}
