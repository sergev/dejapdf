// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package djvu

import (
	"fmt"

	"github.com/sergev/dejapdf/internal/iff"
	"github.com/sergev/dejapdf/internal/jb2"
)

// Mask decodes the page's JB2 mask (Sjbz), or returns nil if it has
// none. A shared dictionary (Djbz, in the page or an include) is
// decoded once per document.
func (d *Document) Mask(p *Page) (*jb2.Image, error) {
	s := p.Form.Find("Sjbz")
	if s == nil {
		return nil, nil
	}
	var dict *jb2.Dict
	if c := p.Chunks("Djbz"); len(c) > 0 {
		var err error
		if dict, err = d.dict(c[0]); err != nil {
			return nil, err
		}
	}
	im, err := jb2.Decode(s.Data, dict)
	if err != nil {
		return nil, fmt.Errorf("djvu: page %q: %w", p.ID, err)
	}
	return im, nil
}

func (d *Document) dict(c *iff.Chunk) (*jb2.Dict, error) {
	if k, ok := d.dicts[c]; ok {
		return k, nil
	}
	k, err := jb2.DecodeDict(c.Data, nil)
	if err != nil {
		return nil, fmt.Errorf("djvu: Djbz: %w", err)
	}
	if d.dicts == nil {
		d.dicts = map[*iff.Chunk]*jb2.Dict{}
	}
	d.dicts[c] = k
	return k, nil
}
