// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package convert

import (
	"fmt"
	"io"
	"runtime"

	"github.com/sergev/dejapdf/internal/djvu"
	"github.com/sergev/dejapdf/internal/iw44"
	"github.com/sergev/dejapdf/internal/jb2"
	"github.com/sergev/dejapdf/internal/jbig2"
	jpxenc "github.com/sergev/dejapdf/internal/jpx"
	"github.com/sergev/dejapdf/internal/pdf"
)

// Options control the conversion.
type Options struct {
	Producer string  // document info /Producer
	Quality  float64 // JPX size as a multiple of the IW44 size; 0 means 1
	MaskOnly bool    // paint only the mask, in black
}

// Convert writes doc as PDF to w.
func Convert(doc *djvu.Document, w io.Writer, opt Options) error {
	if opt.Quality <= 0 {
		opt.Quality = 1
	}
	p := pdf.NewWriter(w)
	// Pages are built in parallel and written in order.
	type result struct {
		b   *pageBuilder
		err error
	}
	results := make([]chan result, len(doc.Pages))
	for i := range results {
		results[i] = make(chan result, 1)
	}
	window := make(chan struct{}, 2*runtime.GOMAXPROCS(0))
	go func() {
		for i, pg := range doc.Pages {
			window <- struct{}{}
			go func(c chan result) {
				b, err := build(doc, pg, &opt)
				c <- result{b, err}
			}(results[i])
		}
	}()
	var err error
	for i := range doc.Pages {
		r := <-results[i]
		if err == nil && r.err != nil {
			err = fmt.Errorf("page %d: %w", i+1, r.err)
		}
		if err == nil {
			r.b.emit(p)
		}
		<-window
	}
	if err != nil {
		return err
	}
	info := pdf.Dict{}
	if opt.Producer != "" {
		info["Producer"] = pdf.TextString(opt.Producer)
	}
	if len(doc.Pages) > 0 {
		if t := title(doc.Pages[0]); t != "" {
			info["Title"] = pdf.TextString(t)
		}
	}
	return p.Close(nil, info)
}

// pageBuilder collects a page's content and objects.
type pageBuilder struct {
	in      djvu.Info
	w, h    float64 // page size in points
	objs    []object
	xobj    pdf.Dict // name to local
	content []byte
}

// object is a stream to write; local values in its dictionary refer to
// other objects of the page.
type object struct {
	dict pdf.Dict
	data []byte
}

// local is the index of an object of the page being built.
type local int

// obj adds an object.
func (b *pageBuilder) obj(d pdf.Dict, data []byte) local {
	b.objs = append(b.objs, object{d, data})
	return local(len(b.objs) - 1)
}

// emit writes the page and its objects.
func (b *pageBuilder) emit(p *pdf.Writer) {
	refs := make([]pdf.Ref, len(b.objs))
	for i := range refs {
		refs[i] = p.Alloc()
	}
	resolve := func(d pdf.Dict) {
		for k, v := range d {
			if l, ok := v.(local); ok {
				d[k] = refs[l]
			}
		}
	}
	for i, o := range b.objs {
		resolve(o.dict)
		p.Stream(refs[i], o.dict, o.data)
	}
	resolve(b.xobj)
	res := pdf.Dict{}
	if len(b.xobj) > 0 {
		res["XObject"] = b.xobj
	}
	p.AddPage(pdf.Page{Width: b.w, Height: b.h, Rotate: b.in.Rotate, Resources: res, Content: b.content})
}

// draw paints XObject name over the area a layer of iw×ih pixels covers.
// A layer reduced k times is ⌈W/k⌉×⌈H/k⌉ and anchored at the bottom left,
// so it may stick out at the top and right; other sizes fill the page.
func (b *pageBuilder) draw(name string, iw, ih int) {
	w, h := b.w, b.h
	if k := reduction(b.in.Width, b.in.Height, iw, ih); k > 0 {
		s := float64(k) * 72 / float64(b.in.DPI)
		w, h = float64(iw)*s, float64(ih)*s
	}
	b.content = fmt.Appendf(b.content, "q %s 0 0 %s 0 0 cm /%s Do Q\n", num(w), num(h), name)
}

// reduction returns k if iw×ih is the page reduced k times, else 0.
func reduction(pw, ph, iw, ih int) int {
	for k := 1; k <= 12; k++ {
		if (pw+k-1)/k == iw && (ph+k-1)/k == ih {
			return k
		}
	}
	return 0
}

// add registers an XObject under a fresh name.
func (b *pageBuilder) add(prefix string, r local) string {
	name := fmt.Sprintf("%s%d", prefix, len(b.xobj))
	b.xobj[name] = r
	return name
}

