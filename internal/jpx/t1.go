package jpx

import (
	"math/bits"

	"github.com/sergev/dejapdf/internal/mq"
)

// Band orientations.
const (
	bandLL = iota
	bandHL
	bandLH
	bandHH
)

// Contexts (D.3): 0-8 zero coding, 9-13 sign, 14-16 refinement, run
// length, uniform.
const (
	ctxSC  = 9
	ctxMR  = 14
	ctxRL  = 17
	ctxUni = 18
)

// fracBits extra fraction bits kept below the quantization index, for
// distortion estimates.
const fracBits = 6

// Coefficient flags.
const (
	fSig   = 1 << iota // significant
	fVisit             // coded in this bit-plane's earlier pass
	fRef               // refined at least once
	fNeg               // negative
)

// zcTab maps orientation and the counts of significant horizontal,
// vertical and diagonal neighbours to a context (Table D.1).
var zcTab [4][3][3][5]uint8

func init() {
	for o := range 4 {
		for h := range 3 {
			for v := range 3 {
				for d := range 5 {
					zcTab[o][h][v][d] = zc(o, h, v, d)
				}
			}
		}
	}
}

func zc(o, h, v, d int) uint8 {
	if o == bandHL {
		h, v = v, h
	}
	if o == bandHH {
		hv := h + v
		switch {
		case d >= 3:
			return 8
		case d == 2:
			return 6 + b2u(hv >= 1)
		case d == 1:
			return 3 + uint8(min(hv, 2))
		}
		return uint8(min(hv, 2))
	}
	switch {
	case h == 2:
		return 8
	case h == 1:
		if v >= 1 {
			return 7
		}
		return 5 + b2u(d >= 1)
	case v == 2:
		return 4
	case v == 1:
		return 3
	}
	return uint8(min(d, 2))
}

func b2u(b bool) uint8 {
	if b {
		return 1
	}
	return 0
}

// scTab maps horizontal and vertical sign contributions (+1 each) to a
// context and the XOR bit (Table D.3).
var scTab = [3][3][2]uint8{
	{{13, 1}, {12, 1}, {11, 1}},
	{{10, 1}, {9, 0}, {10, 0}},
	{{11, 0}, {12, 0}, {13, 0}},
}

// pass is a truncation point: bytes and distortion reduction, cumulative.
type pass struct {
	rate int
	dist float64 // in (index / 2^fracBits)² units
}

// t1 codes code-blocks (Annex D) with one codeword segment.
type t1 struct {
	w, h   int
	stride int
	mag    []uint32 // |coefficient| / step, fracBits fraction bits
	flags  []uint8  // padded by one on every side
	enc    mq.Encoder
	cx     [19]mq.Context
	zc     *[3][3][5]uint8
	dist   float64
	passes []pass
}

// code codes the w×h block in t.mag (signs in t.flags via setNeg) and
// returns the number of magnitude bit-planes, the data and the passes.
func (t *t1) code(orient int) (int, []byte, []pass) {
	var mx uint32
	for _, m := range t.mag[:t.w*t.h] {
		mx = max(mx, m)
	}
	nbp := bits.Len32(mx >> fracBits)
	t.passes = t.passes[:0]
	if nbp == 0 {
		return 0, nil, nil
	}
	t.zc = &zcTab[orient]
	t.enc.Reset()
	for i := range t.cx {
		t.cx[i] = 0
	}
	t.cx[0] = mq.NewContext(4)
	t.cx[ctxRL] = mq.NewContext(3)
	t.cx[ctxUni] = mq.NewContext(46)
	t.dist = 0
	for p := nbp - 1; p >= 0; p-- {
		if p < nbp-1 {
			t.sigPass(p)
			t.mark()
			t.refPass(p)
			t.mark()
		}
		t.cleanup(p)
		t.mark()
	}
	data := t.enc.FlushJPX()
	// Rates must not decrease, nor end on 0xFF.
	last := len(data)
	for i := len(t.passes) - 1; i >= 0; i-- {
		t.passes[i].rate = min(t.passes[i].rate, last)
		last = t.passes[i].rate
	}
	for i := range t.passes {
		if r := t.passes[i].rate; r > 0 && data[r-1] == 0xff {
			t.passes[i].rate--
		}
	}
	return nbp, data, t.passes
}

// mark ends a pass. A cut there keeps 3 bytes past those output, enough
// for the decoder to finish the pass (as OpenJPEG does).
func (t *t1) mark() {
	t.passes = append(t.passes, pass{t.enc.NumBytes() + 3, t.dist})
}

// reset prepares a w×h block.
func (t *t1) reset(w, h int) {
	t.w, t.h, t.stride = w, h, w+2
	n := (w + 2) * (h + 2)
	if cap(t.flags) < n {
		t.flags = make([]uint8, n)
		t.mag = make([]uint32, 64*64)
	}
	t.flags = t.flags[:n]
	clear(t.flags)
}

// fi is the flag index of (x, y).
func (t *t1) fi(x, y int) int { return (y+1)*t.stride + x + 1 }

func (t *t1) setNeg(x, y int) { t.flags[t.fi(x, y)] |= fNeg }

