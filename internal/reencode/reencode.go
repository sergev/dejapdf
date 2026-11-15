// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

// Package reencode re-encodes the scan images of a PDF in place, by
// object number, and copies everything else as it is: bilevel images
// losslessly as JBIG2; gray and colour ones as a form of a JPX
// background and JBIG2 stencils in the colours of the text.
package reencode

import (
	"bytes"
	"fmt"
	"hash/maphash"
	"io"
	"maps"
	"math"
	"runtime"
	"slices"
	"strings"
	"sync"

	"github.com/sergev/dejapdf/internal/jb2"
	"github.com/sergev/dejapdf/internal/jbig2"
	"github.com/sergev/dejapdf/internal/pdf"
	"github.com/sergev/dejapdf/internal/pdfread"
	"github.com/sergev/dejapdf/internal/progress"
	"github.com/sergev/dejapdf/internal/raster"
	"github.com/sergev/dejapdf/internal/rewrite"
	"github.com/sergev/dejapdf/internal/scan"
	"github.com/sergev/dejapdf/internal/shapes"
	"github.com/sergev/dejapdf/internal/stencil"
)

// GroupSize is the number of images sharing one /JBIG2Globals.
var GroupSize = 32

// Options control the re-encoding.
type Options struct {
	Warn    func(error)     // a problem with an image, which is kept
	Meter   *progress.Meter // told of the work done; may be nil
	Lossy   bool            // substitute alike shapes
	Quality float64         // background size cap, a fraction of the image; 0 means DefaultQuality
	Pages   []int           // pages to keep, from 0 in output order; nil means all
}

// NothingError is returned, before anything is written, when the
// document has no scan image to re-encode, with the reason.
type NothingError struct{ Reason string }

func (e *NothingError) Error() string { return "nothing to re-encode: " + e.Reason }

// Stats tell what was done.
type Stats struct {
	Images   int   // scan images found
	Replaced int   // of them, replaced
	Before   int64 // size of the replaced images' streams
	After    int64 // size of their replacements, globals included
}

// Reencode writes the document of r, with the scan images of doc
// re-encoded, to out.
func Reencode(r *pdfread.Reader, doc *scan.Document, out io.Writer, opt Options) (*Stats, error) {
	var ims, seps []*scan.Image
	dpi := map[*scan.Image]float64{}
	seen := map[pdf.Ref]bool{}
	sel := opt.Pages
	if sel == nil {
		sel = make([]int, len(doc.Pages))
		for i := range sel {
			sel[i] = i
		}
	}
	why := map[string]int{}
	for _, i := range sel {
		if i < 0 || i >= len(doc.Pages) {
			continue
		}
		p := &doc.Pages[i]
		for _, u := range p.Uses {
			a, reason := doc.Plan(r, p, u)
			if a == scan.Keep && !seen[u.Image.Ref] {
				why[reason]++
			}
			if seen[u.Image.Ref] || (a != scan.JBIG2 && a != scan.Separate) {
				continue
			}
			seen[u.Image.Ref] = true
			if a == scan.JBIG2 {
				ims = append(ims, u.Image)
			} else {
				seps = append(seps, u.Image)
				dpi[u.Image] = max(u.DPIX, u.DPIY)
			}
		}
	}
	all := len(ims) + len(seps)
	if all == 0 {
		return nil, nothing(doc, why)
	}
	st := &Stats{Images: all}
	repl := map[int]*rewrite.Stream{}
	w := newWork(r, opt.Meter, ims, seps)
	for k := 0; k < len(ims); k += GroupSize {
		group(r, ims[k:min(k+GroupSize, len(ims))], repl, st, &opt, w)
	}
	for k := 0; k < len(seps); k += GroupSize {
		separate(r, seps[k:min(k+GroupSize, len(seps))], dpi, repl, st, &opt, w)
	}
	w.writing()
	return st, rewrite.Select(r, out, repl, opt.Pages, opt.Meter.Add)
}

// coded is an image's replacement.
type coded struct {
	im   *scan.Image
	bm   *raster.Bitmap
	j    *jb2.Image
	data []byte
	glob *stencil.Globals
	err  error
}

