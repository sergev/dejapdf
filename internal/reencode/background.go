// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package reencode

import (
	"github.com/sergev/dejapdf/internal/jpx"
	"github.com/sergev/dejapdf/internal/pdf"
	"github.com/sergev/dejapdf/internal/raster"
	"github.com/sergev/dejapdf/internal/segment"
)

// DefaultQuality caps a background's size, as a fraction of the
// original image's.
const DefaultQuality = 0.15

// bgSlope is the least squared error (8-bit levels) a byte of a
// background must remove: grain costs nothing, photos up to the cap.
var bgSlope = 100.0

// layer is a coded background: a JPX image, or a uniform fill.
type layer struct {
	dict    pdf.Dict
	data    []byte
	fill    segment.Color
	uniform bool
}

// background codes the background of a scan at dpi with text mask m:
// reduced to about 100 dpi, filled under the mask, as JPX of at most
// budget bytes, or a fill when one colour.
func background(p *raster.Pixmap, m *raster.Bitmap, dpi float64, budget int) (*layer, error) {
	k := max(1, int(dpi/100+0.5))
	r0 := max(2, int(dpi/100+0.5))
	bg := segment.Background(p, m, k, r0, max(r0+2, int(dpi/20)))
	if c, ok := segment.Uniform(bg); ok {
		return &layer{fill: c, uniform: true}, nil
	}
	planes := [][]byte{bg.Pix}
	cs := "DeviceGray"
	if bg.N == 3 {
		cs = "DeviceRGB"
		planes = make([][]byte, 3)
		for c := range planes {
			planes[c] = make([]byte, bg.W*bg.H)
			for i := range planes[c] {
				planes[c][i] = bg.Pix[3*i+c]
			}
		}
	}
	data, err := jpx.EncodeSlope(bg.W, bg.H, planes, max(1, budget), bgSlope)
	if err != nil {
		return nil, err
	}
	d := pdf.Dict{
		"Type":       pdf.Name("XObject"),
		"Subtype":    pdf.Name("Image"),
		"Width":      bg.W,
		"Height":     bg.H,
		"ColorSpace": pdf.Name(cs),
		"Filter":     pdf.Name("JPXDecode"),
	}
	return &layer{dict: d, data: data}, nil
}
