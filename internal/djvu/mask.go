// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package djvu

import (
	"fmt"
	"sync"

	"github.com/sergev/dejapdf/internal/iff"
	"github.com/sergev/dejapdf/internal/jb2"
)

// Mask decodes the page's JB2 mask (Sjbz), or its G4 mask (Smmr) as an
// image of one blit, or returns nil if it has neither. A shared
// dictionary (Djbz, in the page or an include) is decoded once per
// document.
func (d *Document) Mask(p *Page) (*jb2.Image, error) {
	s := p.Form.Find("Sjbz")
	if s == nil {
		if m := p.Form.Find("Smmr"); m != nil {
			im, err := mmrMask(m.Data)
			if err != nil {
				return nil, fmt.Errorf("djvu: page %q: %w", p.ID, err)
			}
			return im, nil
		}
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

// dictEntry is a Djbz decoded once.
type dictEntry struct {
	once sync.Once
	dict *jb2.Dict
	err  error
}

func (d *Document) dict(c *iff.Chunk) (*jb2.Dict, error) {
	d.mu.Lock()
	e, ok := d.dicts[c]
	if !ok {
		e = &dictEntry{}
		if d.dicts == nil {
			d.dicts = map[*iff.Chunk]*dictEntry{}
		}
		d.dicts[c] = e
	}
	d.mu.Unlock()
	e.once.Do(func() {
		e.dict, e.err = jb2.DecodeDict(c.Data, nil)
		if e.err != nil {
			e.err = fmt.Errorf("djvu: Djbz: %w", e.err)
		}
	})
	return e.dict, e.err
}