// fill sets the non-stroking colour.
func (b *pageBuilder) fill(c [3]byte) {
	if c[0] == c[1] && c[1] == c[2] {
		b.content = fmt.Appendf(b.content, "%s g\n", channel(c[0]))
		return
	}
	b.content = fmt.Appendf(b.content, "%s %s %s rg\n", channel(c[0]), channel(c[1]), channel(c[2]))
}

func channel(v byte) string { return num(float64(v) / 255) }

// build prepares a page.
func build(doc *djvu.Document, pg *djvu.Page, opt *Options) (*pageBuilder, error) {
	in := pg.Info
	if in.Width <= 0 || in.Height <= 0 {
		return nil, fmt.Errorf("bad size %dx%d", in.Width, in.Height)
	}
	b := &pageBuilder{in: in, xobj: pdf.Dict{},
		w: float64(in.Width) * 72 / float64(in.DPI),
		h: float64(in.Height) * 72 / float64(in.DPI)}
	mask, err := doc.Mask(pg)
	if err != nil {
		return nil, err
	}
	if !opt.MaskOnly {
		if err := background(b, pg, opt); err != nil {
			return nil, err
		}
	}
	if mask != nil {
		if err := foreground(b, pg, mask, opt); err != nil {
			return nil, err
		}
	} else if c := pg.Form.Find("Smmr"); c != nil {
		if err := mmrForeground(b, pg, c.Data, opt); err != nil {
			return nil, err
		}
	}
	return b, nil
}

// background paints BG44 or BGjp.
func background(b *pageBuilder, pg *djvu.Page, opt *Options) error {
	if c := pg.Form.Find("BGjp"); c != nil && pg.Form.Find("BG44") == nil {
		d, w, h, err := jpeg(c.Data)
		if err != nil {
			return fmt.Errorf("BGjp: %w", err)
		}
		b.draw(b.add("Bg", b.obj(d, c.Data)), w, h)
		return nil
	}
	im, err := pg.Background()
	if err != nil || im == nil {
		return err
	}
	if c, ok := uniform(im); ok {
		if c != [3]byte{255, 255, 255} {
			b.fill(c)
			b.content = fmt.Appendf(b.content, "0 0 %s %s re f\n", num(b.w), num(b.h))
		}
		return nil
	}
	d, data, err := jpx(im, budget(pg, "BG44", opt))
	if err != nil {
		return fmt.Errorf("BG44: %w", err)
	}
	b.draw(b.add("Bg", b.obj(d, data)), im.W, im.H)
	return nil
}

// foreground paints the mask: in FGbz palette colours, through FG44 or
// FGjp, or in black.
func foreground(b *pageBuilder, pg *djvu.Page, mask *jb2.Image, opt *Options) error {
	whole := func(c [3]byte, keep func(int) bool) {
		b.fill(c)
		b.draw(b.add("Sm", b.obj(stencil(mask.RenderSome(mask.W, mask.H, keep)))), mask.W, mask.H)
	}
	black := [3]byte{}
	if opt.MaskOnly {
		whole(black, nil)
		return nil
	}
	pal, err := pg.Palette()
	if err != nil {
		return err
	}
	if pal != nil && len(pal.Colors) > 0 {
		if pal.Index == nil {
			whole(pal.Colors[0], nil)
			return nil
		}
		if len(pal.Index) != len(mask.Blits) {
			return fmt.Errorf("FGbz: %d colours for %d blits", len(pal.Index), len(mask.Blits))
		}
		used := make([]bool, len(pal.Colors))
		for _, k := range pal.Index {
			used[k] = true
		}
		for k, c := range pal.Colors {
			if used[k] {
				whole(c, func(i int) bool { return pal.Index[i] == k })
			}
		}
		return nil
	}
	if c := pg.Form.Find("FGjp"); c != nil && pg.Form.Find("FG44") == nil {
		d, w, h, err := jpeg(c.Data)
		if err != nil {
			return fmt.Errorf("FGjp: %w", err)
		}
		d["Mask"] = b.obj(stencil(layerMask(b, mask, w, h)))
		b.draw(b.add("Fg", b.obj(d, c.Data)), w, h)
		return nil
	}
	fg, err := pg.Foreground()
	if err != nil {
		return err
	}
	if fg == nil {
		whole(black, nil)
		return nil
	}
	m := mask.RenderSome(mask.W, mask.H, nil)
	if k := reduction(b.in.Width, b.in.Height, fg.W, fg.H); k > 0 {
		// Pad to the layer's area: at the top and right.
		m = mask.RenderSome(k*fg.W, k*fg.H, nil)
		fillUnused(fg, m, k)
	}
	if c, ok := uniform(fg); ok {
		whole(c, nil)
		return nil
	}
	d, data, err := jpx(fg, budget(pg, "FG44", opt))
	if err != nil {
		return fmt.Errorf("FG44: %w", err)
	}
	d["Mask"] = b.obj(stencil(m))
	b.draw(b.add("Fg", b.obj(d, data)), fg.W, fg.H)
	return nil
}

