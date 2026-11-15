// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package reencode

import (
	"fmt"

	"github.com/sergev/dejapdf/internal/jb2"
	"github.com/sergev/dejapdf/internal/pdf"
	"github.com/sergev/dejapdf/internal/pdfread"
	"github.com/sergev/dejapdf/internal/raster"
	"github.com/sergev/dejapdf/internal/rewrite"
	"github.com/sergev/dejapdf/internal/scan"
	"github.com/sergev/dejapdf/internal/segment"
	"github.com/sergev/dejapdf/internal/shapes"
	"github.com/sergev/dejapdf/internal/stencil"
)

// separated is a gray or colour image split into layers.
type separated struct {
	im   *scan.Image
	dpi  float64
	raw  int // size of the original stream
	j    *jb2.Image
	fg   *segment.Foreground
	bg   *layer
	form *rewrite.Stream
	size int // the form's streams, globals left out
	glob *stencil.Globals
	err  error
}

// separate re-encodes gray and colour images as a form XObject in their
// place: the background, then the text in each colour as a JBIG2
// stencil. The stencils of a group share one dictionary.
func separate(r *pdfread.Reader, ims []*scan.Image, dpi map[*scan.Image]float64, repl map[int]*rewrite.Stream,
	st *Stats, opt *Options) {
	q := opt.Quality
	if q <= 0 {
		q = DefaultQuality
	}
	ss := make([]*separated, len(ims))
	parallel(len(ims), func(i int) {
		s := &separated{im: ims[i], dpi: dpi[ims[i]]}
		ss[i] = s
		if o, ok := r.Get(int(s.im.Ref)).(*pdfread.Stream); ok {
			s.raw = len(o.Raw)
		}
		v, err := raster.Decode(r, s.im)
		if err != nil {
			s.err = err
			return
		}
		p, ok := v.(*raster.Pixmap)
		if !ok {
			s.err = fmt.Errorf("not gray or colour")
			return
		}
		m := segment.Mask(p, s.dpi)
		s.j = shapes.Split(m)
		s.fg = segment.Colors(p, s.j)
		s.bg, s.err = background(p, m, s.dpi, int(q*float64(s.raw)))
	})
	var js []*jb2.Image
	for _, s := range ss {
		if s.err == nil {
			js = append(js, s.j)
		}
	}
	share(js, opt.Lossy)
	cache := &stencil.Cache{}
	parallel(len(ss), func(i int) {
		if s := ss[i]; s.err == nil {
			s.layout(cache)
			s.j = nil
		}
	})

	var keep []*separated
	var before, after int64
	var glob *stencil.Globals
	for _, s := range ss {
		if s.err != nil {
			if opt.Warn != nil {
				opt.Warn(fmt.Errorf("object %d: %w", s.im.Ref, s.err))
			}
			continue
		}
		if s.size >= s.raw {
			continue
		}
		keep = append(keep, s)
		before += int64(s.raw)
		after += int64(s.size)
		if s.glob != nil {
			glob = s.glob
		}
	}
	var gs *rewrite.Stream
	if glob != nil {
		gs = &rewrite.Stream{Dict: pdf.Dict{}, Data: glob.Data()}
		after += int64(len(gs.Data))
	}
	if after >= before {
		return
	}
	for _, s := range keep {
		setGlobals(s.form, gs)
		repl[int(s.im.Ref)] = s.form
	}
	st.Replaced += len(keep)
	st.Before += before
	st.After += after
}

// inset moves the edges of a cropped stencil inside their device
// pixels, in the form's units: viewers snap an edge on a pixel
// boundary either way (docs/Portable_Document_Format.md §8).
const inset = 1e-5

