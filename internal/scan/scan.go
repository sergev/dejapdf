// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

// Package scan finds the scan images of a PDF: images that cover most
// of a page. It follows each page's content through form XObjects,
// records every image drawn with its size on the page, classifies it,
// and decides what can be done with it.
package scan

import (
	"math"
	"strings"

	"github.com/sergev/dejapdf/internal/pdf"
	"github.com/sergev/dejapdf/internal/pdfread"
)

// Kind is the kind of pixels.
type Kind int

const (
	Bilevel Kind = iota // 1 bit, or a stencil mask
	Gray
	Colour
)

func (k Kind) String() string { return [...]string{"bilevel", "gray", "colour"}[k] }

// Image is an image XObject, or an inline image.
type Image struct {
	Ref           pdf.Ref // 0 for an inline image
	Dict          pdf.Dict
	Width, Height int
	BPC           int
	Stencil       bool   // /ImageMask
	Color         string // as pdfimages: gray, rgb, cmyk, index, icc, lab, sep, devn
	Comps         int
	Enc           string // as pdfimages: image, jpeg, jpx, jbig2, ccitt
	Kind          Kind
	Decode        bool // a /Decode array
	SMask         pdf.Ref
	Mask          any  // /Mask: a stream reference or a colour-key array
	UsedAsMask    bool // the /SMask or /Mask of another image
	Pages         []int
}

// Use is one drawing of an image on a page.
type Use struct {
	Image    *Image
	Matrix   [6]float64 // the CTM: the unit square to user space
	Coverage float64    // fraction of the crop box covered
	DPIX     float64
	DPIY     float64
	Inline   bool
	Annot    bool // in an annotation's appearance
	SoftMask bool // in the group of a soft mask
}

// Page is what is drawn on one page.
type Page struct {
	Num      int // from 1
	Uses     []Use
	Text     int // visible text strings shown
	TextOver int // of them, after an image covering part of the page
	Scanned  bool
}

// Document is the result of Find.
type Document struct {
	Pages  []Page
	Images map[pdf.Ref]*Image
}

// minScan is the part of the page that scan images cover together;
// minPart is the least an image covers to count.
const (
	minScan = 0.7
	minPart = 0.05
)

// Find walks all pages of r.
func Find(r *pdfread.Reader) (*Document, error) {
	return FindStep(r, nil)
}

// FindStep is Find calling step, if not nil, after each page.
func FindStep(r *pdfread.Reader, step func(done, all int)) (*Document, error) {
	pages, err := r.Pages()
	if err != nil {
		return nil, err
	}
	doc := &Document{Images: map[pdf.Ref]*Image{}}
	masks := map[pdf.Ref]bool{}
	for i, p := range pages {
		w := &walker{r: r, doc: doc, page: Page{Num: i + 1}, crop: p.CropBox, masks: masks}
		w.content(r.Resolve(p.Dict["Contents"]), p.Resources, identity, 0)
		w.annots(p.Dict)
		pg := w.page
		total := 0.0
		for _, u := range pg.Uses {
			if u.Coverage >= minPart && !u.Annot && !u.SoftMask {
				total += u.Coverage
			}
		}
		pg.Scanned = total >= minScan
		doc.Pages = append(doc.Pages, pg)
		if step != nil {
			step(i+1, len(pages))
		}
	}
	for ref := range masks {
		if im := doc.Images[ref]; im != nil {
			im.UsedAsMask = true
		}
	}
	return doc, nil
}

// IsScan reports whether a use is part of the scan of its page.
func (p *Page) IsScan(u Use) bool {
	return p.Scanned && u.Coverage >= minPart && !u.Annot && !u.SoftMask
}

var identity = [6]float64{1, 0, 0, 1, 0, 0}

// mul returns m × n: m applied first.
func mul(m, n [6]float64) [6]float64 {
	return [6]float64{
		m[0]*n[0] + m[1]*n[2],
		m[0]*n[1] + m[1]*n[3],
		m[2]*n[0] + m[3]*n[2],
		m[2]*n[1] + m[3]*n[3],
		m[4]*n[0] + m[5]*n[2] + n[4],
		m[4]*n[1] + m[5]*n[3] + n[5],
	}
}

type walker struct {
	r     *pdfread.Reader
	doc   *Document
	page  Page
	crop  [4]float64
	masks map[pdf.Ref]bool
	forms []pdf.Ref // the forms being drawn, against cycles
	annot bool      // drawing an annotation's appearance
	smask bool      // drawing a soft mask
	big   bool      // an image covering part of the page was drawn
}

// maxForms limits the nesting of form XObjects.
const maxForms = 20

