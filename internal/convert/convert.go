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
	jpxenc "github.com/sergev/dejapdf/internal/jpx"
	"github.com/sergev/dejapdf/internal/pdf"
)

// DefaultQuality is the default JPX size relative to IW44. Re-encoding
// IW44 output loses 0.3–1.5 dB against the original at 1×, 0.1–1 dB at
// 1.2× (docs/JPEG_2000_Image_Coding.md §10).
const DefaultQuality = 1.2

// Options control the conversion.
type Options struct {
	Producer string  // document info /Producer
	Quality  float64 // JPX size as a multiple of the IW44 size; 0 means DefaultQuality
	MaskOnly bool    // paint only the mask, in black

	// Pages to convert, indexes from 0 in output order; nil means all.
	// Links and bookmarks to other pages are dropped.
	Pages []int
	// Warn, if not nil, is told of a layer that cannot be converted,
	// which is then left out; if nil, that is an error.
	Warn func(page int, err error)
	// Progress, if not nil, is called after each page is written.
	Progress func(Progress)
	// Stats, if not nil, receives sizes by layer.
	Stats *Stats
}

// Progress tells how far a conversion is.
type Progress struct {
	Page, Pages int   // pages written, pages to write
	Done, Total int64 // DjVu bytes of the pages written, of all pages to write
	Written     int64 // PDF bytes written
}

// Convert writes doc as PDF to w.
func Convert(doc *djvu.Document, w io.Writer, opt Options) error {
	if opt.Quality <= 0 {
		opt.Quality = DefaultQuality
	}
	sel := opt.Pages
	if sel == nil {
		sel = make([]int, len(doc.Pages))
		for i := range sel {
			sel[i] = i
		}
	}
	cw := &counter{w: w}
	p := pdf.NewWriter(cw)
	pages := make([]pdf.Ref, len(doc.Pages)) // 0 if not converted
	for _, i := range sel {
		if i < 0 || i >= len(doc.Pages) || pages[i] != 0 {
			return fmt.Errorf("bad page selection: %d", i+1)
		}
		pages[i] = p.Alloc()
	}
	for _, w := range doc.Warnings {
		if opt.Warn == nil {
			return w
		}
		opt.Warn(-1, w)
	}
	blank := blankSizes(doc)
	tg := newTargets(doc, pages)
	font := &textFont{}
	// Pages are built in parallel and written in order.
	type result struct {
		b   *pageBuilder
		err error
	}
	results := make([]chan result, len(sel))
	for i := range results {
		results[i] = make(chan result, 1)
	}
	window := make(chan struct{}, 2*runtime.GOMAXPROCS(0))
	dicts := &sharedDicts{}
	go func() {
		for k, i := range sel {
			window <- struct{}{}
			go func(i int, c chan result) {
				defer func() {
					// A decoder bug must not take down the whole file.
					if e := recover(); e != nil {
						err := fmt.Errorf("internal error: %v", e)
						if opt.Warn == nil {
							c <- result{nil, err}
							return
						}
						opt.Warn(i, err)
						c <- result{blankPage(blank[i], dicts), nil}
					}
				}()
				b, err := build(doc, doc.Pages[i], i, &opt, dicts, tg, blank[i])
				c <- result{b, err}
			}(i, results[k])
		}
	}()
	var err error
	var pr Progress
	weight := make([]int64, len(sel))
	if opt.Progress != nil {
		pr.Pages = len(sel)
		for k, i := range sel {
			weight[k] = chunkSize(doc.Pages[i].Form)
			pr.Total += weight[k]
		}
	}
	for k, i := range sel {
		r := <-results[k]
		if err == nil && r.err != nil {
			err = fmt.Errorf("page %d: %w", i+1, r.err)
		}
		if err == nil {
			r.b.emit(p, pages[i], pages, font, opt.Stats)
			if opt.Progress != nil {
				pr.Page, pr.Done, pr.Written = k+1, pr.Done+weight[k], cw.n
				opt.Progress(pr)
			}
		}
		<-window
	}
	if err != nil {
		return err
	}
	info := pdf.Dict{}
	cat := pdf.Dict{}
	for _, pg := range doc.Pages {
		if pg.Err == nil {
			metadata(pg, info)
			if x := xmp(pg, opt.Producer); x != nil {
				cat["Metadata"] = p.AddStream(pdf.Dict{"Type": pdf.Name("Metadata"), "Subtype": pdf.Name("XML"),
					"Filter": pdf.Name("FlateDecode")}, pdf.Flate(x))
			}
			break
		}
	}
	if l := pageLabels(doc, sel); l != nil {
		cat["PageLabels"] = l
	}
	if opt.Producer != "" {
		info["Producer"] = pdf.TextString(opt.Producer)
	}
	bms, err := doc.Outline()
	if err != nil {
		if opt.Warn == nil {
			return err
		}
		opt.Warn(-1, err)
	}
	bms = tg.prune(bms)
	if len(bms) > 0 {
		cat["Outlines"] = outline(p, bms, tg, pages)
		cat["PageMode"] = pdf.Name("UseOutlines")
	}
	if err := p.Close(cat, info); err != nil {
		return err
	}
	if s := opt.Stats; s != nil {
		djvuSizes(doc, sel, s)
		s.PDF[Other] = cw.n - s.PDF[Mask] - s.PDF[Colour] - s.PDF[Text]
	}
	return nil
}

