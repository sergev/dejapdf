// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package pdfread

import (
	"bytes"
	"compress/flate"
	"compress/zlib"
	"errors"
	"fmt"
	"io"

	"github.com/sergev/dejapdf/internal/pdf"
)

// maxDecoded caps the size of decoded data; the fuzzer lowers it.
var maxDecoded = 1 << 30

// decodeFilters applies a filter or array of filters with their
// parameters. On damaged data it returns what it decoded with an error.
func decodeFilters(data []byte, filter, parms any, resolve func(any) any) ([]byte, error) {
	var fs, ps pdf.Array
	switch f := filter.(type) {
	case nil:
		return data, nil
	case pdf.Name:
		fs = pdf.Array{f}
		ps = pdf.Array{parms}
	case pdf.Array:
		fs = f
		ps, _ = parms.(pdf.Array)
	default:
		return nil, errors.New("pdf: bad /Filter")
	}
	for i, f := range fs {
		var p pdf.Dict
		if i < len(ps) {
			p, _ = resolve(ps[i]).(pdf.Dict)
		}
		name, _ := resolve(f).(pdf.Name)
		var err error
		switch name {
		case "FlateDecode", "Fl":
			data, err = inflate(data)
			if err == nil || len(data) > 0 {
				var err2 error
				data, err2 = predict(data, p, resolve)
				if err == nil {
					err = err2
				}
			}
		case "Crypt":
			// Done by the security handler.
		default:
			return nil, fmt.Errorf("pdf: unsupported filter %s", name)
		}
		if err != nil {
			return data, err
		}
	}
	return data, nil
}

// inflate decodes zlib data; raw deflate data and a broken or missing
// checksum are accepted, as in viewers.
func inflate(data []byte) ([]byte, error) {
	var rd io.Reader
	if zr, err := zlib.NewReader(bytes.NewReader(data)); err == nil {
		rd = zr
	} else {
		rd = flate.NewReader(bytes.NewReader(data))
	}
	out, err := io.ReadAll(io.LimitReader(rd, int64(maxDecoded)))
	if err == zlib.ErrChecksum || err == io.ErrUnexpectedEOF && len(out) > 0 {
		err = nil
	}
	return out, err
}

// predict undoes a PNG predictor (7.4.4.4); TIFF predictor 2 is T3.
func predict(data []byte, p pdf.Dict, resolve func(any) any) ([]byte, error) {
	num := func(k string, def int) int {
		if n, ok := resolve(p[k]).(int); ok {
			return n
		}
		return def
	}
	pred := num("Predictor", 1)
	if pred < 10 {
		if pred == 1 {
			return data, nil
		}
		return data, fmt.Errorf("pdf: unsupported predictor %d", pred)
	}
	colors, bpc, cols := num("Colors", 1), num("BitsPerComponent", 8), num("Columns", 1)
	if colors < 1 || colors > 32 || bpc < 1 || bpc > 16 || cols < 1 || cols > 1<<24 {
		return nil, errors.New("pdf: bad predictor parameters")
	}
	bpp := (colors*bpc + 7) / 8
	row := (colors*bpc*cols + 7) / 8
	if row > maxDecoded {
		return nil, errors.New("pdf: bad predictor parameters")
	}
	out := make([]byte, 0, len(data)/(row+1)*row)
	prev := make([]byte, row)
	for i := 0; i < len(data) && len(out) < maxDecoded; i += row + 1 {
		t := data[i]
		cur := make([]byte, row)
		copy(cur, data[i+1:min(i+1+row, len(data))])
		for k := range row {
			var a, b, c byte
			if k >= bpp {
				a = cur[k-bpp]
				c = prev[k-bpp]
			}
			b = prev[k]
			switch t {
			case 1:
				cur[k] += a
			case 2:
				cur[k] += b
			case 3:
				cur[k] += byte((int(a) + int(b)) / 2)
			case 4:
				cur[k] += paeth(a, b, c)
			}
		}
		out = append(out, cur...)
		prev = cur
	}
	return out, nil
}

func paeth(a, b, c byte) byte {
	p := int(a) + int(b) - int(c)
	pa, pb, pc := abs(p-int(a)), abs(p-int(b)), abs(p-int(c))
	if pa <= pb && pa <= pc {
		return a
	}
	if pb <= pc {
		return b
	}
	return c
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