// content draws a content stream (or an array of them) with resources
// res under ctm.
func (w *walker) content(c any, res pdf.Dict, ctm [6]float64, depth int) {
	var data []byte
	switch c := c.(type) {
	case *pdfread.Stream:
		data, _ = w.r.Decode(c)
	case pdf.Array:
		for _, s := range c {
			if s, ok := w.r.Resolve(s).(*pdfread.Stream); ok {
				d, _ := w.r.Decode(s)
				data = append(append(data, d...), '\n')
			}
		}
	}
	if len(data) == 0 {
		return
	}
	// The graphics state we track: the CTM and the text render mode.
	type state struct {
		ctm  [6]float64
		mode int
	}
	var stack []state
	mode := 0
	pdfread.Content(data, func(op string, args []any) bool {
		switch op {
		case "q":
			if len(stack) < 1000 {
				stack = append(stack, state{ctm, mode})
			}
		case "Q":
			if n := len(stack); n > 0 {
				ctm, mode = stack[n-1].ctm, stack[n-1].mode
				stack = stack[:n-1]
			}
		case "cm":
			if m, ok := matrix(args); ok {
				ctm = mul(m, ctm)
			}
		case "Do":
			if len(args) == 1 {
				if name, ok := args[0].(pdf.Name); ok {
					w.do(name, res, ctm, depth)
				}
			}
		case "BI":
			if im, ok := args[0].(*pdfread.InlineImage); ok {
				w.inline(im, res, ctm)
			}
		case "Tr":
			if len(args) == 1 {
				if n, ok := args[0].(int); ok {
					mode = n
				}
			}
		case "Tj", "TJ", "'", "\"":
			// Text that shows: not render mode 3 (invisible) or 7 (clip).
			if mode != 3 && mode != 7 && !w.smask && !w.annot {
				w.page.Text++
				if w.big {
					w.page.TextOver++
				}
			}
		case "gs":
			if len(args) == 1 {
				if name, ok := args[0].(pdf.Name); ok {
					w.softMask(name, res, ctm, depth)
				}
			}
		}
		return true
	})
}

// matrix reads six numbers.
func matrix(args []any) ([6]float64, bool) {
	var m [6]float64
	if len(args) != 6 {
		return m, false
	}
	for i, a := range args {
		switch a := a.(type) {
		case int:
			m[i] = float64(a)
		case float64:
			m[i] = a
		default:
			return m, false
		}
	}
	return m, true
}

// softMask draws the group of a soft mask set by gs (11.6.5.2), in
// the coordinates in effect at gs; what it draws is a mask.
func (w *walker) softMask(name pdf.Name, res pdf.Dict, ctm [6]float64, depth int) {
	egs, _ := w.r.Resolve(res["ExtGState"]).(pdf.Dict)
	gs, _ := w.r.Resolve(egs[string(name)]).(pdf.Dict)
	sm, _ := w.r.Resolve(gs["SMask"]).(pdf.Dict)
	g, _ := sm["G"].(pdf.Ref)
	if g == 0 || depth >= maxForms || w.smask {
		return
	}
	w.smask = true
	w.form(g, res, ctm, depth)
	w.smask = false
}

func (w *walker) do(name pdf.Name, res pdf.Dict, ctm [6]float64, depth int) {
	xo, _ := w.r.Resolve(res["XObject"]).(pdf.Dict)
	ref, _ := xo[string(name)].(pdf.Ref)
	s, ok := w.r.Resolve(xo[string(name)]).(*pdfread.Stream)
	if !ok {
		return
	}
	switch s.Dict["Subtype"] {
	case pdf.Name("Image"):
		im := w.doc.Images[ref]
		if im == nil || ref == 0 {
			im = w.image(s.Dict, res)
			im.Ref = ref
			if ref != 0 {
				w.doc.Images[ref] = im
			}
		}
		w.use(im, ctm, false)
	case pdf.Name("Form"):
		if ref != 0 {
			w.form(ref, res, ctm, depth)
		}
	}
}

// form draws a form XObject.
func (w *walker) form(ref pdf.Ref, res pdf.Dict, ctm [6]float64, depth int) {
	s, ok := w.r.Resolve(ref).(*pdfread.Stream)
	if !ok || depth >= maxForms {
		return
	}
	for _, f := range w.forms {
		if f == ref && ref != 0 {
			return
		}
	}
	w.forms = append(w.forms, ref)
	if m, ok := matrix(arr(w.r, s.Dict["Matrix"])); ok {
		ctm = mul(m, ctm)
	}
	// A form without resources uses its parent's (7.8.3).
	if fr, ok := w.r.Resolve(s.Dict["Resources"]).(pdf.Dict); ok {
		res = fr
	}
	w.content(s, res, ctm, depth+1)
	w.forms = w.forms[:len(w.forms)-1]
}

func arr(r *pdfread.Reader, v any) []any {
	a, _ := r.Resolve(v).(pdf.Array)
	out := make([]any, len(a))
	for i, x := range a {
		out[i] = r.Resolve(x)
	}
	return out
}

