// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package pdfread

import (
	"errors"

	"github.com/sergev/dejapdf/internal/pdf"
)

// Page is a leaf of the page tree with its inherited attributes
// (7.7.3.4) resolved.
type Page struct {
	Ref       pdf.Ref // 0 if the page is a direct object
	Dict      pdf.Dict
	Resources pdf.Dict
	MediaBox  [4]float64
	CropBox   [4]float64 // MediaBox if absent
	Rotate    int        // 0, 90, 180 or 270
}

// letter is the MediaBox of a page that has none.
var letter = [4]float64{0, 0, 612, 792}

// Pages returns the pages in order.
func (r *Reader) Pages() ([]Page, error) {
	root, ok := r.Resolve(r.Catalog()["Pages"]).(pdf.Dict)
	if !ok {
		return nil, errors.New("pdf: no page tree")
	}
	var pages []Page
	seen := map[pdf.Ref]bool{}
	type inherit struct {
		res         pdf.Dict
		media, crop any
		rotate      any
	}
	var walk func(ref pdf.Ref, d pdf.Dict, in inherit, depth int)
	walk = func(ref pdf.Ref, d pdf.Dict, in inherit, depth int) {
		if depth > maxDepth {
			return
		}
		if res, ok := r.Resolve(d["Resources"]).(pdf.Dict); ok {
			in.res = res
		}
		if v := r.Resolve(d["MediaBox"]); v != nil {
			in.media = v
		}
		if v := r.Resolve(d["CropBox"]); v != nil {
			in.crop = v
		}
		if v := r.Resolve(d["Rotate"]); v != nil {
			in.rotate = v
		}
		kids, isNode := r.Resolve(d["Kids"]).(pdf.Array)
		if !isNode || d["Type"] == pdf.Name("Page") {
			p := Page{Ref: ref, Dict: d, Resources: in.res}
			var ok bool
			if p.MediaBox, ok = rect(r, in.media); !ok {
				p.MediaBox = letter
			}
			if p.CropBox, ok = rect(r, in.crop); !ok {
				p.CropBox = p.MediaBox
			}
			rot := 0
			switch v := in.rotate.(type) {
			case int:
				rot = v
			case float64:
				rot = int(v)
			}
			p.Rotate = (rot%360 + 360) % 360 / 90 * 90
			pages = append(pages, p)
			return
		}
		for _, k := range kids {
			kref, _ := k.(pdf.Ref)
			if kref != 0 {
				if seen[kref] {
					continue
				}
				seen[kref] = true
			}
			kd, ok := r.Resolve(k).(pdf.Dict)
			if !ok {
				continue
			}
			walk(kref, kd, in, depth+1)
		}
	}
	rootRef, _ := r.Catalog()["Pages"].(pdf.Ref)
	seen[rootRef] = true
	walk(rootRef, root, inherit{}, 0)
	return pages, nil
}

// rect reads a rectangle, normalised so that x0 <= x1, y0 <= y1.
func rect(r *Reader, v any) ([4]float64, bool) {
	var b [4]float64
	a, ok := r.Resolve(v).(pdf.Array)
	if !ok || len(a) != 4 {
		return b, false
	}
	for i := range b {
		switch x := r.Resolve(a[i]).(type) {
		case int:
			b[i] = float64(x)
		case float64:
			b[i] = x
		default:
			return b, false
		}
	}
	if b[0] > b[2] {
		b[0], b[2] = b[2], b[0]
	}
	if b[1] > b[3] {
		b[1], b[3] = b[3], b[1]
	}
	return b, true
}
