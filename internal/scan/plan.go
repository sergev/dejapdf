// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package scan

import (
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/sergev/dejapdf/internal/pdf"
	"github.com/sergev/dejapdf/internal/pdfread"
)

// Action is what to do with an image.
type Action int

const (
	None     Action = iota // not a scan image
	Keep                   // a scan image left as it is; see the reason
	JBIG2                  // bilevel: re-encode losslessly as JBIG2
	Separate               // gray or colour: a mask and a background
)

func (a Action) String() string {
	return [...]string{"none", "keep", "JBIG2", "separate"}[a]
}

// minScanned is the part of the pages that must be scanned for the
// document to be treated as scanned; born-digital documents are left
// alone.
const minScanned = 0.5

// Scanned reports whether most pages of the document are scanned.
func (d *Document) Scanned() bool {
	n := 0
	for _, p := range d.Pages {
		if p.Scanned {
			n++
		}
	}
	return len(d.Pages) > 0 && float64(n) >= minScanned*float64(len(d.Pages))
}

// Plan decides what to do with an image drawn on a page, and why.
func (d *Document) Plan(r *pdfread.Reader, p *Page, u Use) (Action, string) {
	im := u.Image
	switch {
	case !p.IsScan(u):
		return None, ""
	case !d.Scanned():
		return Keep, "document mostly not scanned"
	case u.Inline:
		return Keep, "inline image"
	case im.UsedAsMask:
		return Keep, "used as a mask"
	case im.SMask != 0:
		return Keep, "has a soft mask"
	case im.Mask != nil:
		return Keep, "has a mask"
	case im.Enc == "jbig2" || im.Enc == "jpx":
		return Keep, "already " + im.Enc
	case im.Enc == "ccitt" && ccittK(r, im.Dict) > 0:
		return Keep, "CCITT G3 2-D"
	case im.BPC != 1 && im.BPC != 2 && im.BPC != 4 && im.BPC != 8:
		return Keep, fmt.Sprintf("%d bits per component", im.BPC)
	case im.Comps == 0 || im.Color == "lab" || im.Color == "sep" || im.Color == "devn" || im.Color == "?":
		return Keep, "colour space " + im.Color
	case im.Kind == Bilevel:
		return JBIG2, ""
	case p.TextOver > 0:
		return Keep, "text drawn over it"
	}
	return Separate, ""
}

// ccittK returns the /K of a CCITT image's parameters.
func ccittK(r *pdfread.Reader, d pdf.Dict) int {
	f := r.Resolve(d["Filter"])
	p := r.Resolve(d["DecodeParms"])
	if a, ok := f.(pdf.Array); ok {
		pa, _ := p.(pdf.Array)
		p = nil
		if n := len(a); n > 0 && n <= len(pa) {
			p = r.Resolve(pa[n-1])
		}
	}
	pd, _ := p.(pdf.Dict)
	k, _ := r.Resolve(pd["K"]).(int)
	return k
}

// Dump prints one line per page: the scan images found and what would
// be done with them, or what else is drawn.
func Dump(w io.Writer, r *pdfread.Reader, d *Document) {
	scanned := 0
	for _, p := range d.Pages {
		if p.Scanned {
			scanned++
		}
	}
	kind := "born-digital"
	if d.Scanned() {
		kind = "scanned"
	}
	fmt.Fprintf(w, "%d pages, %d scanned: %s\n", len(d.Pages), scanned, kind)
	for i := range d.Pages {
		p := &d.Pages[i]
		var parts []string
		others := 0
		for _, u := range p.Uses {
			a, why := d.Plan(r, p, u)
			if a == None {
				others++
				continue
			}
			s := describe(u)
			if a == Keep {
				s += ": keep, " + why
			} else {
				s += ": " + a.String()
			}
			parts = append(parts, s)
		}
		line := strings.Join(parts, "; ")
		switch {
		case len(parts) == 0 && others == 0:
			line = "no images"
		case len(parts) == 0:
			line = fmt.Sprintf("%d images, not a scan", others)
		case others > 0:
			line += fmt.Sprintf("; %d other images", others)
		}
		fmt.Fprintf(w, "page %d: %s\n", p.Num, line)
	}
}

// describe tells an image use: size, colour, bits, encoding, object,
// resolution and coverage.
func describe(u Use) string {
	im := u.Image
	obj := "inline"
	if !u.Inline {
		obj = fmt.Sprintf("#%d", im.Ref)
	}
	color := im.Color
	if im.Stencil {
		color = "stencil"
	}
	s := fmt.Sprintf("%dx%d %s %d-bit %s %s", im.Width, im.Height, color, im.BPC, im.Enc, obj)
	if im.Decode {
		s += " decode"
	}
	x, y := math.Round(u.DPIX), math.Round(u.DPIY)
	if x == y {
		s += fmt.Sprintf(" %.0f dpi", x)
	} else {
		s += fmt.Sprintf(" %.0fx%.0f dpi", x, y)
	}
	return s + fmt.Sprintf(" %.0f%%", 100*u.Coverage)
}
