// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package djvu

import (
	"errors"
	"fmt"

	"github.com/sergev/dejapdf/internal/bzz"
)

// Palette is a decoded FGbz chunk: foreground colours per blit.
type Palette struct {
	Colors [][3]byte // R, G, B
	Index  []int     // colour of each blit, in record order; nil if absent
}

// Palette decodes the page's FGbz chunk, or returns nil if it has none.
func (p *Page) Palette() (*Palette, error) {
	c := p.Form.Find("FGbz")
	if c == nil {
		return nil, nil
	}
	pal, err := parsePalette(c.Data)
	if err != nil {
		return nil, fmt.Errorf("djvu: page %q: FGbz: %w", p.ID, err)
	}
	return pal, nil
}

// parsePalette reads FGbz (DjVu_Format.md §10): flags, colour count,
// B G R triples, then optionally a blit count and BZZ-coded indices.
func parsePalette(b []byte) (*Palette, error) {
	if len(b) < 3 {
		return nil, errors.New("too short")
	}
	if b[0]&0x7f != 0 {
		return nil, fmt.Errorf("version %d", b[0]&0x7f)
	}
	n := int(b[1])<<8 | int(b[2])
	c := b[3:]
	if len(c) < 3*n {
		return nil, errors.New("palette truncated")
	}
	pal := &Palette{Colors: make([][3]byte, n)}
	for i := range n {
		pal.Colors[i] = [3]byte{c[3*i+2], c[3*i+1], c[3*i]}
	}
	if b[0]&0x80 == 0 {
		return pal, nil
	}
	return pal, pal.indices(c[3*n:])
}

func (pal *Palette) indices(b []byte) error {
	if len(b) < 3 {
		return errors.New("blit count missing")
	}
	nb := u24(b)
	d, err := bzz.Decode(b[3:])
	if err != nil {
		return err
	}
	if len(d) < 2*nb {
		return errors.New("indices truncated")
	}
	pal.Index = make([]int, nb)
	for i := range nb {
		k := int(d[2*i])<<8 | int(d[2*i+1])
		if k >= len(pal.Colors) {
			return fmt.Errorf("index %d of %d colours", k, len(pal.Colors))
		}
		pal.Index[i] = k
	}
	return nil
}
