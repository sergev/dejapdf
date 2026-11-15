// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

// Package raster decodes the scan images of a PDF into the forms the
// encoders take: a bitmap for bilevel images, 8-bit gray, RGB or CMYK
// pixels for the others. It applies what PDF says about samples: the
// colour space, /Decode, the palette of Indexed images, and the
// polarity of stencil masks.
package raster

import (
	"errors"
	"fmt"
	"math"

	"github.com/sergev/dejapdf/internal/pdf"
	"github.com/sergev/dejapdf/internal/pdfread"
	"github.com/sergev/dejapdf/internal/scan"
)

// Bitmap is a bilevel image: rows top to bottom, packed MSB first,
// 1 = ink (the darker colour, or the painted part of a stencil).
type Bitmap struct {
	W, H, Stride int
	Pix          []byte
}

// Pixmap is an 8-bit image with N components per pixel: 1 gray (0 is
// black), 3 RGB, or 4 CMYK (0 is no ink); rows top to bottom.
type Pixmap struct {
	W, H, N int
	Pix     []byte
}

// maxPixels caps the size of an image we decode.
const maxPixels = 1 << 28

// Decode decodes a scan image: to *Bitmap if bilevel, else to *Pixmap.
func Decode(r *pdfread.Reader, im *scan.Image) (any, error) {
	s, ok := r.Get(int(im.Ref)).(*pdfread.Stream)
	if !ok {
		return nil, errors.New("raster: not an image stream")
	}
	w, h := im.Width, im.Height
	if w <= 0 || h <= 0 || w*h > maxPixels {
		return nil, fmt.Errorf("raster: bad size %dx%d", w, h)
	}
	cs, err := newColorSpace(r, r.Resolve(s.Dict["ColorSpace"]), im)
	if err != nil {
		return nil, err
	}
	bpc := im.BPC
	n := cs.n // components per sample
	if cs.index != nil || cs.stencil {
		n = 1
	}
	data, err := r.Decode(s)
	var smp *samples
	var u *pdfread.UnsupportedFilterError
	switch {
	case err == nil:
		smp = &samples{w: w, h: h, n: n, bpc: bpc, data: data}
	case errors.As(err, &u) && u.Filter == "DCTDecode":
		smp, err = decodeJPEG(data, w, h, n)
		bpc = 8
	case errors.As(err, &u) && u.Filter == "CCITTFaxDecode":
		smp, err = decodeCCITT(data, filterParms(r, s.Dict, u.Index), w, h)
		bpc = 1
	default:
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	dec := decodeArray(r, s.Dict, cs, bpc)
	if im.Kind == scan.Bilevel {
		return toBitmap(smp, cs, dec)
	}
	return toPixmap(smp, cs, dec)
}

// filterParms returns the DecodeParms of filter i.
func filterParms(r *pdfread.Reader, d pdf.Dict, i int) pdf.Dict {
	p := r.Resolve(d["DecodeParms"])
	if a, ok := p.(pdf.Array); ok {
		if i >= len(a) {
			return nil
		}
		p = r.Resolve(a[i])
	}
	pd, _ := p.(pdf.Dict)
	return pd
}

// samples are the image data as stored: rows of n components of bpc
// bits, each row starting on a byte.
type samples struct {
	w, h, n, bpc int
	data         []byte
}

func (s *samples) stride() int { return (s.w*s.n*s.bpc + 7) / 8 }

// row returns row y, padded with zeros if the data is short.
func (s *samples) row(y int, buf []byte) []byte {
	st := s.stride()
	off := y * st
	if off+st <= len(s.data) {
		return s.data[off : off+st]
	}
	clear(buf)
	if off < len(s.data) {
		copy(buf, s.data[off:])
	}
	return buf
}

// at returns component sample i of a row.
func at(row []byte, i, bpc int) int {
	switch bpc {
	case 8:
		return int(row[i])
	case 16:
		return int(row[2*i])<<8 | int(row[2*i+1])
	}
	bit := i * bpc
	return int(row[bit>>3]) >> (8 - bpc - bit&7) & (1<<bpc - 1)
}

// colorSpace is what decoding needs of a colour space.
type colorSpace struct {
	n       int    // components of the pixels
	stencil bool   // an image mask: one component, 1 = painted
	index   []byte // Indexed: the palette, n bytes per entry
	hival   int
}

func newColorSpace(r *pdfread.Reader, cs any, im *scan.Image) (*colorSpace, error) {
	if im.Stencil {
		return &colorSpace{n: 1, stencil: true}, nil
	}
	switch im.Color {
	case "gray", "rgb", "cmyk":
		return &colorSpace{n: im.Comps}, nil
	case "icc":
		if im.Comps == 1 || im.Comps == 3 || im.Comps == 4 {
			return &colorSpace{n: im.Comps}, nil
		}
	case "index":
		return indexed(r, cs)
	}
	return nil, fmt.Errorf("raster: unsupported colour space %s", im.Color)
}

// indexed reads [/Indexed base hival lookup].
func indexed(r *pdfread.Reader, cs any) (*colorSpace, error) {
	a, _ := cs.(pdf.Array)
	if len(a) != 4 {
		return nil, errors.New("raster: bad Indexed colour space")
	}
	base, n := baseName(r, r.Resolve(a[1]))
	if base == "" {
		return nil, errors.New("raster: unsupported Indexed base")
	}
	hival, _ := r.Resolve(a[2]).(int)
	if hival < 0 || hival > 255 {
		return nil, errors.New("raster: bad Indexed hival")
	}
	var lookup []byte
	switch l := r.Resolve(a[3]).(type) {
	case pdf.String:
		lookup = []byte(l)
	case *pdfread.Stream:
		var err error
		if lookup, err = r.Decode(l); err != nil {
			return nil, err
		}
	}
	// A short palette is padded with zeros, as viewers do.
	if need := (hival + 1) * n; len(lookup) < need {
		lookup = append(lookup, make([]byte, need-len(lookup))...)
	}
	return &colorSpace{n: n, index: lookup, hival: hival}, nil
}

// baseName returns the family of a palette's base colour space and its
// number of components.
func baseName(r *pdfread.Reader, cs any) (string, int) {
	switch cs := cs.(type) {
	case pdf.Name:
		switch cs {
		case "DeviceGray", "CalGray", "G":
			return "gray", 1
		case "DeviceRGB", "CalRGB", "RGB":
			return "rgb", 3
		case "DeviceCMYK", "CMYK":
			return "cmyk", 4
		}
	case pdf.Array:
		if len(cs) == 0 {
			return "", 0
		}
		switch r.Resolve(cs[0]) {
		case pdf.Name("CalGray"):
			return "gray", 1
		case pdf.Name("CalRGB"):
			return "rgb", 3
		case pdf.Name("ICCBased"):
			if len(cs) > 1 {
				if s, ok := r.Resolve(cs[1]).(*pdfread.Stream); ok {
					n, _ := r.Resolve(s.Dict["N"]).(int)
					if n == 1 || n == 3 || n == 4 {
						return "icc", n
					}
				}
			}
		}
	}
	return "", 0
}

// decodeArray returns the /Decode pairs for the sample components, or
// the default.
func decodeArray(r *pdfread.Reader, d pdf.Dict, cs *colorSpace, bpc int) []float64 {
	n := cs.n
	if cs.index != nil || cs.stencil {
		n = 1
	}
	def := make([]float64, 2*n)
	for i := range n {
		def[2*i+1] = 1
	}
	if cs.index != nil {
		def[1] = float64(int(1)<<bpc - 1)
	}
	a, _ := r.Resolve(d["Decode"]).(pdf.Array)
	if len(a) < 2*n {
		return def
	}
	out := make([]float64, 2*n)
	for i := range out {
		switch v := r.Resolve(a[i]).(type) {
		case int:
			out[i] = float64(v)
		case float64:
			out[i] = v
		default:
			return def
		}
	}
	return out
}

// mapSample applies a Decode pair to a sample of bpc bits (8.9.5.2).
func mapSample(s, bpc int, dmin, dmax float64) float64 {
	return dmin + float64(s)*(dmax-dmin)/float64(int(1)<<bpc-1)
}

// level converts a value in 0..1 to a byte.
func level(v float64) byte {
	return byte(math.Round(255 * min(max(v, 0), 1)))
}

// toPixmap expands samples to 8-bit pixels.
func toPixmap(s *samples, cs *colorSpace, dec []float64) (*Pixmap, error) {
	if s.n != cs.n && cs.index == nil {
		return nil, errors.New("raster: components do not match the colour space")
	}
	pm := &Pixmap{W: s.w, H: s.h, N: cs.n, Pix: make([]byte, s.w*s.h*cs.n)}
	buf := make([]byte, s.stride())
	identity := s.bpc == 8 && cs.index == nil
	for i := 0; i < len(dec) && identity; i += 2 {
		identity = dec[i] == 0 && dec[i+1] == 1
	}
	// Each component's levels, by sample value.
	var lut [][]byte
	if !identity && cs.index == nil {
		lut = make([][]byte, cs.n)
		for c := range lut {
			lut[c] = make([]byte, 1<<s.bpc)
			for v := range lut[c] {
				lut[c][v] = level(mapSample(v, s.bpc, dec[2*c], dec[2*c+1]))
			}
		}
	}
	for y := range s.h {
		row := s.row(y, buf)
		out := pm.Pix[y*s.w*cs.n : (y+1)*s.w*cs.n]
		switch {
		case identity:
			copy(out, row)
		case cs.index != nil:
			for x := range s.w {
				i := int(math.Round(mapSample(at(row, x, s.bpc), s.bpc, dec[0], dec[1])))
				i = min(max(i, 0), cs.hival)
				copy(out[x*cs.n:(x+1)*cs.n], cs.index[i*cs.n:])
			}
		default:
			for i := range s.w * cs.n {
				out[i] = lut[i%cs.n][at(row, i, s.bpc)]
			}
		}
	}
	return pm, nil
}

// toBitmap converts 1-bit samples to ink.
func toBitmap(s *samples, cs *colorSpace, dec []float64) (*Bitmap, error) {
	if s.bpc != 1 || s.n != 1 && cs.index == nil {
		return nil, errors.New("raster: not a bilevel image")
	}
	// Which of the two sample values is ink.
	var ink [2]bool
	switch {
	case cs.stencil:
		// Painted where the decoded sample is 0 (8.9.6.2).
		for v := range 2 {
			ink[v] = mapSample(v, 1, dec[0], dec[1]) < 0.5
		}
	case cs.index != nil:
		var lum [2]float64
		for v := range 2 {
			i := min(max(int(math.Round(mapSample(v, 1, dec[0], dec[1]))), 0), cs.hival)
			lum[v] = luminance(cs.index[i*cs.n : (i+1)*cs.n])
		}
		ink[0], ink[1] = lum[0] < lum[1], lum[1] < lum[0]
	case cs.n == 1:
		for v := range 2 {
			ink[v] = mapSample(v, 1, dec[0], dec[1]) < 0.5
		}
	default:
		return nil, errors.New("raster: not a bilevel image")
	}
	bm := &Bitmap{W: s.w, H: s.h, Stride: (s.w + 7) / 8}
	bm.Pix = make([]byte, bm.Stride*s.h)
	if ink[0] == ink[1] {
		if ink[0] {
			for i := range bm.Pix {
				bm.Pix[i] = 0xff
			}
			bm.clearPadding()
		}
		return bm, nil
	}
	buf := make([]byte, s.stride())
	for y := range s.h {
		row := s.row(y, buf)
		out := bm.Pix[y*bm.Stride : (y+1)*bm.Stride]
		copy(out, row)
		if ink[0] {
			for i := range out {
				out[i] = ^out[i]
			}
		}
	}
	bm.clearPadding()
	return bm, nil
}

// clearPadding zeroes the bits past the width in each row.
func (b *Bitmap) clearPadding() {
	if b.W%8 == 0 {
		return
	}
	mask := byte(0xff << (8 - b.W%8))
	for y := range b.H {
		b.Pix[y*b.Stride+b.Stride-1] &= mask
	}
}

// luminance estimates the lightness of a palette colour: gray, RGB or
// CMYK by its number of components.
func luminance(c []byte) float64 {
	switch len(c) {
	case 1:
		return float64(c[0])
	case 3:
		return 0.299*float64(c[0]) + 0.587*float64(c[1]) + 0.114*float64(c[2])
	case 4:
		k := float64(c[3])
		return 255 - min(255, 0.299*float64(c[0])+0.587*float64(c[1])+0.114*float64(c[2])+k)
	}
	return 0
}