// layout builds the form: the unit square is the image, as when it was
// drawn.
func (s *separated) layout(cache *stencil.Cache) {
	w, h := float64(s.im.Width), float64(s.im.Height)
	xobj := pdf.Dict{}
	var content []byte
	size := 0
	if s.bg.uniform {
		content = append(fill(content, s.bg.fill), "0 0 1 1 re f\n"...)
	} else {
		// The layer is the image reduced by k, rounded up: it may
		// overhang the bottom and right edges, which the BBox clips.
		k := s.bg.k
		bw, bh := float64(s.bg.dict["Width"].(int)*k)/w, float64(s.bg.dict["Height"].(int)*k)/h
		xobj["Bg"] = &rewrite.Stream{Dict: s.bg.dict, Data: s.bg.data}
		content = fmt.Appendf(content, "q %s 0 0 %s 0 %s cm /Bg Do Q\n", num(bw), num(bh), num(1-bh))
		size += len(s.bg.data)
	}
	// One stencil per colour, cropped to its blits.
	j := s.j
	box := make([][4]int, len(s.fg.Palette)) // x0, y0, x1, y1 from the bottom left
	for k := range box {
		box[k] = [4]int{j.W, j.H, 0, 0}
	}
	for i, bl := range j.Blits {
		sh := j.Shape(bl.Shape)
		if sh.W == 0 || sh.H == 0 {
			continue
		}
		b := &box[s.fg.Index[i]]
		b[0], b[1] = min(b[0], max(bl.X, 0)), min(b[1], max(bl.Y, 0))
		b[2], b[3] = max(b[2], min(bl.X+sh.W, j.W)), max(b[3], min(bl.Y+sh.H, j.H))
	}
	for k, c := range s.fg.Palette {
		b := box[k]
		if b[2] <= b[0] || b[3] <= b[1] {
			continue
		}
		keep := func(i int) bool { return s.fg.Index[i] == k }
		d, data := stencil.Code(j, b[0], b[1], b[2]-b[0], b[3]-b[1], keep, cache)
		if d == nil {
			continue
		}
		if p, ok := d["DecodeParms"].(pdf.Dict); ok {
			s.glob = p["JBIG2Globals"].(*stencil.Globals)
		}
		name := fmt.Sprintf("S%d", k)
		xobj[name] = &rewrite.Stream{Dict: d, Data: data}
		size += len(data)
		x0, y0 := float64(b[0])/w+inset, float64(b[1])/h+inset
		sw, sh := float64(b[2]-b[0])/w-2*inset, float64(b[3]-b[1])/h-2*inset
		content = fill(append(content, "q "...), c)
		content = fmt.Appendf(content, "%s 0 0 %s %s %s cm /%s Do Q\n", num(sw), num(sh), num(x0), num(y0), name)
	}
	d := pdf.Dict{
		"Type":    pdf.Name("XObject"),
		"Subtype": pdf.Name("Form"),
		"BBox":    pdf.Array{0, 0, 1, 1},
	}
	if len(xobj) > 0 {
		d["Resources"] = pdf.Dict{"XObject": xobj}
	}
	if oc, ok := s.im.Dict["OC"]; ok {
		d["OC"] = oc
	}
	s.form = &rewrite.Stream{Dict: d, Data: content}
	s.size = size + len(content) + 60*len(xobj)
}

// setGlobals points the form's stencils at the group's globals.
func setGlobals(form *rewrite.Stream, gs *rewrite.Stream) {
	res, _ := form.Dict["Resources"].(pdf.Dict)
	xobj, _ := res["XObject"].(pdf.Dict)
	for _, v := range xobj {
		if p, ok := v.(*rewrite.Stream).Dict["DecodeParms"].(pdf.Dict); ok {
			p["JBIG2Globals"] = gs
		}
	}
}

// fill sets the non-stroking colour.
func fill(b []byte, c segment.Color) []byte {
	if c[0] == c[1] && c[1] == c[2] {
		return fmt.Appendf(b, "%s g ", channel(c[0]))
	}
	return fmt.Appendf(b, "%s %s %s rg ", channel(c[0]), channel(c[1]), channel(c[2]))
}

func channel(v byte) string { return num(float64(v) / 255) }

// num formats a coordinate.
func num(f float64) string { return string(pdf.AppendReal(nil, f)) }
