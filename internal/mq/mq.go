// Copyright 2026 肖其顿 (XIAO QI DUN)
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
// Modified for dejapdf: taken from jbig2_arith_encoder.go and
// jbig2_arith_decoder.go; exported API, no output limit, JPEG 2000 flush
// and byte count added.

// Package mq implements the MQ arithmetic encoder of ITU-T T.88 Annex E
// and T.800 Annex C, shared by JBIG2 and JPEG 2000.
package mq

// Context is a probability state: Qe table index << 1 | MPS.
type Context uint8

// NewContext returns a context starting at Qe table index i, MPS 0.
func NewContext(i int) Context { return Context(i << 1) }

type qe struct {
	qe         uint16
	nmps, nlps uint8
	sw         bool
}

// Table E.1 of T.88 (C.2 of T.800).
var qeTable = [47]qe{
	{0x5601, 1, 1, true}, {0x3401, 2, 6, false}, {0x1801, 3, 9, false},
	{0x0AC1, 4, 12, false}, {0x0521, 5, 29, false}, {0x0221, 38, 33, false},
	{0x5601, 7, 6, true}, {0x5401, 8, 14, false}, {0x4801, 9, 14, false},
	{0x3801, 10, 14, false}, {0x3001, 11, 17, false}, {0x2401, 12, 18, false},
	{0x1C01, 13, 20, false}, {0x1601, 29, 21, false}, {0x5601, 15, 14, true},
	{0x5401, 16, 14, false}, {0x5101, 17, 15, false}, {0x4801, 18, 16, false},
	{0x3801, 19, 17, false}, {0x3401, 20, 18, false}, {0x3001, 21, 19, false},
	{0x2801, 22, 19, false}, {0x2401, 23, 20, false}, {0x2201, 24, 21, false},
	{0x1C01, 25, 22, false}, {0x1801, 26, 23, false}, {0x1601, 27, 24, false},
	{0x1401, 28, 25, false}, {0x1201, 29, 26, false}, {0x1101, 30, 27, false},
	{0x0AC1, 31, 28, false}, {0x09C1, 32, 29, false}, {0x08A1, 33, 30, false},
	{0x0521, 34, 31, false}, {0x0441, 35, 32, false}, {0x02A1, 36, 33, false},
	{0x0221, 37, 34, false}, {0x0141, 38, 35, false}, {0x0111, 39, 36, false},
	{0x0085, 40, 37, false}, {0x0049, 41, 38, false}, {0x0025, 42, 39, false},
	{0x0015, 43, 40, false}, {0x0009, 44, 41, false}, {0x0005, 45, 42, false},
	{0x0001, 45, 43, false}, {0x5601, 46, 46, false},
}

type state struct {
	qe         uint32
	nmps, nlps Context
}

var states = func() (t [94]state) {
	for s := range t {
		q := qeTable[s>>1]
		mps := Context(s & 1)
		t[s] = state{uint32(q.qe), Context(q.nmps)<<1 | mps, Context(q.nlps)<<1 | mps}
		if q.sw {
			t[s].nlps ^= 1
		}
	}
	return
}()

// Encoder is an MQ encoder. The zero value is not ready; use New or Reset.
type Encoder struct {
	a, c    uint32
	ct      uint
	b       byte
	started bool
	data    []byte
}

// New returns an encoder.
func New() *Encoder {
	e := &Encoder{}
	e.Reset()
	return e
}

// Reset starts a new stream, reusing the buffer.
func (e *Encoder) Reset() {
	*e = Encoder{a: 0x8000, ct: 12, data: e.data[:0]}
}

// Encode codes one decision in context cx.
func (e *Encoder) Encode(cx *Context, bit int) {
	s := &states[*cx]
	e.a -= s.qe
	if Context(bit) == *cx&1 {
		if e.a&0x8000 != 0 {
			e.c += s.qe
			return
		}
		if e.a < s.qe {
			e.a = s.qe
		} else {
			e.c += s.qe
		}
		*cx = s.nmps
	} else {
		if e.a < s.qe {
			e.c += s.qe
		} else {
			e.a = s.qe
		}
		*cx = s.nlps
	}
	for e.a&0x8000 == 0 {
		e.a <<= 1
		e.c <<= 1
		e.ct--
		if e.ct == 0 {
			e.byteOut()
		}
	}
}

func (e *Encoder) byteOut() {
	if e.b != 0xff && e.c&0x08000000 != 0 {
		e.b++
		e.c &= 0x07ffffff
	}
	if e.started {
		e.data = append(e.data, e.b)
	}
	e.started = true
	if e.b == 0xff {
		e.b = byte(e.c >> 20)
		e.c &= 0x000fffff
		e.ct = 7
	} else {
		e.b = byte(e.c >> 19)
		e.c &= 0x0007ffff
		e.ct = 8
	}
}

// NumBytes is the number of bytes output so far, without the one pending.
func (e *Encoder) NumBytes() int { return len(e.data) }

func (e *Encoder) flush() {
	end := e.c + e.a
	e.c |= 0xffff
	if e.c >= end {
		e.c -= 0x8000
	}
	e.c <<= e.ct
	e.byteOut()
	e.c <<= e.ct
	e.byteOut()
}

// FlushJPX terminates a JPEG 2000 stream (T.800 C.2.9): a final 0xFF is
// dropped. The result aliases the encoder's buffer.
func (e *Encoder) FlushJPX() []byte {
	e.flush()
	if e.b != 0xff {
		e.data = append(e.data, e.b)
	}
	return e.data
}

// FlushJBIG2 terminates a JBIG2 stream with the FF AC marker (T.88 E.2.9).
func (e *Encoder) FlushJBIG2() []byte {
	e.flush()
	e.data = append(e.data, e.b)
	if e.b != 0xff {
		e.data = append(e.data, 0xff)
	}
	return append(e.data, 0xac)
}
