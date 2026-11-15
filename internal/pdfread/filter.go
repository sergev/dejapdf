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

// UnsupportedFilterError is a filter we do not decode: the image
// filters (DCT, CCITT, JBIG2, JPX) and unknown ones. Decode returns it
// with the data decoded up to that filter.
type UnsupportedFilterError struct {
	Filter pdf.Name
	Index  int // in the filter array
}

func (e *UnsupportedFilterError) Error() string {
	return fmt.Sprintf("pdf: unsupported filter %s", e.Filter)
}

var errData = errors.New("pdf: damaged stream data")

// decodeFilters applies a filter or array of filters with their
// parameters (7.4). On damaged data it returns what it decoded with an
// error.
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
		switch p := parms.(type) {
		case pdf.Array:
			ps = p
		case pdf.Dict:
			ps = pdf.Array{p}
		}
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
			data, err = predictAfter(data, err, p, resolve)
		case "LZWDecode", "LZW":
			early := 1
			if e, ok := resolve(p["EarlyChange"]).(int); ok {
				early = e
			}
			data, err = lzwDecode(data, early != 0)
			data, err = predictAfter(data, err, p, resolve)
		case "ASCII85Decode", "A85":
			data, err = ascii85Decode(data)
		case "ASCIIHexDecode", "AHx":
			data, err = asciiHexDecode(data)
		case "RunLengthDecode", "RL":
			data, err = runLengthDecode(data)
		case "Crypt":
			// Done by the security handler.
		default:
			return data, &UnsupportedFilterError{Filter: name, Index: i}
		}
		if err != nil {
			return data, err
		}
	}
	return data, nil
}

// predictAfter applies the predictor to data that decoded, even if
// only in part.
func predictAfter(data []byte, err error, p pdf.Dict, resolve func(any) any) ([]byte, error) {
	if err != nil && len(data) == 0 {
		return data, err
	}
	data, err2 := predict(data, p, resolve)
	if err == nil {
		err = err2
	}
	return data, err
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

// lzwDecode decodes LZW (7.4.4.2): codes of 9 to 12 bits, most
// significant bit first, 256 clear, 257 end. With early change the
// code width grows one code early.
func lzwDecode(data []byte, early bool) ([]byte, error) {
	e := 0
	if early {
		e = 1
	}
	// Each table entry is a run of the output: the previous code's
	// string plus the first byte of the next one, which follows it.
	var off, ln [4096]int
	var out []byte
	width, next := 9, 258
	prevOff, prevLen := -1, 0
	var buf uint32
	bits := 0
	for i := 0; ; {
		for bits < width && i < len(data) {
			buf = buf<<8 | uint32(data[i])
			bits += 8
			i++
		}
		if bits < width {
			return out, nil // no end code: accepted
		}
		code := int(buf>>(bits-width)) & (1<<width - 1)
		bits -= width
		switch {
		case code == 256:
			width, next = 9, 258
			prevOff = -1
			continue
		case code == 257:
			return out, nil
		case len(out) >= maxDecoded:
			return out, errors.New("pdf: stream too large")
		}
		start := len(out)
		switch {
		case code < 256:
			out = append(out, byte(code))
		case code < next && prevOff >= 0:
			out = append(out, out[off[code]:off[code]+ln[code]]...)
		case code == next && prevOff >= 0:
			out = append(out, out[prevOff:prevOff+prevLen]...)
			out = append(out, out[prevOff])
		default:
			return out, errData
		}
		if prevOff >= 0 && next < 4096 {
			off[next], ln[next] = prevOff, prevLen+1
			next++
			if next+e >= 1<<width && width < 12 {
				width++
			}
		}
		prevOff, prevLen = start, len(out)-start
	}
}

// ascii85Decode decodes base-85 (7.4.3); white space is ignored and ~>
// ends the data.
func ascii85Decode(data []byte) ([]byte, error) {
	data = bytes.TrimLeft(data, " \t\r\n\f\x00")
	data = bytes.TrimPrefix(data, []byte("<~"))
	out := make([]byte, 0, len(data)*4/5)
	var v uint64
	n := 0
	for _, c := range data {
		switch {
		case c == '~':
			goto end
		case isSpace(c):
		case c == 'z' && n == 0:
			out = append(out, 0, 0, 0, 0)
		case c >= '!' && c <= 'u':
			v = v*85 + uint64(c-'!')
			n++
			if n == 5 {
				if v > 0xffffffff {
					return out, errData
				}
				out = append(out, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
				v, n = 0, 0
			}
		default:
			return out, errData
		}
	}
end:
	if n == 1 {
		return out, errData
	}
	if n > 1 {
		for k := n; k < 5; k++ {
			v = v*85 + 84
		}
		if v > 0xffffffff {
			return out, errData
		}
		b := []byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)}
		out = append(out, b[:n-1]...)
	}
	return out, nil
}