// annots draws the normal appearances of a page's annotations, as
// viewers do (12.5.5): the form's bounding box, transformed by its
// matrix, is fitted to the annotation's rectangle.
func (w *walker) annots(page pdf.Dict) {
	for _, a := range arr(w.r, page["Annots"]) {
		ad, ok := a.(pdf.Dict)
		if !ok {
			continue
		}
		if f, _ := w.r.Resolve(ad["F"]).(int); f&2 != 0 { // hidden
			continue
		}
		ap, _ := w.r.Resolve(ad["AP"]).(pdf.Dict)
		n := w.r.Resolve(ap["N"])
		if d, ok := n.(pdf.Dict); ok {
			// Appearance states: the one in /AS.
			as, _ := w.r.Resolve(ad["AS"]).(pdf.Name)
			n = w.r.Resolve(d[string(as)])
		}
		s, ok := n.(*pdfread.Stream)
		if !ok {
			continue
		}
		rect, ok1 := matrix(append(arr(w.r, ad["Rect"]), 0, 0))
		bbox, ok2 := matrix(append(arr(w.r, s.Dict["BBox"]), 0, 0))
		if !ok1 || !ok2 {
			continue
		}
		m := identity
		if mm, ok := matrix(arr(w.r, s.Dict["Matrix"])); ok {
			m = mm
		}
		// The transformed bounding box, then a map onto the rectangle.
		x0, y0, x1, y1 := bounds(m, bbox[0], bbox[1], bbox[2], bbox[3])
		if x1-x0 == 0 || y1-y0 == 0 {
			continue
		}
		sx := (math.Max(rect[0], rect[2]) - math.Min(rect[0], rect[2])) / (x1 - x0)
		sy := (math.Max(rect[1], rect[3]) - math.Min(rect[1], rect[3])) / (y1 - y0)
		fit := [6]float64{sx, 0, 0, sy, math.Min(rect[0], rect[2]) - x0*sx, math.Min(rect[1], rect[3]) - y0*sy}
		ctm := mul(m, fit)
		res, _ := w.r.Resolve(s.Dict["Resources"]).(pdf.Dict)
		w.annot = true
		w.content(s, res, ctm, 1)
		w.annot = false
	}
}

// bounds is the bounding box of a rectangle transformed by m.
func bounds(m [6]float64, x0, y0, x1, y1 float64) (float64, float64, float64, float64) {
	minx, miny := math.Inf(1), math.Inf(1)
	maxx, maxy := math.Inf(-1), math.Inf(-1)
	for _, p := range [][2]float64{{x0, y0}, {x1, y0}, {x0, y1}, {x1, y1}} {
		x := m[0]*p[0] + m[2]*p[1] + m[4]
		y := m[1]*p[0] + m[3]*p[1] + m[5]
		minx, maxx = math.Min(minx, x), math.Max(maxx, x)
		miny, maxy = math.Min(miny, y), math.Max(maxy, y)
	}
	return minx, miny, maxx, maxy
}

// use records a drawing of im under ctm.
func (w *walker) use(im *Image, ctm [6]float64, inline bool) {
	u := Use{Image: im, Matrix: ctm, Inline: inline, Annot: w.annot, SoftMask: w.smask}
	if w.smask && im.Ref != 0 {
		w.masks[im.Ref] = true
	}
	x0, y0, x1, y1 := bounds(ctm, 0, 0, 1, 1)
	c := w.crop
	ix := math.Min(x1, c[2]) - math.Max(x0, c[0])
	iy := math.Min(y1, c[3]) - math.Max(y0, c[1])
	if area := (c[2] - c[0]) * (c[3] - c[1]); ix > 0 && iy > 0 && area > 0 {
		u.Coverage = ix * iy / area
	}
	// Pixels per inch along the image's own axes.
	if l := math.Hypot(ctm[0], ctm[1]); l > 0 {
		u.DPIX = float64(im.Width) * 72 / l
	}
	if l := math.Hypot(ctm[2], ctm[3]); l > 0 {
		u.DPIY = float64(im.Height) * 72 / l
	}
	w.page.Uses = append(w.page.Uses, u)
	if u.Coverage >= minPart && !w.annot && !w.smask {
		w.big = true
	}
	if n := len(im.Pages); n == 0 || im.Pages[n-1] != w.page.Num {
		im.Pages = append(im.Pages, w.page.Num)
	}
	if im.SMask != 0 {
		w.masks[im.SMask] = true
	}
	if m, ok := im.Mask.(pdf.Ref); ok {
		w.masks[m] = true
	}
}

// inline records an inline image, its keys expanded.
func (w *walker) inline(ii *pdfread.InlineImage, res pdf.Dict, ctm [6]float64) {
	d := pdf.Dict{}
	for k, v := range ii.Dict {
		if full, ok := inlineKeys[k]; ok {
			k = full
		}
		d[k] = v
	}
	im := w.image(d, res)
	w.use(im, ctm, true)
}

