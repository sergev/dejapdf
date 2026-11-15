// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package jpeg

// The inverse DCT of libjpeg's "islow" method (the accurate integer
// method of libjpeg 6b and libjpeg-turbo), written from its published
// algorithm (Loeffler, Ligtenberg and Moschytz, with 13-bit constants
// and 2 extra bits between passes), so that pixels come out as PDF
// viewers decode them.

// A block is an 8x8 array of DCT coefficients or samples.
type block [blockSize]int32

const blockSize = 8 * 8

const (
	constBits = 13
	pass1Bits = 2

	fix0_298631336 = 2446
	fix0_390180644 = 3196
	fix0_541196100 = 4433
	fix0_765366865 = 6270
	fix0_899976223 = 7373
	fix1_175875602 = 9633
	fix1_501321110 = 12299
	fix1_847759065 = 15137
	fix1_961570560 = 16069
	fix2_053119869 = 16819
	fix2_562915447 = 20995
	fix3_072711026 = 25172
)

// descale divides by 2^n, rounding.
func descale(x int32, n uint) int32 { return (x + 1<<(n-1)) >> n }

// idct transforms dequantized coefficients in natural order into
// samples minus 128: columns first, then rows.
func idct(b *block) {
	var ws [blockSize]int32
	for c := range 8 {
		ws[0*8+c], ws[1*8+c], ws[2*8+c], ws[3*8+c],
			ws[4*8+c], ws[5*8+c], ws[6*8+c], ws[7*8+c] = idct1(
			b[0*8+c], b[1*8+c], b[2*8+c], b[3*8+c], b[4*8+c], b[5*8+c], b[6*8+c], b[7*8+c],
			constBits-pass1Bits)
	}
	for r := range 8 {
		w := ws[r*8 : r*8+8]
		b[r*8+0], b[r*8+1], b[r*8+2], b[r*8+3], b[r*8+4], b[r*8+5], b[r*8+6], b[r*8+7] = idct1(
			w[0], w[1], w[2], w[3], w[4], w[5], w[6], w[7], constBits+pass1Bits+3)
	}
}

// idct1 is one 1-D pass, descaling its outputs by n bits.
func idct1(i0, i1, i2, i3, i4, i5, i6, i7 int32, n uint) (o0, o1, o2, o3, o4, o5, o6, o7 int32) {
	// Even part.
	z1 := (i2 + i6) * fix0_541196100
	tmp2 := z1 - i6*fix1_847759065
	tmp3 := z1 + i2*fix0_765366865
	tmp0 := (i0 + i4) << constBits
	tmp1 := (i0 - i4) << constBits
	tmp10, tmp13 := tmp0+tmp3, tmp0-tmp3
	tmp11, tmp12 := tmp1+tmp2, tmp1-tmp2

	// Odd part.
	t0, t1, t2, t3 := i7, i5, i3, i1
	z1 = t0 + t3
	z2 := t1 + t2
	z3 := t0 + t2
	z4 := t1 + t3
	z5 := (z3 + z4) * fix1_175875602
	t0 *= fix0_298631336
	t1 *= fix2_053119869
	t2 *= fix3_072711026
	t3 *= fix1_501321110
	z1 *= -fix0_899976223
	z2 *= -fix2_562915447
	z3 = z3*-fix1_961570560 + z5
	z4 = z4*-fix0_390180644 + z5
	t0 += z1 + z3
	t1 += z2 + z4
	t2 += z2 + z3
	t3 += z1 + z4

	return descale(tmp10+t3, n), descale(tmp11+t2, n), descale(tmp12+t1, n), descale(tmp13+t0, n),
		descale(tmp13-t0, n), descale(tmp12-t1, n), descale(tmp11-t2, n), descale(tmp10-t3, n)
}
