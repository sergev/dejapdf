package djvu

import (
	"fmt"

	"github.com/sergev/dejapdf/internal/bzz"
	"github.com/sergev/dejapdf/internal/iff"
)

// Annotations returns the S-expressions of all ANTa and ANTz chunks
// of the page and its includes, in order.
func (p *Page) Annotations() ([]*Sexp, error) {
	return annoOf(append([]*iff.Chunk{p.Form}, p.Includes...)...)
}

func annoOf(forms ...*iff.Chunk) ([]*Sexp, error) {
	var r []*Sexp
	for _, f := range forms {
		for _, c := range f.Children {
			b := c.Data
			switch c.ID {
			case "ANTa":
			case "ANTz":
				var err error
				if b, err = bzz.Decode(b); err != nil {
					return r, fmt.Errorf("djvu: ANTz: %w", err)
				}
			default:
				continue
			}
			r = append(r, ParseSexps(b)...)
		}
	}
	return r, nil
}
