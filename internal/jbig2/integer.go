// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package jbig2

import "github.com/sergev/dejapdf/internal/mq"

// intCoder codes integers with one context set (Annex A.2): IADH,
// IADW, IAEX, IADT, IAFS, IADS, IAIT.
type intCoder [512]mq.Context

// intRanges are the value ranges of Table A.1: prefix length, value bits
// and offset.
var intRanges = [...]struct{ prefix, bits, base int }{
	{1, 2, 0}, {2, 4, 4}, {3, 6, 20}, {4, 8, 84}, {5, 12, 340}, {5, 32, 4436},
}

// encode codes v.
func (c *intCoder) encode(e *mq.Encoder, v int) {
	s, m := 0, v
	if v < 0 {
		s, m = 1, -v
	}
	r := len(intRanges) - 1
	for i, ir := range intRanges[:len(intRanges)-1] {
		if m < ir.base+1<<ir.bits {
			r = i
			break
		}
	}
	prev := 1
	put := func(bit int) {
		e.Encode(&c[prev], bit)
		if prev < 256 {
			prev = prev<<1 | bit
		} else {
			prev = (prev<<1|bit)&511 | 256
		}
	}
	put(s)
	ir := intRanges[r]
	for i := range ir.prefix {
		// r ones then a zero; the last range has five ones only.
		if i < r {
			put(1)
		} else {
			put(0)
		}
	}
	m -= ir.base
	for i := ir.bits - 1; i >= 0; i-- {
		put(m >> i & 1)
	}
}

// oob codes the out-of-band value: −0 in the first range.
func (c *intCoder) oob(e *mq.Encoder) {
	prev := 1
	for _, bit := range []int{1, 0, 0, 0} {
		e.Encode(&c[prev], bit)
		prev = prev<<1 | bit
	}
}

// idCoder codes symbol IDs in a fixed number of bits (A.3).
type idCoder struct {
	n  int
	cx []mq.Context
}

func newIDCoder(n int) *idCoder { return &idCoder{n, make([]mq.Context, 1<<n)} }

func (c *idCoder) encode(e *mq.Encoder, v int) {
	prev := 1
	for i := c.n - 1; i >= 0; i-- {
		bit := v >> i & 1
		e.Encode(&c.cx[prev], bit)
		prev = prev<<1 | bit
	}
}