// group re-encodes images sharing one dictionary of the shapes found in
// more than one of them.
func group(r *pdfread.Reader, ims []*scan.Image, repl map[int]*rewrite.Stream, st *Stats, opt *Options,
	w *work) {
	cs := make([]*coded, len(ims))
	parallel(len(ims), func(i int) {
		defer w.add(ims[i], bilevelCost.decode)
		c := &coded{im: ims[i]}
		cs[i] = c
		v, err := raster.Decode(r, c.im)
		if err != nil {
			c.err = err
			return
		}
		c.bm = v.(*raster.Bitmap)
		c.j = shapes.Split(c.bm)
	})
	var js []*jb2.Image
	for _, c := range cs {
		if c.err == nil {
			js = append(js, c.j)
		}
	}
	step, shared := w.sharing(ims, bilevelCost.share)
	shareStep(js, opt.Lossy, step)
	shared()
	cache := &stencil.Cache{}
	parallel(len(cs), func(i int) {
		defer w.coded(ims[i], bilevelCost.code)
		c := cs[i]
		if c.err != nil {
			return
		}
		c.data, c.glob = code(c.j, cache)
		c.j = nil
	})

	// Keep the originals where not smaller: each image, then the group
	// with its globals.
	var keep []*coded
	var before, after int64
	var glob *stencil.Globals
	for _, c := range cs {
		if c.err != nil {
			if opt.Warn != nil {
				opt.Warn(fmt.Errorf("object %d: %w", c.im.Ref, c.err))
			}
			continue
		}
		s, _ := r.Get(int(c.im.Ref)).(*pdfread.Stream)
		if s == nil || len(c.data) >= len(s.Raw) {
			continue
		}
		keep = append(keep, c)
		before += int64(len(s.Raw))
		after += int64(len(c.data))
		if c.glob != nil {
			glob = c.glob
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
	for _, c := range keep {
		repl[int(c.im.Ref)] = &rewrite.Stream{Dict: imageDict(c, gs), Data: c.data}
	}
	st.Replaced += len(keep)
	st.Before += before
	st.After += after
}

// code codes j as JBIG2, its shapes in c's globals if it has a
// dictionary.
func code(j *jb2.Image, c *stencil.Cache) ([]byte, *stencil.Globals) {
	d, data := stencil.Code(j, 0, 0, j.W, j.H, nil, c)
	if d == nil {
		return jbig2.GenericPage(jbig2.NewBitmap(j.W, j.H), 0), nil
	}
	if p, ok := d["DecodeParms"].(pdf.Dict); ok {
		return data, p["JBIG2Globals"].(*stencil.Globals)
	}
	return data, nil
}

// imageDict is the image's dictionary for JBIG2 data: the original,
// with the filter, bits and /Decode replaced.
func imageDict(c *coded, glob *rewrite.Stream) pdf.Dict {
	d := pdf.Dict{}
	for k, v := range c.im.Dict {
		switch k {
		case "Filter", "DecodeParms", "Decode", "Length", "BitsPerComponent", "DL":
		default:
			d[k] = v
		}
	}
	d["Filter"] = pdf.Name("JBIG2Decode")
	d["BitsPerComponent"] = 1
	if glob != nil {
		d["DecodeParms"] = pdf.Dict{"JBIG2Globals": glob}
	}
	if dec := c.bm.Decode; dec != [2]float64{0, 1} {
		d["Decode"] = pdf.Array{number(dec[0]), number(dec[1])}
	}
	return d
}

// number is v as an integer if it is one.
func number(v float64) any {
	if v == math.Trunc(v) && math.Abs(v) < 1<<31 {
		return int(v)
	}
	return v
}

// Refine has lossless re-encoding code shapes like a representative
// as its refinements.
var Refine = true

// share lays out the shapes of the group: alike ones, found by
// shapes.Classify, are substituted (lossy) or refined (Refine); those
// the images use, or refine from, in more than one image go to a
// dictionary they share.
func share(js []*jb2.Image, lossy bool) {
	shareStep(js, lossy, nil)
}

// shareStep is share telling step, if not nil, how far it is.
func shareStep(js []*jb2.Image, lossy bool, step func(done, all int)) {
	// Equal shapes are one class.
	seed := maphash.MakeSeed()
	var classes []*jb2.Bitmap
	byHash := map[uint64][]int{}
	ids := make([][]int, len(js)) // shape to class, by image
	for k, j := range js {
		ids[k] = make([]int, len(j.Shapes))
		for i, s := range j.Shapes {
			h := maphash.Bytes(seed, s.Pix) ^ uint64(s.W)<<32 ^ uint64(s.H)
			id := -1
			for _, c := range byHash[h] {
				if b := classes[c]; b.W == s.W && b.H == s.H && bytes.Equal(b.Pix, s.Pix) {
					id = c
					break
				}
			}
			if id < 0 {
				id = len(classes)
				classes = append(classes, s)
				byHash[h] = append(byHash[h], id)
			}
			ids[k][i] = id
		}
	}
	ref := make([]int, len(classes)) // the class a class refines, or -1
	for i := range ref {
		ref[i] = -1
	}
	// Blits by class, per image.
	type blit struct{ class, x, y int }
	blits := make([][]blit, len(js))
	for k, j := range js {
		blits[k] = make([]blit, len(j.Blits))
		for b, bl := range j.Blits {
			blits[k][b] = blit{ids[k][bl.Shape], bl.X, bl.Y}
		}
	}
	if lossy || Refine {
		cl := shapes.ClassifyStep(classes, step)
		if lossy {
			for k := range blits {
				for b, bl := range blits[k] {
					// Blit.Y is the bottom row, from the bottom.
					if x := cl[bl.class]; x.Rep != bl.class {
						s, rep := classes[bl.class], classes[x.Rep]
						blits[k][b] = blit{x.Rep, bl.x + x.DX, bl.y + s.H - x.DY - rep.H}
					}
				}
			}
		} else {
			for c, x := range cl {
				if s, rep := classes[c], classes[x.Rep]; x.Rep != c && s.W == rep.W && s.H == rep.H {
					ref[c] = x.Rep
				}
			}
		}
	}

	// Which images use each class.
	images := make([]int, len(classes))
	last := make([]int, len(classes))
	for i := range last {
		last[i] = -1
	}
	use := func(c, k int) {
		if last[c] != k {
			last[c] = k
			images[c]++
		}
	}
	for k := range blits {
		for _, bl := range blits[k] {
			use(bl.class, k)
			if r := ref[bl.class]; r >= 0 {
				use(r, k)
			}
		}
	}
	d := &jb2.Dict{}
	dict := make([]int, len(classes)) // index in the dictionary, or -1
	for c := range classes {
		dict[c] = -1
		if images[c] > 1 {
			dict[c] = len(d.Shapes)
			d.Shapes = append(d.Shapes, classes[c])
		}
	}
	d.Refs = make([]jb2.Ref, len(d.Shapes))
	for c, i := range dict {
		if i >= 0 {
			d.Refs[i] = jb2.Ref{Shape: -1}
			if r := ref[c]; r >= 0 {
				d.Refs[i] = jb2.Ref{Shape: dict[r]}
			}
		}
	}
	for k, j := range js {
		// Numbering: the dictionary, then the image's other classes in
		// class order.
		num := map[int]int{}
		var own []int
		add := func(c int) {
			if _, ok := num[c]; ok {
				return
			}
			if dict[c] >= 0 {
				num[c] = dict[c]
			} else {
				num[c] = -1 // numbered below
				own = append(own, c)
			}
		}
		for _, bl := range blits[k] {
			add(bl.class)
			if r := ref[bl.class]; r >= 0 {
				add(r)
			}
		}
		slices.Sort(own)
		nd := 0
		if len(d.Shapes) > 0 {
			nd = len(d.Shapes)
			j.Dict = d
		}
		j.NDict = nd
		j.Shapes = make([]*jb2.Bitmap, len(own))
		j.Refs = make([]jb2.Ref, len(own))
		for i, c := range own {
			num[c] = nd + i
		}
		for i, c := range own {
			j.Shapes[i] = classes[c]
			j.Refs[i] = jb2.Ref{Shape: -1}
			if r := ref[c]; r >= 0 {
				j.Refs[i] = jb2.Ref{Shape: num[r]}
			}
		}
		j.Blits = j.Blits[:len(blits[k])]
		for b, bl := range blits[k] {
			j.Blits[b] = jb2.Blit{Shape: num[bl.class], X: bl.x, Y: bl.y}
		}
	}
}

// parallel runs f(0..n-1) on all CPUs.
func parallel(n int, f func(int)) {
	var wg sync.WaitGroup
	next := make(chan int)
	for range min(runtime.GOMAXPROCS(0), n) {
		wg.Go(func() {
			for i := range next {
				f(i)
			}
		})
	}
	for i := range n {
		next <- i
	}
	close(next)
	wg.Wait()
}

// nothing explains why there is nothing to re-encode.
func nothing(doc *scan.Document, why map[string]int) error {
	n := 0
	for _, p := range doc.Pages {
		if p.Scanned {
			n++
		}
	}
	if !doc.Scanned() || n == 0 {
		return &NothingError{fmt.Sprintf("not a scanned document (scans on %d of %d pages)", n, len(doc.Pages))}
	}
	if len(why) == 0 {
		return &NothingError{"no scan images on these pages"}
	}
	var rs []string
	for _, k := range slices.Sorted(maps.Keys(why)) {
		rs = append(rs, fmt.Sprintf("%d %s", why[k], k))
	}
	return &NothingError{"the scan images are kept: " + strings.Join(rs, "; ")}
}