// zcCtx is the zero coding context at flag index i.
func (t *t1) zcCtx(i int) uint8 {
	f, s := t.flags, t.stride
	h := f[i-1]&fSig + f[i+1]&fSig
	v := f[i-s]&fSig + f[i+s]&fSig
	d := f[i-s-1]&fSig + f[i-s+1]&fSig + f[i+s-1]&fSig + f[i+s+1]&fSig
	return t.zc[h][v][d]
}

// contrib is a neighbour's sign contribution: 0, 1 or -1.
func contrib(f uint8) int {
	if f&fSig == 0 {
		return 0
	}
	if f&fNeg != 0 {
		return -1
	}
	return 1
}

func (t *t1) sign(i int) {
	f, s := t.flags, t.stride
	h := max(-1, min(1, contrib(f[i-1])+contrib(f[i+1])))
	v := max(-1, min(1, contrib(f[i-s])+contrib(f[i+s])))
	e := scTab[h+1][v+1]
	t.enc.Encode(&t.cx[e[0]], int(f[i]>>3&1^e[1]))
}

// newSig codes the sign of a coefficient found significant at plane p.
func (t *t1) newSig(i, k, p int) {
	t.sign(i)
	t.flags[i] |= fSig
	m := int64(t.mag[k])
	e := m - (1<<(p+fracBits) | 1<<(p+fracBits-1))
	t.dist += float64(m*m - e*e)
}

func (t *t1) bit(k, p int) int { return int(t.mag[k] >> (p + fracBits) & 1) }

// sigPass is the significance propagation pass (D.3.1).
func (t *t1) sigPass(p int) {
	for y0 := 0; y0 < t.h; y0 += 4 {
		for x := range t.w {
			for y := y0; y < min(y0+4, t.h); y++ {
				i := t.fi(x, y)
				if t.flags[i]&fSig != 0 {
					continue
				}
				c := t.zcCtx(i)
				if c == 0 {
					continue
				}
				k := y*t.w + x
				b := t.bit(k, p)
				t.enc.Encode(&t.cx[c], b)
				if b == 1 {
					t.newSig(i, k, p)
				}
				t.flags[i] |= fVisit
			}
		}
	}
}

// refPass is the magnitude refinement pass (D.3.3).
func (t *t1) refPass(p int) {
	s := t.stride
	for y0 := 0; y0 < t.h; y0 += 4 {
		for x := range t.w {
			for y := y0; y < min(y0+4, t.h); y++ {
				i := t.fi(x, y)
				f := t.flags
				if f[i]&(fSig|fVisit) != fSig {
					continue
				}
				c := 16
				if f[i]&fRef == 0 {
					c = 14
					if (f[i-1]|f[i+1]|f[i-s]|f[i+s]|f[i-s-1]|f[i-s+1]|f[i+s-1]|f[i+s+1])&fSig != 0 {
						c = 15
					}
				}
				k := y*t.w + x
				t.enc.Encode(&t.cx[c], t.bit(k, p))
				f[i] |= fRef
				m := int64(t.mag[k])
				q := m >> (p + fracBits + 1) << (p + fracBits + 1)
				e0 := m - (q | 1<<(p+fracBits))
				e1 := m - (m>>(p+fracBits)<<(p+fracBits) | 1<<(p+fracBits-1))
				t.dist += float64(e0*e0 - e1*e1)
			}
		}
	}
}

// cleanup is the cleanup pass (D.3.4); it also clears the visit flags.
func (t *t1) cleanup(p int) {
	for y0 := 0; y0 < t.h; y0 += 4 {
		y1 := min(y0+4, t.h)
		for x := range t.w {
			y := y0
			if y1-y0 == 4 && t.runnable(x, y0) {
				first := 4
				for k := range 4 {
					if t.bit((y0+k)*t.w+x, p) == 1 {
						first = k
						break
					}
				}
				if first == 4 {
					t.enc.Encode(&t.cx[ctxRL], 0)
					continue
				}
				t.enc.Encode(&t.cx[ctxRL], 1)
				t.enc.Encode(&t.cx[ctxUni], first>>1)
				t.enc.Encode(&t.cx[ctxUni], first&1)
				y = y0 + first
				t.newSig(t.fi(x, y), y*t.w+x, p)
				y++
			}
			for ; y < y1; y++ {
				i := t.fi(x, y)
				if t.flags[i]&(fSig|fVisit) != 0 {
					t.flags[i] &^= fVisit
					continue
				}
				k := y*t.w + x
				b := t.bit(k, p)
				t.enc.Encode(&t.cx[t.zcCtx(i)], b)
				if b == 1 {
					t.newSig(i, k, p)
				}
			}
		}
	}
}

// runnable reports whether the column of four at (x, y0) can use run
// length coding: none significant, visited, or with a significant
// neighbour.
func (t *t1) runnable(x, y0 int) bool {
	for y := y0; y < y0+4; y++ {
		i := t.fi(x, y)
		if t.flags[i]&(fSig|fVisit) != 0 || t.zcCtx(i) != 0 {
			return false
		}
	}
	return true
}
