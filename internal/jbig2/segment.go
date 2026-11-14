// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package jbig2

import "encoding/binary"

// Segment types (7.3).
const (
	TypeSymbolDict    = 0
	TypeTextRegion    = 6
	TypeGenericRegion = 38
	TypePageInfo      = 48
	TypeEndOfPage     = 49
	TypeEndOfFile     = 51
)

// Segment is one segment: header fields and data.
type Segment struct {
	Number uint32
	Type   int
	Refs   []uint32 // referred-to segments, lower numbered
	Page   uint32   // page association, 0 for globals
	Data   []byte
}

// Append appends the segment, header then data (7.2).
func (s *Segment) Append(b []byte) []byte {
	b = binary.BigEndian.AppendUint32(b, s.Number)
	flags := byte(s.Type)
	if s.Page > 255 {
		flags |= 0x40
	}
	b = append(b, flags)
	if n := len(s.Refs); n <= 4 {
		b = append(b, byte(n<<5))
	} else {
		// Long form (7.2.4): count, then one retain bit per reference.
		b = binary.BigEndian.AppendUint32(b, 7<<29|uint32(n))
		b = append(b, make([]byte, (n+8)/8)...)
	}
	for _, r := range s.Refs {
		switch {
		case s.Number <= 256:
			b = append(b, byte(r))
		case s.Number <= 65536:
			b = binary.BigEndian.AppendUint16(b, uint16(r))
		default:
			b = binary.BigEndian.AppendUint32(b, r)
		}
	}
	if s.Page > 255 {
		b = binary.BigEndian.AppendUint32(b, s.Page)
	} else {
		b = append(b, byte(s.Page))
	}
	b = binary.BigEndian.AppendUint32(b, uint32(len(s.Data)))
	return append(b, s.Data...)
}

// PageInfo returns the data of a page information segment (7.4.8): a
// white page, OR operator, no striping. dpi 0 leaves the resolution
// unknown.
func PageInfo(w, h, dpi int) []byte {
	ppm := uint32(dpi * 10000 / 254)
	b := binary.BigEndian.AppendUint32(nil, uint32(w))
	b = binary.BigEndian.AppendUint32(b, uint32(h))
	b = binary.BigEndian.AppendUint32(b, ppm)
	b = binary.BigEndian.AppendUint32(b, ppm)
	return append(b, 0, 0, 0)
}

// regionInfo appends a region segment information field (7.4.1), OR.
func regionInfo(b []byte, w, h, x, y int) []byte {
	b = binary.BigEndian.AppendUint32(b, uint32(w))
	b = binary.BigEndian.AppendUint32(b, uint32(h))
	b = binary.BigEndian.AppendUint32(b, uint32(x))
	b = binary.BigEndian.AppendUint32(b, uint32(y))
	return append(b, 0)
}

// File wraps the segments of one page in the D.4 file header, with end
// of page and end of file, for standalone decoders such as jbig2dec.
func File(segs []Segment) []byte {
	b := []byte{0x97, 'J', 'B', '2', '\r', '\n', 0x1a, '\n', 1, 0, 0, 0, 1}
	n := uint32(0)
	for i := range segs {
		b = segs[i].Append(b)
		n = max(n, segs[i].Number+1)
	}
	end := Segment{Number: n, Type: TypeEndOfPage, Page: 1}
	b = end.Append(b)
	eof := Segment{Number: n + 1, Type: TypeEndOfFile}
	return eof.Append(b)
}

// GenericPage returns an embedded stream for PDF (D.3): the page
// information and one generic region holding the whole bitmap.
func GenericPage(b *Bitmap, dpi int) []byte {
	pi := Segment{Number: 0, Type: TypePageInfo, Page: 1, Data: PageInfo(b.W, b.H, dpi)}
	gr := Segment{Number: 1, Type: TypeGenericRegion, Page: 1, Data: GenericRegion(b, 0, 0)}
	return gr.Append(pi.Append(nil))
}
