// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package jbig2

import (
	"bytes"
	"math/rand"
	"testing"

	"github.com/sergev/dejapdf/internal/mq"
)

// pixel returns pixel x of a packed row; 0 outside it.
func pixel(row []byte, x, w int) uint32 {
	if row == nil || x < 0 || x >= w {
		return 0
	}
	return uint32(row[x>>3] >> (7 - x&7) & 1)
}

// slowGeneric is generic.code pixel by pixel, the reference.
func slowGeneric(cx *[1 << 16]mq.Context, e *mq.Encoder, img *Bitmap, tpgdon bool) {
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

// slowRefine is refiner.code from the template table, the reference.
func slowRefine(cx *[1 << 13]mq.Context, e *mq.Encoder, img, ref *Bitmap, dx, dy, tmpl int) {
	t := &refTemplates[tmpl]
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
			e.Encode(&cx[c], int(row[x>>3]>>(7-x&7)&1))
		}
	}
}

// dirty sets the padding bits past each row's end, which coders must ignore.
func dirty(b *Bitmap) *Bitmap {
	if b.W&7 != 0 {
		for y := range b.H {
			b.Pix[y*b.Stride+b.Stride-1] |= 0xff >> (b.W & 7)
		}
	}
	return b
}

func TestGenericSame(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	var g generic
	var cx [1 << 16]mq.Context
	fast, slow := mq.New(), mq.New()
	for k := range 300 {
		b := testBitmap(1+r.Intn(70), 1+r.Intn(40), r.Int63())
		if k%3 == 0 {
			// Repeated rows for typical prediction.
			for y := 1; y < b.H; y += 2 {
				copy(b.Row(y), b.Row(y-1))
			}
		}
		dirty(b)
		tp := k%2 == 0
		g.code(fast, b, tp)
		slowGeneric(&cx, slow, b, tp)
	}
	if !bytes.Equal(fast.FlushJBIG2(), slow.FlushJBIG2()) {
		t.Error("generic coder differs from the reference")
	}
}

func TestRefineSame(t *testing.T) {
	r := rand.New(rand.NewSource(2))
	var g refiner
	var cx [1 << 13]mq.Context
	fast, slow := mq.New(), mq.New()
	for range 300 {
		img := dirty(testBitmap(1+r.Intn(50), 1+r.Intn(50), r.Int63()))
		ref := dirty(testBitmap(1+r.Intn(50), 1+r.Intn(50), r.Int63()))
		dx, dy := r.Intn(81)-40, r.Intn(81)-40
		if r.Intn(2) == 0 {
			dx, dy = r.Intn(5)-2, r.Intn(5)-2
		}
		g.code(fast, img, ref, dx, dy)
		slowRefine(&cx, slow, img, ref, dx, dy, GRTemplate)
	}
	if !bytes.Equal(fast.FlushJBIG2(), slow.FlushJBIG2()) {
		t.Error("refinement coder differs from the reference")
	}
}

func benchSyms(n int) []*Bitmap {
	r := rand.New(rand.NewSource(3))
	syms := make([]*Bitmap, n)
	for i := range syms {
		syms[i] = testBitmap(8+r.Intn(30), 10+r.Intn(30), r.Int63())
	}
	return syms
}

func BenchmarkSymbolDict(b *testing.B) {
	syms := benchSyms(500)
	for b.Loop() {
		SymbolDict(syms)
	}
}

func BenchmarkRefineDict(b *testing.B) {
	inputs := benchSyms(500)
	r := rand.New(rand.NewSource(4))
	syms := make([]*Bitmap, len(inputs))
	refs := make([]Ref, len(inputs))
	for i, s := range inputs {
		c := NewBitmap(s.W, s.H)
		copy(c.Pix, s.Pix)
		for range 10 {
			c.Set(r.Intn(s.W), r.Intn(s.H))
		}
		syms[i], refs[i] = c, Ref{ID: i}
	}
	for b.Loop() {
		RefineDict(syms, refs, inputs)
	}
}

func BenchmarkGenericPage(b *testing.B) {
	img := testBitmap(2500, 3500, 5)
	for b.Loop() {
		GenericRegion(img, 0, 0)
	}
}
