// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package convert

import "github.com/sergev/dejapdf/internal/djvu"

// Time of converting each kind of layer, in nanoseconds per byte of its
// chunks (on all CPUs). Measured on DjVu books (16 CPUs); they only
// pace the progress line.
var layerCost = map[string]float64{
	"BG44": 1000, // IW44 decode, JPX encode
	"FG44": 1000,
	"BGjp": 1, // passed through
	"FGjp": 1,
	"Sjbz": 100, // JB2 decode, JBIG2 encode
	"Smmr": 100,
	"FGbz": 1,
	"TXTz": 80,
	"TXTa": 80,
	"ANTz": 1,
	"ANTa": 1,
}

// pagesShare is the share of the pages in the time after reading; the
// rest is writing the outline and the cross-reference stream.
const pagesShare = 0.98

// pageCost is the time of building a page by step: background,
// foreground, text and links.
type pageCost struct{ bg, fg, text int64 }

func (c pageCost) total() int64 { return c.bg + c.fg + c.text }

func costOf(pg *djvu.Page) pageCost {
	if pg.Form == nil {
		return pageCost{}
	}
	n := func(ids ...string) int64 {
		var s float64
		for _, id := range ids {
			for _, c := range pg.Form.All(id) {
				s += float64(len(c.Data)) * layerCost[id]
			}
		}
		return int64(s) + 1
	}
	return pageCost{
		bg:   n("BG44", "BGjp"),
		fg:   n("Sjbz", "Smmr", "FG44", "FGjp", "FGbz"),
		text: n("TXTz", "TXTa", "ANTz", "ANTa"),
	}
}