// layerMask renders the mask over the same area as a layer of iw×ih
// pixels: padded at the top and right to k·iw × k·ih.
func layerMask(b *pageBuilder, mask *jb2.Image, iw, ih int) *jb2.Bitmap {
	w, h := mask.W, mask.H
	if k := reduction(b.in.Width, b.in.Height, iw, ih); k > 0 {
		w, h = k*iw, k*ih
	}
	return mask.RenderSome(w, h, nil)
}

// uniform reports whether a layer is one colour, within ±2.
func uniform(im *iw44.Image) ([3]byte, bool) {
	n := 3
	if im.Gray {
		n = 1
	}
	var lo, hi [3]byte
	for c := range n {
		lo[c], hi[c] = 255, 0
	}
	for i, v := range im.Pix {
		c := i % n
		lo[c], hi[c] = min(lo[c], v), max(hi[c], v)
		if hi[c]-lo[c] > 4 {
			return [3]byte{}, false
		}
	}
	var mid [3]byte
	for c := range n {
		mid[c] = byte((int(lo[c]) + int(hi[c]) + 1) / 2)
	}
	if n == 1 {
		mid[1], mid[2] = mid[0], mid[0]
	}
	return mid, true
}

// budget is the JPX byte budget for a layer: its IW44 chunks' size
// times the quality factor.
func budget(pg *djvu.Page, id string, opt *Options) int {
	n := 0
	for _, c := range pg.Form.All(id) {
		n += len(c.Data)
	}
	return max(1, int(float64(n)*opt.Quality))
}

// jpx writes an IW44 layer as a JPXDecode image.
func jpx(im *iw44.Image, budget int) (pdf.Dict, []byte, error) {
	planes := [][]byte{im.Pix}
	cs := "DeviceGray"
	if !im.Gray {
		cs = "DeviceRGB"
		planes = make([][]byte, 3)
		for c := range planes {
			planes[c] = make([]byte, im.W*im.H)
			for i := range planes[c] {
				planes[c][i] = im.Pix[3*i+c]
			}
		}
	}
	data, err := jpxenc.Encode(im.W, im.H, planes, budget)
	if err != nil {
		return nil, nil, err
	}
	d := pdf.Dict{
		"Type":             pdf.Name("XObject"),
		"Subtype":          pdf.Name("Image"),
		"Width":            im.W,
		"Height":           im.H,
		"ColorSpace":       pdf.Name(cs),
		"BitsPerComponent": 8,
		"Filter":           pdf.Name("JPXDecode"),
	}
	return d, data, nil
}

// stencil makes a JBIG2-coded image mask that paints a bitmap's black
// pixels: JBIG2Decode gives black as 0, which a stencil paints with the
// default Decode.
func stencil(b *jb2.Bitmap) (pdf.Dict, []byte) {
	m := jbig2.NewBitmap(b.W, b.H)
	for r := range b.H {
		row := m.Pix[r*m.Stride:]
		for c, v := range b.Pix[r*b.W : (r+1)*b.W] {
			row[c>>3] |= v << (7 - c&7)
		}
	}
	return pdf.Dict{
		"Type":             pdf.Name("XObject"),
		"Subtype":          pdf.Name("Image"),
		"Width":            b.W,
		"Height":           b.H,
		"ImageMask":        true,
		"BitsPerComponent": 1,
		"Filter":           pdf.Name("JBIG2Decode"),
	}, jbig2.GenericPage(m, 0)
}

// num formats a coordinate.
func num(f float64) string {
	return string(pdf.AppendReal(nil, f))
}

// title returns the Title from the page's (metadata ...) annotations.
func title(pg *djvu.Page) string {
	ann, _ := pg.Annotations()
	for _, a := range ann {
		if a.Head() != "metadata" {
			continue
		}
		for _, kv := range a.List[1:] {
			if kv.Head() == "Title" && len(kv.List) == 2 && kv.List[1].Quoted {
				return kv.List[1].Atom
			}
		}
	}
	return ""
}
