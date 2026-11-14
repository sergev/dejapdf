// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package jpx

import "math"

// 9/7 lifting constants (T.800 Table F.4).
const (
	alpha = -1.586134342059924
	beta  = -0.052980118572961
	gamma = 0.882911075530934
	delta = 0.443506852043971
	kappa = 1.230174104914001
)

// fwd97 is one level of the forward 9/7 transform of x (F.4.8.2), with
// whole-sample symmetric extension; the result is deinterleaved into
// low then high samples, using tmp.
func fwd97(x, tmp []float32) {
	n := len(x)
	if n < 2 {
		return // a single sample passes through (F.4.8.1)
	}
	lift(x, 1, alpha)
	lift(x, 0, beta)
	lift(x, 1, gamma)
	lift(x, 0, delta)
	nl := (n + 1) / 2
	for i := range nl {
		tmp[i] = x[2*i] / kappa
	}
	for i := range n / 2 {
		tmp[nl+i] = x[2*i+1] * kappa
	}
	copy(x, tmp[:n])
}

// inv97 undoes fwd97.
func inv97(x, tmp []float32) {
	n := len(x)
	if n < 2 {
		return
	}
	nl := (n + 1) / 2
	for i := range nl {
		tmp[2*i] = x[i] * kappa
	}
	for i := range n / 2 {
		tmp[2*i+1] = x[nl+i] / kappa
	}
	copy(x, tmp[:n])
	lift(x, 0, -delta)
	lift(x, 1, -gamma)
	lift(x, 0, -beta)
	lift(x, 1, -alpha)
}

// lift adds c times the sum of both neighbours to the samples of parity p,
// mirroring at the ends.
func lift(x []float32, p int, c float32) {
	n := len(x)
	for i := p; i < n; i += 2 {
		l, r := i-1, i+1
		if l < 0 {
			l = 1
		}
		if r >= n {
			r = n - 2
		}
		x[i] += c * (x[l] + x[r])
	}
}

// forward applies nl levels of the 2-D transform (F.4) to the w×h plane p
// in place, leaving the Mallat layout: each level's LL in the top left.
func forward(p []float32, w, h, nl int) {
	stride := w
	tmp := make([]float32, max(w, h))
	line := make([]float32, max(w, h))
	for range nl {
		for x := range w {
			for y := range h {
				line[y] = p[y*stride+x]
			}
			fwd97(line[:h], tmp)
			for y := range h {
				p[y*stride+x] = line[y]
			}
		}
		for y := range h {
			fwd97(p[y*stride:y*stride+w], tmp)
		}
		w, h = (w+1)/2, (h+1)/2
	}
}

// inverse undoes forward.
func inverse(p []float32, w, h, nl int) {
	stride := w
	tmp := make([]float32, max(w, h))
	line := make([]float32, max(w, h))
	for l := nl - 1; l >= 0; l-- {
		lw, lh := ceilDiv(w, l), ceilDiv(h, l)
		for y := range lh {
			inv97(p[y*stride:y*stride+lw], tmp)
		}
		for x := range lw {
			for y := range lh {
				line[y] = p[y*stride+x]
			}
			inv97(line[:lh], tmp)
			for y := range lh {
				p[y*stride+x] = line[y]
			}
		}
	}
}

// ceilDiv is ⌈n / 2^k⌉.
func ceilDiv(n, k int) int { return (n + 1<<k - 1) >> k }

// norms97 returns the L2 norm of the synthesis basis functions of a 1-D
// low band after n levels and of a high band at level n (n ≥ 1).
func norms97(n int) (low, high float64) {
	const size = 1024
	basis := func(start int) float64 {
		x := make([]float32, size)
		tmp := make([]float32, size)
		x[start] = 1
		for k := n; k >= 1; k-- {
			m := size >> (k - 1)
			inv97(x[:m], tmp)
		}
		s := 0.0
		for _, v := range x {
			s += float64(v) * float64(v)
		}
		return math.Sqrt(s)
	}
	m := size >> n        // low samples after n levels
	low = basis(m / 2)    // middle of the low band
	high = basis(m + m/2) // middle of level n's high band
	return
}
