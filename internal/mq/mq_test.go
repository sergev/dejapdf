// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package mq

import (
	"math/rand"
	"testing"
)

// decoder is the MQ decoder of T.800 C.3, for round trips.
type decoder struct {
	data []byte
	bp   int
	a, c uint32
	ct   int
}

func newDecoder(data []byte) *decoder {
	d := &decoder{data: data}
	d.c = uint32(d.byte(0)) << 16
	d.byteIn()
	d.c <<= 7
	d.ct -= 7
	d.a = 0x8000
	return d
}

func (d *decoder) byte(i int) byte {
	if i < len(d.data) {
		return d.data[i]
	}
	return 0xff
}

func (d *decoder) byteIn() {
	if d.byte(d.bp) == 0xff {
		if d.byte(d.bp+1) > 0x8f {
			d.c += 0xff00
			d.ct = 8
		} else {
			d.bp++
			d.c += uint32(d.byte(d.bp)) << 9
			d.ct = 7
		}
	} else {
		d.bp++
		d.c += uint32(d.byte(d.bp)) << 8
		d.ct = 8
	}
}

func (d *decoder) decode(cx *Context) int {
	s := &states[*cx]
	d.a -= s.qe
	var bit int
	if d.c>>16 < s.qe {
		// LPS exchange
		if d.a < s.qe {
			bit = int(*cx & 1)
			*cx = s.nmps
		} else {
			bit = int(*cx&1) ^ 1
			*cx = s.nlps
		}
		d.a = s.qe
	} else {
		d.c -= s.qe << 16
		if d.a&0x8000 != 0 {
			return int(*cx & 1)
		}
		if d.a < s.qe {
			bit = int(*cx&1) ^ 1
			*cx = s.nlps
		} else {
			bit = int(*cx & 1)
			*cx = s.nmps
		}
	}
	for d.a&0x8000 == 0 {
		if d.ct == 0 {
			d.byteIn()
		}
		d.a <<= 1
		d.c <<= 1
		d.ct--
	}
	return bit
}

func TestRoundTrip(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	e := New()
	for _, n := range []int{0, 1, 7, 100, 10000} {
		for _, p := range []float64{0.5, 0.9, 0.99} {
			bits := make([]int, n)
			cxs := make([]int, n)
			for i := range bits {
				cxs[i] = r.Intn(4)
				if r.Float64() > p+float64(cxs[i])*0.002 {
					bits[i] = 1
				}
			}
			for jbig := range 2 {
				e.Reset()
				var ec [4]Context
				for i, b := range bits {
					e.Encode(&ec[cxs[i]], b)
				}
				var out []byte
				if jbig == 1 {
					out = e.FlushJBIG2()
				} else {
					out = e.FlushJPX()
				}
				d := newDecoder(append([]byte(nil), out...))
				var dc [4]Context
				for i, b := range bits {
					if g := d.decode(&dc[cxs[i]]); g != b {
						t.Fatalf("n=%d p=%v jbig=%d: bit %d = %d, want %d", n, p, jbig, i, g, b)
					}
				}
			}
		}
	}
}

// TestTruncation checks that a prefix of NumBytes()+3 bytes taken at any
// point decodes the symbols coded up to that point.
func TestTruncation(t *testing.T) {
	r := rand.New(rand.NewSource(2))
	const n = 5000
	bits := make([]int, n)
	for i := range bits {
		if r.Intn(10) == 0 {
			bits[i] = 1
		}
	}
	e := New()
	var ec Context
	marks := map[int]int{}
	for i, b := range bits {
		e.Encode(&ec, b)
		if i%97 == 0 {
			marks[i+1] = e.NumBytes() + 3
		}
	}
	out := e.FlushJPX()
	for k, l := range marks {
		l = min(l, len(out))
		if l > 0 && out[l-1] == 0xff {
			l--
		}
		d := newDecoder(out[:l])
		var dc Context
		for i := range k {
			if g := d.decode(&dc); g != bits[i] {
				t.Fatalf("prefix %d bytes for %d symbols: bit %d wrong", l, k, i)
			}
		}
	}
}

// TestAnnexH2 is the test sequence of T.88 H.2: one context, 256
// decisions, MSB first.
func TestAnnexH2(t *testing.T) {
	in := []byte{
		0x00, 0x02, 0x00, 0x51, 0x00, 0x00, 0x00, 0xC0, 0x03, 0x52, 0x87, 0x2A, 0xAA, 0xAA, 0xAA, 0xAA,
		0x82, 0xC0, 0x20, 0x00, 0xFC, 0xD7, 0x9E, 0xF6, 0xBF, 0x7F, 0xED, 0x90, 0x4F, 0x46, 0xA3, 0xBF,
	}
	want := []byte{
		0x84, 0xC7, 0x3B, 0xFC, 0xE1, 0xA1, 0x43, 0x04, 0x02, 0x20, 0x00, 0x00, 0x41, 0x0D, 0xBB, 0x86,
		0xF4, 0x31, 0x7F, 0xFF, 0x88, 0xFF, 0x37, 0x47, 0x1A, 0xDB, 0x6A, 0xDF, 0xFF, 0xAC,
	}
	e := New()
	var cx Context
	for _, b := range in {
		for i := 7; i >= 0; i-- {
			e.Encode(&cx, int(b>>i&1))
		}
	}
	if got := e.FlushJBIG2(); string(got) != string(want) {
		t.Errorf("got  % X\nwant % X", got, want)
	}
}
