// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package djvu

import (
	"fmt"

	"github.com/sergev/dejapdf/internal/iw44"
)

// Background decodes the page's BG44 chunks, or returns nil if it has
// none. The image has the layer's own (usually reduced) size.
func (p *Page) Background() (*iw44.Image, error) {
	return p.iw44("BG44")
}

// Foreground decodes the page's FG44 chunk, or returns nil.
func (p *Page) Foreground() (*iw44.Image, error) {
	return p.iw44("FG44")
}

func (p *Page) iw44(id string) (*iw44.Image, error) {
	cs := p.Form.All(id)
	if len(cs) == 0 {
		return nil, nil
	}
	var d iw44.Decoder
	for _, c := range cs {
		if err := d.Decode(c.Data); err != nil {
			return nil, fmt.Errorf("djvu: page %q: %s: %w", p.ID, id, err)
		}
	}
	return d.Image()
}