// pageBuilder collects a page's content and objects.
type pageBuilder struct {
	in      djvu.Info
	w, h    float64 // page size in points
	objs    []object
	xobj    pdf.Dict // name to local
	content []byte
	shared  *sharedDicts
	font    bool       // the content uses the hidden-text font
	annots  []pdf.Dict // pageNum values refer to pages
	pat     pdf.Dict   // pattern name to local
	text    int        // compressed size of the text operators, if counted
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

// emit writes the page as object r, and its objects.
func (b *pageBuilder) emit(p *pdf.Writer, r pdf.Ref, pages []pdf.Ref, font *textFont, st *Stats) {
	refs := make([]pdf.Ref, len(b.objs))
	for i := range refs {
		refs[i] = p.Alloc()
	}
	var value func(v any) any
	value = func(v any) any {
		switch v := v.(type) {
		case local:
			return refs[v]
		case pageNum:
			return pages[v]
		case *shared:
			if v.ref == 0 {
				v.ref = p.AddStream(pdf.Dict{}, v.data)
				if st != nil {
					st.PDF[Mask] += int64(len(v.data))
				}
			}
			return v.ref
		case pdf.Dict:
			for k, e := range v {
				v[k] = value(e)
			}
		case pdf.Array:
			for i, e := range v {
				v[i] = value(e)
			}
		}
		return v
	}
	resolve := func(d pdf.Dict) { value(d) }
	for i, o := range b.objs {
		resolve(o.dict)
		p.Stream(refs[i], o.dict, o.data)
		if st != nil {
			st.PDF[streamKind(o.dict)] += int64(len(o.data))
		}
	}
	if st != nil {
		st.PDF[Text] += int64(b.text)
	}
	resolve(b.xobj)
	res := pdf.Dict{}
	if len(b.xobj) > 0 {
		res["XObject"] = b.xobj
	}
	if b.font {
		res["Font"] = pdf.Dict{"Tx": font.get(p)}
	}
	if len(b.pat) > 0 {
		resolve(b.pat)
		res["Pattern"] = b.pat
	}
	var annots pdf.Array
	for _, a := range b.annots {
		resolve(a)
		annots = append(annots, p.Add(a))
	}
	p.AddPage(pdf.Page{Ref: r, Width: b.w, Height: b.h, Rotate: b.in.Rotate,
		Resources: res, Content: b.content, Annots: annots})
}

// draw paints XObject name over the area a layer of iw×ih pixels covers.
// A layer reduced k times is ⌈W/k⌉×⌈H/k⌉ and anchored at the bottom left,
// so it may stick out at the top and right; other sizes fill the page.
func (b *pageBuilder) draw(name string, iw, ih int) {
	w, h := b.area(iw, ih)
	b.content = fmt.Appendf(b.content, "q %s 0 0 %s 0 0 cm /%s Do Q\n", num(w), num(h), name)
}

// drawAt paints XObject name over the w×h page pixels at (x, y) from the
// bottom left, inset by a hair on every side. Viewers snap image edges
// to pixels: MuPDF truncates the low edges, Poppler floors both and adds
// one to the high edges, so an edge that float error puts just past a
// pixel boundary stretches the image by a pixel. Edges strictly inside
// their pixels snap exactly (docs/Portable_Document_Format.md §8).
func (b *pageBuilder) drawAt(name string, x, y, w, h int) {
	const inset = 0.002 // points
	s := 72 / float64(b.in.DPI)
	b.content = fmt.Appendf(b.content, "q %s 0 0 %s %s %s cm /%s Do Q\n",
		num(float64(w)*s-2*inset), num(float64(h)*s-2*inset), num(float64(x)*s+inset), num(float64(y)*s+inset), name)
}

// area is the size in points that a layer of iw×ih pixels covers.
func (b *pageBuilder) area(iw, ih int) (float64, float64) {
	if k := reduction(b.in.Width, b.in.Height, iw, ih); k > 0 {
		s := float64(k) * 72 / float64(b.in.DPI)
		return float64(iw) * s, float64(ih) * s
	}
	return b.w, b.h
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

// build prepares a page. A layer that fails is left out with a warning,
// if opt.Warn is set.
// A page that cannot be read becomes a blank page of size blank.
func build(doc *djvu.Document, pg *djvu.Page, i int, opt *Options, dicts *sharedDicts, tg *targets,
	blank djvu.Info) (*pageBuilder, error) {
	in := pg.Info
	err := pg.Err
	if err == nil && (in.Width <= 0 || in.Height <= 0) {
		err = fmt.Errorf("bad size %dx%d", in.Width, in.Height)
	}
	if err != nil {
		if opt.Warn == nil {
			return nil, err
		}
		opt.Warn(i, fmt.Errorf("blank page: %w", err))
		in = blank
	}
	b := blankPage(in, dicts)
	if err != nil {
		return b, nil
	}
	var first error
	try := func(err error) {
		if err == nil {
			return
		}
		if opt.Warn != nil {
			opt.Warn(i, err)
		} else if first == nil {
			first = err
		}
	}
	if !opt.MaskOnly {
		try(background(b, pg, opt))
	}
	mask, err := doc.Mask(pg)
	switch {
	case err != nil:
		try(err)
	case mask != nil:
		try(foreground(b, pg, mask, opt))
	}
	try(text(b, pg, opt.Stats != nil))
	try(links(b, pg, tg, i))
	return b, first
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
		if d, data := b.maskImage(mask, 0, 0, mask.W, mask.H, keep); d != nil {
			b.fill(c)
			b.draw(b.add("Sm", b.obj(d, data)), mask.W, mask.H)
		}
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
		// One stencil per colour, cropped to its blits: a full-page
		// stencil for each of many colours renders slowly.
		box := make([][4]int, len(pal.Colors)) // x0, y0, x1, y1
		for k := range box {
			box[k] = [4]int{mask.W, mask.H, 0, 0}
		}
		for i, bl := range mask.Blits {
			s := mask.Shape(bl.Shape)
			if s.W == 0 || s.H == 0 {
				continue
			}
			r := &box[pal.Index[i]]
			r[0], r[1] = min(r[0], max(bl.X, 0)), min(r[1], max(bl.Y, 0))
			r[2], r[3] = max(r[2], min(bl.X+s.W, mask.W)), max(r[3], min(bl.Y+s.H, mask.H))
		}
		for k, c := range pal.Colors {
			r := box[k]
			if r[2] <= r[0] || r[3] <= r[1] {
				continue
			}
			keep := func(i int) bool { return pal.Index[i] == k }
			if mask.W != b.in.Width || mask.H != b.in.Height {
				whole(c, keep)
				continue
			}
			if d, data := b.maskImage(mask, r[0], r[1], r[2]-r[0], r[3]-r[1], keep); d != nil {
				b.fill(c)
				b.drawAt(b.add("Sm", b.obj(d, data)), r[0], r[1], r[2]-r[0], r[3]-r[1])
			}
		}
		return nil
	}
	if c := pg.Form.Find("FGjp"); c != nil && pg.Form.Find("FG44") == nil {
		d, w, h, err := jpeg(c.Data)
		if err != nil {
			return fmt.Errorf("FGjp: %w", err)
		}
		b.fgPattern(mask, b.obj(d, c.Data), w, h)
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
	if k := reduction(b.in.Width, b.in.Height, fg.W, fg.H); k > 0 {
		fillUnused(fg, mask.RenderSome(k*fg.W, k*fg.H, nil), k)
	}
	if c, ok := uniform(fg); ok {
		whole(c, nil)
		return nil
	}
	d, data, err := jpx(fg, budget(pg, "FG44", opt))
	if err != nil {
		return fmt.Errorf("FG44: %w", err)
	}
	b.fgPattern(mask, b.obj(d, data), fg.W, fg.H)
	return nil
}

// fgPattern paints the mask in the colours of img, an iw×ih foreground
// layer: the stencil filled with a tiling pattern whose one cell is the
// image. An image with the stencil as /Mask (or /SMask) would be simpler,
// but Poppler blends such an image into the background along the mask's
// edges (docs/Portable_Document_Format.md §8).
func (b *pageBuilder) fgPattern(mask *jb2.Image, img local, iw, ih int) {
	sd, sdata := b.maskImage(mask, 0, 0, mask.W, mask.H, nil)
	if sd == nil {
		return
	}
	w, h := b.area(iw, ih)
	cell := fmt.Appendf(nil, "q %s 0 0 %s 0 0 cm /Fg Do Q", num(w), num(h))
	pat := b.obj(pdf.Dict{"Type": pdf.Name("Pattern"), "PatternType": 1, "PaintType": 1, "TilingType": 1,
		"BBox": pdf.Array{0, 0, w, h}, "XStep": w, "YStep": h,
		"Resources": pdf.Dict{"XObject": pdf.Dict{"Fg": img}}}, cell)
	if b.pat == nil {
		b.pat = pdf.Dict{}
	}
	name := fmt.Sprintf("P%d", len(b.pat))
	b.pat[name] = pat
	b.content = fmt.Appendf(b.content, "/Pattern cs /%s scn\n", name)
	b.draw(b.add("Sm", b.obj(sd, sdata)), mask.W, mask.H)
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
		"Subtype":    pdf.Name("Image"),
		"Width":      im.W,
		"Height":     im.H,
		"ColorSpace": pdf.Name(cs),
		"Filter":     pdf.Name("JPXDecode"),
	}
	return d, data, nil
}

// num formats a coordinate.
func num(f float64) string {
	return string(pdf.AppendReal(nil, f))
}

// blankPage starts a page of the given size.
func blankPage(in djvu.Info, dicts *sharedDicts) *pageBuilder {
	return &pageBuilder{in: in, xobj: pdf.Dict{}, shared: dicts,
		w: float64(in.Width) * 72 / float64(in.DPI),
		h: float64(in.Height) * 72 / float64(in.DPI)}
}

// blankSizes gives each page the size it gets if it cannot be read: the
// nearest readable page's, before or after it, or US Letter.
func blankSizes(doc *djvu.Document) []djvu.Info {
	r := make([]djvu.Info, len(doc.Pages))
	last := djvu.Info{Width: 2550, Height: 3300, DPI: 300}
	for _, pg := range doc.Pages {
		if ok(pg) {
			last = pg.Info
			break
		}
	}
	for i, pg := range doc.Pages {
		if ok(pg) {
			last = pg.Info
		}
		r[i] = djvu.Info{Width: last.Width, Height: last.Height, DPI: last.DPI}
	}
	return r
}

func ok(pg *djvu.Page) bool { return pg.Err == nil && pg.Info.Width > 0 && pg.Info.Height > 0 }
