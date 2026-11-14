// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package jbig2

// Bitmap is a bilevel image, rows top to bottom, packed MSB first,
// 1 = black.
type Bitmap struct {
	W, H, Stride int
	Pix          []byte
}

// NewBitmap returns a white w×h bitmap.
func NewBitmap(w, h int) *Bitmap {
	s := (w + 7) / 8
	return &Bitmap{W: w, H: h, Stride: s, Pix: make([]byte, s*h)}
}

// Row returns row y, or nil outside the bitmap.
func (b *Bitmap) Row(y int) []byte {
	if y < 0 || y >= b.H {
		return nil
	}
	return b.Pix[y*b.Stride : (y+1)*b.Stride]
}

// Set makes pixel (x, y) black.
func (b *Bitmap) Set(x, y int) { b.Pix[y*b.Stride+x>>3] |= 0x80 >> (x & 7) }