var inlineKeys = map[string]string{
	"BPC": "BitsPerComponent", "CS": "ColorSpace", "D": "Decode", "DP": "DecodeParms",
	"F": "Filter", "H": "Height", "IM": "ImageMask", "I": "Interpolate", "W": "Width",
}

var inlineNames = map[pdf.Name]pdf.Name{
	"G": "DeviceGray", "RGB": "DeviceRGB", "CMYK": "DeviceCMYK", "I": "Indexed",
	"AHx": "ASCIIHexDecode", "A85": "ASCII85Decode", "LZW": "LZWDecode", "Fl": "FlateDecode",
	"RL": "RunLengthDecode", "CCF": "CCITTFaxDecode", "DCT": "DCTDecode",
}

// image describes an image from its dictionary; res resolves the named
// colour spaces of inline images.
func (w *walker) image(d pdf.Dict, res pdf.Dict) *Image {
	r := w.r
	num := func(k string) int { n, _ := r.Resolve(d[k]).(int); return n }
	im := &Image{Dict: d, Width: num("Width"), Height: num("Height"), BPC: num("BitsPerComponent")}
	im.Stencil, _ = r.Resolve(d["ImageMask"]).(bool)
	im.Decode = d["Decode"] != nil
	im.SMask, _ = d["SMask"].(pdf.Ref)
	im.Mask = d["Mask"]
	if im.Stencil {
		im.BPC = 1
	}
	// The last filter decides the encoding.
	f := r.Resolve(d["Filter"])
	if a, ok := f.(pdf.Array); ok && len(a) > 0 {
		f = r.Resolve(a[len(a)-1])
	}
	if n, ok := f.(pdf.Name); ok {
		if full, ok := inlineNames[n]; ok {
			n = full
		}
		switch n {
		case "DCTDecode":
			im.Enc = "jpeg"
		case "JPXDecode":
			im.Enc = "jpx"
		case "JBIG2Decode":
			im.Enc = "jbig2"
		case "CCITTFaxDecode":
			im.Enc = "ccitt"
		}
	}
	if im.Enc == "" {
		im.Enc = "image"
	}
	cs := r.Resolve(d["ColorSpace"])
	if n, ok := cs.(pdf.Name); ok && res != nil {
		named, _ := r.Resolve(res["ColorSpace"]).(pdf.Dict)
		if full, ok := inlineNames[n]; ok {
			n = full
		}
		if named[string(n)] != nil {
			cs = r.Resolve(named[string(n)])
		} else if def := named["Default"+strings.TrimPrefix(string(n), "Device")]; def != nil && strings.HasPrefix(string(n), "Device") {
			// DefaultGray, DefaultRGB, DefaultCMYK (8.6.5.6).
			cs = r.Resolve(def)
		}
	}
	im.Color, im.Comps = colorSpace(r, cs, 0)
	if im.Stencil {
		im.Color, im.Comps = "-", 1
	}
	if im.Enc == "jpx" && cs == nil {
		im.Color = "-"
	}
	switch {
	case im.Stencil || im.BPC == 1 && im.Comps == 1:
		im.Kind = Bilevel
	case im.Comps == 1 && im.Color != "index":
		im.Kind = Gray
	default:
		im.Kind = Colour
	}
	return im
}

// colorSpace names a colour space as pdfimages does, with its number
// of components.
func colorSpace(r *pdfread.Reader, cs any, depth int) (string, int) {
	if depth > 5 {
		return "?", 0
	}
	switch cs := cs.(type) {
	case pdf.Name:
		switch cs {
		case "DeviceGray", "G", "CalGray":
			return "gray", 1
		case "DeviceRGB", "RGB", "CalRGB":
			return "rgb", 3
		case "DeviceCMYK", "CMYK":
			return "cmyk", 4
		}
	case pdf.Array:
		if len(cs) == 0 {
			break
		}
		name, _ := r.Resolve(cs[0]).(pdf.Name)
		switch name {
		case "CalGray":
			return "gray", 1
		case "CalRGB":
			return "rgb", 3
		case "Lab":
			return "lab", 3
		case "Indexed", "I":
			return "index", 1
		case "Separation":
			return "sep", 1
		case "DeviceN":
			return "devn", len(arr(r, cs[min(1, len(cs)-1)]))
		case "ICCBased":
			if len(cs) > 1 {
				if s, ok := r.Resolve(cs[1]).(*pdfread.Stream); ok {
					n, _ := r.Resolve(s.Dict["N"]).(int)
					return "icc", n
				}
			}
			return "icc", 0
		default:
			if len(cs) == 1 {
				return colorSpace(r, name, depth+1)
			}
		}
	}
	return "?", 0
}
