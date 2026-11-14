// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package convert

import (
	"errors"

	"github.com/sergev/dejapdf/internal/pdf"
)

// jpegSize reads the size and component count from a JPEG's SOF marker.
func jpegSize(b []byte) (w, h, nc int, err error) {
	if len(b) < 4 || b[0] != 0xff || b[1] != 0xd8 {
		return 0, 0, 0, errors.New("not a JPEG")
	}
	for i := 2; i+4 <= len(b); {
		if b[i] != 0xff {
			return 0, 0, 0, errors.New("bad JPEG marker")
		}
		m := b[i+1]
		if m == 0xff {
			i++
			continue
		}
		n := int(b[i+2])<<8 | int(b[i+3])
		if m >= 0xc0 && m <= 0xcf && m != 0xc4 && m != 0xc8 && m != 0xcc {
			if i+10 > len(b) {
				break
			}
			h = int(b[i+5])<<8 | int(b[i+6])
			w = int(b[i+7])<<8 | int(b[i+8])
			return w, h, int(b[i+9]), nil
		}
		i += 2 + n
	}
	return 0, 0, 0, errors.New("no JPEG frame header")
}

// jpeg returns the dictionary of a JPEG file used unchanged as a
// DCTDecode image, and its size.
func jpeg(b []byte) (pdf.Dict, int, int, error) {
	w, h, nc, err := jpegSize(b)
	if err != nil {
		return nil, 0, 0, err
	}
	cs := map[int]string{1: "DeviceGray", 3: "DeviceRGB", 4: "DeviceCMYK"}[nc]
	if cs == "" || w == 0 || h == 0 {
		return nil, 0, 0, errors.New("unsupported JPEG")
	}
	d := pdf.Dict{
		"Type":             pdf.Name("XObject"),
		"Subtype":          pdf.Name("Image"),
		"Width":            w,
		"Height":           h,
		"ColorSpace":       pdf.Name(cs),
		"BitsPerComponent": 8,
		"Filter":           pdf.Name("DCTDecode"),
	}
	return d, w, h, nil
}