// asciiHexDecode decodes hex digits up to >; an odd last digit is
// followed by 0 (7.4.2).
func asciiHexDecode(data []byte) ([]byte, error) {
	out := make([]byte, 0, len(data)/2)
	d := -1
	for _, c := range data {
		if c == '>' {
			break
		}
		if isSpace(c) {
			continue
		}
		v := unhex(c)
		if v < 0 {
			return out, errData
		}
		if d < 0 {
			d = v
		} else {
			out = append(out, byte(d<<4|v))
			d = -1
		}
	}
	if d >= 0 {
		out = append(out, byte(d<<4))
	}
	return out, nil
}

// runLengthDecode decodes RunLength (7.4.5): n < 128 copies n+1 bytes,
// n > 128 repeats one byte 257-n times, 128 ends.
func runLengthDecode(data []byte) ([]byte, error) {
	var out []byte
	for i := 0; i < len(data); {
		n := int(data[i])
		i++
		switch {
		case n == 128:
			return out, nil
		case n < 128:
			k := min(n+1, len(data)-i)
			out = append(out, data[i:i+k]...)
			i += k
		case i < len(data):
			for range 257 - n {
				out = append(out, data[i])
			}
			i++
		}
		if len(out) > maxDecoded {
			return out, errors.New("pdf: stream too large")
		}
	}
	return out, nil
}

// predict undoes a TIFF (2) or PNG (10-15) predictor (7.4.4.4).
func predict(data []byte, p pdf.Dict, resolve func(any) any) ([]byte, error) {
	num := func(k string, def int) int {
		if n, ok := resolve(p[k]).(int); ok {
			return n
		}
		return def
	}
	pred := num("Predictor", 1)
	if pred == 1 {
		return data, nil
	}
	if pred != 2 && pred < 10 || pred > 15 {
		return data, fmt.Errorf("pdf: unsupported predictor %d", pred)
	}
	colors, bpc, cols := num("Colors", 1), num("BitsPerComponent", 8), num("Columns", 1)
	if colors < 1 || colors > 32 || cols < 1 || cols > 1<<24 {
		return nil, errors.New("pdf: bad predictor parameters")
	}
	switch bpc {
	case 1, 2, 4, 8, 16:
	default:
		return nil, errors.New("pdf: bad predictor parameters")
	}
	row := (colors*bpc*cols + 7) / 8
	if row > maxDecoded {
		return nil, errors.New("pdf: bad predictor parameters")
	}
	if pred == 2 {
		return tiffPredict(data, row, colors, bpc, cols), nil
	}
	bpp := (colors*bpc + 7) / 8
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

// tiffPredict undoes TIFF predictor 2: each sample is a difference from
// the sample of the same colour to its left. A partial last row is
// decoded as far as it goes.
func tiffPredict(data []byte, row, colors, bpc, cols int) []byte {
	out := bytes.Clone(data)
	for r := 0; r < len(out); r += row {
		b := out[r:min(r+row, len(out))]
		switch bpc {
		case 8:
			for k := colors; k < len(b); k++ {
				b[k] += b[k-colors]
			}
		case 16:
			for k := 2 * colors; k+1 < len(b); k += 2 {
				v := uint16(b[k])<<8 | uint16(b[k+1])
				v += uint16(b[k-2*colors])<<8 | uint16(b[k-2*colors+1])
				b[k], b[k+1] = byte(v>>8), byte(v)
			}
		default:
			mask := 1<<bpc - 1
			get := func(s int) int { return int(b[s*bpc/8]) >> (8 - bpc - s*bpc%8) & mask }
			set := func(s, v int) {
				sh := 8 - bpc - s*bpc%8
				b[s*bpc/8] = b[s*bpc/8]&^byte(mask<<sh) | byte(v<<sh)
			}
			for s := colors; s < cols*colors && s*bpc/8 < len(b); s++ {
				set(s, (get(s)+get(s-colors))&mask)
			}
		}
	}
	return out
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
