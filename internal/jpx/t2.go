// Copyright 2024 Joshua Kolden
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
// Modified for dejapdf: pktWriter, tagTreeEnc and writeNumPasses taken from
// go-jpeg2000's t2_encode.go, with reset added; packet assembly rewritten
// for one layer.

package jpx

import "math/bits"

// pktWriter emits packet-header bits with JPEG 2000 bit stuffing: after a
// byte of 0xFF the next byte carries only seven bits (B.10.1).
type pktWriter struct {
	buf    []byte
	acc    uint32
	nbits  uint
	lastFF bool
}

func (w *pktWriter) writeBit(b uint32) {
	limit := uint(8)
	if w.lastFF {
		limit = 7
	}
	w.acc = (w.acc << 1) | (b & 1)
	w.nbits++
	if w.nbits == limit {
		v := byte(w.acc)
		w.buf = append(w.buf, v)
		w.lastFF = v == 0xFF
		w.acc = 0
		w.nbits = 0
	}
}

func (w *pktWriter) writeBits(v uint32, n uint) {
	for i := int(n) - 1; i >= 0; i-- {
		w.writeBit((v >> uint(i)) & 1)
	}
}

// flush pads to a byte boundary; a header ending on 0xFF gets a zero byte.
func (w *pktWriter) flush() {
	if w.nbits > 0 {
		limit := uint(8)
		if w.lastFF {
			limit = 7
		}
		w.acc <<= limit - w.nbits
		v := byte(w.acc)
		w.buf = append(w.buf, v)
		w.lastFF = v == 0xFF
		w.acc = 0
		w.nbits = 0
	}
	if w.lastFF {
		w.buf = append(w.buf, 0x00)
		w.lastFF = false
	}
}

// tagTreeEnc encodes values into a tag tree (B.10.2).
type tagTreeEnc struct {
	levels int
	dimW   []int
	dimH   []int
	value  [][]int
	state  [][]int  // per-node lower bound already emitted
	known  [][]bool // per-node: the value has been established
}

func newTagTreeEnc(w, h int) *tagTreeEnc {
	w, h = max(w, 1), max(h, 1)
	t := &tagTreeEnc{}
	lw, lh := w, h
	for {
		t.dimW = append(t.dimW, lw)
		t.dimH = append(t.dimH, lh)
		t.value = append(t.value, make([]int, lw*lh))
		t.state = append(t.state, make([]int, lw*lh))
		t.known = append(t.known, make([]bool, lw*lh))
		if lw == 1 && lh == 1 {
			break
		}
		lw = (lw + 1) / 2
		lh = (lh + 1) / 2
	}
	t.levels = len(t.dimW)
	return t
}

// reset forgets what has been coded, keeping the values.
func (t *tagTreeEnc) reset() {
	for l := range t.levels {
		clear(t.state[l])
		clear(t.known[l])
	}
}

// setLeaf records a leaf value and recomputes its ancestors as minima.
// All leaves must be set before coding.
func (t *tagTreeEnc) setLeaf(x, y, v int) {
	t.value[0][y*t.dimW[0]+x] = v
	for lvl := 1; lvl < t.levels; lvl++ {
		xx, yy := x>>uint(lvl), y>>uint(lvl)
		idx := yy*t.dimW[lvl] + xx
		m := -1
		pw, ph := t.dimW[lvl-1], t.dimH[lvl-1]
		for dy := range 2 {
			for dx := range 2 {
				cx, cy := xx*2+dx, yy*2+dy
				if cx >= pw || cy >= ph {
					continue
				}
				cv := t.value[lvl-1][cy*pw+cx]
				if m < 0 || cv < m {
					m = cv
				}
			}
		}
		t.value[lvl][idx] = max(m, 0)
	}
}

// encode emits the bits telling whether the leaf value is below threshold.
func (t *tagTreeEnc) encode(w *pktWriter, x, y, threshold int) {
	low := 0
	for lvl := t.levels - 1; lvl >= 0; lvl-- {
		xx, yy := x>>uint(lvl), y>>uint(lvl)
		idx := yy*t.dimW[lvl] + xx
		if t.state[lvl][idx] < low {
			t.state[lvl][idx] = low
		}
		v := t.value[lvl][idx]
		for !t.known[lvl][idx] && t.state[lvl][idx] < threshold {
			if t.state[lvl][idx] == v {
				w.writeBit(1)
				t.known[lvl][idx] = true
			} else {
				w.writeBit(0)
				t.state[lvl][idx]++
			}
		}
		low = t.state[lvl][idx]
		if !t.known[lvl][idx] {
			return
		}
	}
}

// writeNumPasses encodes the coding-pass count (Table B.4).
func writeNumPasses(w *pktWriter, n int) {
	switch {
	case n <= 1:
		w.writeBit(0)
	case n == 2:
		w.writeBits(2, 2)
	case n <= 5:
		w.writeBits(3, 2)
		w.writeBits(uint32(n-3), 2)
	case n <= 36:
		w.writeBits(15, 4)
		w.writeBits(uint32(n-6), 5)
	default:
		w.writeBits(511, 9)
		w.writeBits(uint32(n-37), 7)
	}
}

// packet writes the header of the packet holding bands bs, given each
// code-block's kept pass count, and returns the body size (B.9, B.10).
func packet(w *pktWriter, bs []*band) int {
	body := 0
	any := false
	for _, b := range bs {
		for i := range b.blocks {
			if b.blocks[i].keep > 0 {
				any = true
			}
		}
	}
	if !any {
		w.writeBit(0)
		w.flush()
		return 0
	}
	w.writeBit(1)
	for _, b := range bs {
		if len(b.blocks) == 0 {
			continue
		}
		for i := range b.blocks {
			cb := &b.blocks[i]
			incl := 1
			if cb.keep > 0 {
				incl = 0
			}
			b.incl.setLeaf(i%b.ncx, i/b.ncx, incl)
			b.zbp.setLeaf(i%b.ncx, i/b.ncx, cb.zbp)
		}
		b.incl.reset()
		b.zbp.reset()
		for i := range b.blocks {
			cb := &b.blocks[i]
			x, y := i%b.ncx, i/b.ncx
			b.incl.encode(w, x, y, 1)
			if cb.keep == 0 {
				continue
			}
			b.zbp.encode(w, x, y, cb.zbp+1)
			writeNumPasses(w, cb.keep)
			n := cb.passes[cb.keep-1].rate
			body += n
			lb := 3 + bits.Len(uint(cb.keep)) - 1
			for k := bits.Len(uint(n)); k > lb; lb++ {
				w.writeBit(1)
			}
			w.writeBit(0)
			w.writeBits(uint32(n), uint(lb))
		}
	}
	w.flush()
	return body
}
