// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package raster

import (
	"bytes"
	"errors"
	"io"

	"github.com/sergev/dejapdf/internal/ccitt"
	"github.com/sergev/dejapdf/internal/pdf"
)

// decodeCCITT decodes CCITT fax data (7.4.6) to 1-bit samples: G4, or
// G3 1-D. With BlackIs1 false, as by default, black is 0. Columns
// other than the width are cropped or padded; damaged data leaves the
// rest of the image white.
func decodeCCITT(data []byte, p pdf.Dict, w, h int) (*samples, error) {
	num := func(k string, def int) int {
		if n, ok := p[k].(int); ok {
			return n
		}
		return def
	}
	flag := func(k string, def bool) bool {
		if b, ok := p[k].(bool); ok {
			return b
		}
		return def
	}
	k := num("K", 0)
	cols := num("Columns", 1728)
	rows := num("Rows", 0)
	if rows <= 0 || rows > h {
		rows = h
	}
	if cols <= 0 || cols > 1<<16 {
		return nil, errors.New("raster: bad CCITT columns")
	}
	sf := ccitt.Group4
	switch {
	case k > 0:
		return nil, errors.New("raster: CCITT G3 2-D is not supported")
	case k == 0:
		sf = ccitt.Group3
	}
	black1 := flag("BlackIs1", false)
	opts := &ccitt.Options{Align: flag("EncodedByteAlign", false), Invert: black1}
	rd := ccitt.NewReader(bytes.NewReader(data), ccitt.MSB, sf, cols, rows, opts)
	cstride := (cols + 7) / 8
	buf := make([]byte, cstride*rows)
	n, err := io.ReadFull(rd, buf)
	if n < cstride {
		return nil, errors.New("raster: damaged CCITT data")
	}
	_ = err // A damaged tail stays white, as in viewers.
	white := byte(0xff)
	if black1 {
		white = 0
	}
	for i := n / cstride * cstride; i < len(buf); i++ {
		buf[i] = white
	}
	s := &samples{w: w, h: h, n: 1, bpc: 1}
	if cols == w && rows == h {
		s.data = buf
		return s, nil
	}
	stride := (w + 7) / 8
	s.data = make([]byte, stride*h)
	for i := range s.data {
		s.data[i] = white
	}
	for y := range rows {
		row := s.data[y*stride : (y+1)*stride]
		src := buf[y*cstride : (y+1)*cstride]
		copy(row, src[:min(cstride, stride)])
		if cols < w && cols%8 != 0 {
			// Past the coded columns is white.
			m := byte(0xff >> (cols % 8))
			if black1 {
				row[cols/8] &^= m
			} else {
				row[cols/8] |= m
			}
		}
	}
	return s, nil
}
