// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package convert

import (
	"errors"

	"github.com/sergev/dejapdf/internal/djvu"
	"github.com/sergev/dejapdf/internal/pdf"
)

// mmrStencil returns an Smmr mask (DjVu_Format.md §11) as a stencil
// passing its G4 data through to CCITTFaxDecode.
func mmrStencil(b []byte) (pdf.Dict, []byte, error) {
	if len(b) < 8 || string(b[:3]) != "MMR" {
		return nil, nil, errors.New("Smmr: bad header")
	}
	if b[3]&2 != 0 {
		return nil, nil, errors.New("Smmr: striped masks not supported")
	}
	w, h := int(b[4])<<8|int(b[5]), int(b[6])<<8|int(b[7])
	if w == 0 || h == 0 {
		return nil, nil, errors.New("Smmr: empty")
	}
	d := pdf.Dict{
		"Subtype":     pdf.Name("Image"),
		"Width":       w,
		"Height":      h,
		"ImageMask":   true,
		"Filter":      pdf.Name("CCITTFaxDecode"),
		"DecodeParms": pdf.Dict{"K": -1, "Columns": w, "Rows": h},
	}
	// CCITTFaxDecode gives black as 0, which a stencil paints; reverse
	// video swaps the colours.
	if b[3]&1 != 0 {
		d["Decode"] = pdf.Array{1, 0}
	}
	return d, b[8:], nil
}

// mmrForeground paints an Smmr mask in one colour: a uniform FG44, the
// first FGbz colour, or black. A varying FG44 is not supported yet and
// is painted black.
func mmrForeground(b *pageBuilder, pg *djvu.Page, chunk []byte, opt *Options) error {
	d, data, err := mmrStencil(chunk)
	if err != nil {
		return err
	}
	col := [3]byte{}
	if !opt.MaskOnly {
		if pal, err := pg.Palette(); err != nil {
			return err
		} else if pal != nil && len(pal.Colors) > 0 {
			col = pal.Colors[0]
		} else if fg, err := pg.Foreground(); err != nil {
			return err
		} else if fg != nil {
			if u, ok := uniform(fg); ok {
				col = u
			}
		}
	}
	b.fill(col)
	b.draw(b.add("Sm", b.obj(d, data)), d["Width"].(int), d["Height"].(int))
	return nil
}
