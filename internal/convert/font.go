// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package convert

import (
	"bytes"
	"encoding/binary"
	"sort"

	"github.com/sergev/dejapdf/internal/pdf"
)

// Hidden text uses a font whose glyphs are all empty and 500 units
// wide: a Type0 font with Identity-H encoding, so a character code is
// a UTF-16 code unit, mapped back to Unicode by ToUnicode.

const fontName = "GlyphLessFont"

// glyphless returns a TrueType font with an empty .notdef and the
// glyph every CID maps to: a 500×1000 box, never painted (render mode
// 3), but giving readers the character extent.
func glyphless() []byte {
	be := func(v ...any) []byte {
		var b bytes.Buffer
		for _, x := range v {
			binary.Write(&b, binary.BigEndian, x)
		}
		return b.Bytes()
	}
	type u16 = uint16
	type i16 = int16
	type u32 = uint32
	tables := map[string][]byte{
		// version, revision, checksum adjustment, magic, flags, units per em,
		// created, modified, bbox, macStyle, lowest ppem, direction, loca
		// format, glyph format.
		"head": be(u32(0x10000), u32(0x10000), u32(0), u32(0x5f0f3cf5), u16(3), u16(1000),
			uint64(0), uint64(0), i16(0), i16(0), i16(500), i16(1000),
			u16(0), u16(8), i16(2), i16(0), i16(0)),
		// version, ascender, descender, line gap, max advance, min lsb, min
		// rsb, max extent, caret rise, run, offset, 4 reserved, format,
		// number of metrics.
		"hhea": be(u32(0x10000), i16(1000), i16(0), i16(0), u16(500), i16(0), i16(0), i16(0),
			i16(1), i16(0), i16(0), [4]i16{}, i16(0), u16(1)),
		"hmtx": be(u16(500), i16(0), i16(0)),
		"maxp": be(u32(0x10000), u16(2), u16(4), u16(1), u16(0), u16(0), u16(2), [8]u16{}),
		"loca": be(u16(0), u16(0), u16(17)),
		// One contour of 4 on-curve points; x and y as 16-bit deltas.
		"glyf": be(i16(1), i16(0), i16(0), i16(500), i16(1000), u16(3), u16(0), [4]uint8{1, 1, 1, 1},
			[4]i16{0, 500, 0, -500}, [4]i16{0, 0, 1000, 0}),
		// version 3: no glyph names.
		"post": be(u32(0x30000), u32(0), i16(-100), i16(50), u32(0), [4]u32{}),
	}
	tags := make([]string, 0, len(tables))
	for t := range tables {
		tags = append(tags, t)
	}
	sort.Strings(tags)
	n := len(tags)
	sel := 0
	for 2<<sel <= n {
		sel++
	}
	head := be(u32(0x10000), u16(n), u16(16<<sel), u16(sel), u16(16*n-16<<sel))
	off := len(head) + 16*n
	var dir, body []byte
	headOff := 0
	for _, t := range tags {
		d := tables[t]
		pad := append(d, make([]byte, -len(d)&3)...)
		if t == "head" {
			headOff = off + len(body)
		}
		dir = append(dir, t...)
		dir = append(dir, be(checksum(pad), u32(off+len(body)), u32(len(d)))...)
		body = append(body, pad...)
	}
	f := append(append(head, dir...), body...)
	binary.BigEndian.PutUint32(f[headOff+8:], 0xb1b0afba-checksum(f))
	return f
}

func checksum(b []byte) uint32 {
	var s uint32
	for i := 0; i+4 <= len(b); i += 4 {
		s += binary.BigEndian.Uint32(b[i:])
	}
	return s
}

const toUnicode = `/CIDInit /ProcSet findresource begin
12 dict begin
begincmap
/CIDSystemInfo << /Registry (Adobe) /Ordering (UCS) /Supplement 0 >> def
/CMapName /Adobe-Identity-UCS def
/CMapType 2 def
1 begincodespacerange
<0000> <FFFF>
endcodespacerange
1 beginbfrange
<0000> <FFFF> <0000>
endbfrange
endcmap
CMapName currentdict /CMapResource defineresource pop
end
end
`

// textFont is the document's hidden-text font, written on first use.
type textFont struct {
	ref pdf.Ref
}

// get writes the font if needed and returns its reference.
func (f *textFont) get(p *pdf.Writer) pdf.Ref {
	if f.ref != 0 {
		return f.ref
	}
	ttf := glyphless()
	file := p.AddStream(pdf.Dict{"Length1": len(ttf), "Filter": pdf.Name("FlateDecode")}, pdf.Flate(ttf))
	desc := p.Add(pdf.Dict{
		"Type": pdf.Name("FontDescriptor"), "FontName": pdf.Name(fontName),
		"Flags": 5, "FontBBox": pdf.Array{0, 0, 500, 1000}, "ItalicAngle": 0,
		"Ascent": 1000, "Descent": 0, "CapHeight": 1000, "StemV": 80, "FontFile2": file,
	})
	gids := bytes.Repeat([]byte{0, 1}, 1<<16)
	cidToGID := p.AddStream(pdf.Dict{"Filter": pdf.Name("FlateDecode")}, pdf.Flate(gids))
	cid := p.Add(pdf.Dict{
		"Type": pdf.Name("Font"), "Subtype": pdf.Name("CIDFontType2"), "BaseFont": pdf.Name(fontName),
		"CIDSystemInfo":  pdf.Dict{"Registry": pdf.String("Adobe"), "Ordering": pdf.String("Identity"), "Supplement": 0},
		"FontDescriptor": desc, "DW": 500, "CIDToGIDMap": cidToGID,
	})
	cmap := p.AddStream(pdf.Dict{"Filter": pdf.Name("FlateDecode")}, pdf.Flate([]byte(toUnicode)))
	f.ref = p.Add(pdf.Dict{
		"Type": pdf.Name("Font"), "Subtype": pdf.Name("Type0"), "BaseFont": pdf.Name(fontName),
		"Encoding": pdf.Name("Identity-H"), "DescendantFonts": pdf.Array{cid}, "ToUnicode": cmap,
	})
	return f.ref
}
