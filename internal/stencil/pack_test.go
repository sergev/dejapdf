// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package stencil

import (
	"math/rand"
	"testing"

	"github.com/sergev/dejapdf/internal/jb2"
)

func TestPack(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for range 200 {
		w, h := 1+r.Intn(40), 1+r.Intn(5)
		b := &jb2.Bitmap{W: w, H: h, Pix: make([]byte, w*h)}
		for i := range b.Pix {
			b.Pix[i] = byte(r.Intn(2))
		}
		m := pack(b)
		for y := range h {
			for x := range w {
				got := m.Pix[y*m.Stride+x>>3] >> (7 - x&7) & 1
				if got != b.Pix[y*w+x] {
					t.Fatalf("%dx%d: pixel (%d, %d) = %d", w, h, x, y, got)
				}
			}
			if pad := m.Pix[y*m.Stride+m.Stride-1] & (0xff >> uint(w&7)); w&7 != 0 && pad != 0 {
				t.Fatalf("%dx%d: padding bits set", w, h)
			}
		}
	}
}
