// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package raster

import (
	"bytes"
	"fmt"

	"github.com/sergev/dejapdf/internal/jpeg"
)

// PDF viewers decode JPEG with libjpeg: its integer inverse DCT (which
// internal/jpeg has), its interpolation of subsampled components
// ("fancy upsampling") and its YCbCr conversion, each term rounded on
// its own. We do the same, to get the pixels viewers show.

// decodeJPEG decodes DCT data to 8-bit samples of n components, as
// stored: an Adobe CMYK image stays inverted, as PDF viewers read it
// (its /Decode undoes that). The image is cropped or padded to w×h.
func decodeJPEG(data []byte, w, h, n int) (*samples, error) {
	jp, err := jpeg.DecodePlanes(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("raster: %w", err)
	}
	if len(jp.Comps) != n {
		return nil, fmt.Errorf("raster: JPEG has %d components for %d", len(jp.Comps), n)
	}
	iw, ih := jp.W, jp.H
	planes := make([][]byte, n)
	for i, c := range jp.Comps {
		planes[i] = upsample(c.Pix, c.W, c.H, iw, ih, c.Fx, c.Fy)
	}
	if jp.YCbCr {
		// YCbCr to RGB; for YCCK, CMY = 255 - RGB and K as stored.
		for i := range planes[0] {
			r, g, b := ycc(planes[0][i], planes[1][i], planes[2][i])
			if n == 4 {
				r, g, b = 255-r, 255-g, 255-b
			}
			planes[0][i], planes[1][i], planes[2][i] = r, g, b
		}
	}
	s := &samples{w: w, h: h, n: n, bpc: 8, data: make([]byte, w*h*n)}
	for y := range min(h, ih) {
		for x := range min(w, iw) {
			for c := range n {
				s.data[(y*w+x)*n+c] = planes[c][y*iw+x]
			}
		}
	}
	return s, nil
}

// upsample scales a cw×ch plane by fx×fy to w×h: libjpeg's triangle
// filters for 2×1, 2×2 and 1×2, repetition otherwise.
func upsample(p []byte, cw, ch, w, h, fx, fy int) []byte {
	switch {
	case fx == 1 && fy == 1:
		return p
	case fx == 2 && fy == 1:
		out := make([]byte, w*h)
		row := make([]byte, 2*cw)
		for y := range h {
			h2v1(p[y*cw:(y+1)*cw], row)
			copy(out[y*w:(y+1)*w], row)
		}
		return out
	case fx == 2 && fy == 2:
		out := make([]byte, w*h)
		row := make([]byte, 2*cw)
		sum := make([]int, cw)
		for y := range h {
			in := y / 2
			nb := in - 1 // the upper output row leans on the row above
			if y%2 == 1 {
				nb = in + 1
			}
			nb = min(max(nb, 0), ch-1)
			for x := range cw {
				sum[x] = 3*int(p[in*cw+x]) + int(p[nb*cw+x])
			}
			h2v2(sum, row)
			copy(out[y*w:(y+1)*w], row)
		}
		return out
	case fx == 1 && fy == 2:
		out := make([]byte, w*h)
		for y := range h {
			in := y / 2
			nb, bias := in-1, 1
			if y%2 == 1 {
				nb, bias = in+1, 2
			}
			nb = min(max(nb, 0), ch-1)
			for x := range w {
				out[y*w+x] = byte((3*int(p[in*cw+x]) + int(p[nb*cw+x]) + bias) >> 2)
			}
		}
		return out
	}
	out := make([]byte, w*h)
	for y := range h {
		for x := range w {
			out[y*w+x] = p[(y/fy)*cw+x/fx]
		}
	}
	return out
}

// h2v1 doubles a row: each output sample is 3/4 of the nearer input
// and 1/4 of the farther, with libjpeg's alternating rounding.
func h2v1(in, out []byte) {
	n := len(in)
	if n == 1 {
		out[0], out[1] = in[0], in[0]
		return
	}
	out[0] = in[0]
	out[1] = byte((3*int(in[0]) + int(in[1]) + 2) >> 2)
	for i := 1; i < n-1; i++ {
		v := 3 * int(in[i])
		out[2*i] = byte((v + int(in[i-1]) + 1) >> 2)
		out[2*i+1] = byte((v + int(in[i+1]) + 2) >> 2)
	}
	out[2*n-2] = byte((3*int(in[n-1]) + int(in[n-2]) + 1) >> 2)
	out[2*n-1] = in[n-1]
}

// h2v2 doubles a row of column sums (3 × this row + the neighbour row).
func h2v2(sum []int, out []byte) {
	n := len(sum)
	if n == 1 {
		out[0] = byte((4*sum[0] + 8) >> 4)
		out[1] = byte((4*sum[0] + 7) >> 4)
		return
	}
	out[0] = byte((4*sum[0] + 8) >> 4)
	out[1] = byte((3*sum[0] + sum[1] + 7) >> 4)
	for i := 1; i < n-1; i++ {
		out[2*i] = byte((3*sum[i] + sum[i-1] + 8) >> 4)
		out[2*i+1] = byte((3*sum[i] + sum[i+1] + 7) >> 4)
	}
	out[2*n-2] = byte((3*sum[n-1] + sum[n-2] + 8) >> 4)
	out[2*n-1] = byte((4*sum[n-1] + 7) >> 4)
}

// ycc converts JFIF YCbCr to RGB with libjpeg's fixed-point terms,
// each rounded on its own.
func ycc(y, cb, cr uint8) (uint8, uint8, uint8) {
	const half = 1 << 15
	b := int32(cb) - 128
	r := int32(cr) - 128
	yy := int32(y)
	R := yy + (91881*r+half)>>16
	G := yy + (-22554*b-46802*r+half)>>16
	B := yy + (116130*b+half)>>16
	return clamp(R), clamp(G), clamp(B)
}

func clamp(v int32) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}
