// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

// Package reencode re-encodes the scan images of a PDF in place, by
// object number, and copies everything else as it is: bilevel images
// losslessly as JBIG2.
package reencode

import (
	"bytes"
	"fmt"
	"hash/maphash"
	"io"
	"math"
	"runtime"
	"sync"

	"github.com/sergev/dejapdf/internal/jb2"
	"github.com/sergev/dejapdf/internal/jbig2"
	"github.com/sergev/dejapdf/internal/pdf"
	"github.com/sergev/dejapdf/internal/pdfread"
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
	Warn     func(error)         // a problem with an image, which is kept
	Progress func(done, all int) // images done
}

// Stats tell what was done.
type Stats struct {
	Images   int   // bilevel scan images found
	Replaced int   // of them, replaced
	Before   int64 // size of the replaced images' streams
	After    int64 // size of their replacements, globals included
}

// Reencode writes the document of r, with the scan images of doc
// re-encoded, to out.
func Reencode(r *pdfread.Reader, doc *scan.Document, out io.Writer, opt Options) (*Stats, error) {
	var ims []*scan.Image
	seen := map[pdf.Ref]bool{}
	for i := range doc.Pages {
		p := &doc.Pages[i]
		for _, u := range p.Uses {
			if a, _ := doc.Plan(r, p, u); a == scan.JBIG2 && !seen[u.Image.Ref] {
				seen[u.Image.Ref] = true
				ims = append(ims, u.Image)
			}
		}
	}
	st := &Stats{Images: len(ims)}
	repl := map[int]*rewrite.Stream{}
	for k := 0; k < len(ims); k += GroupSize {
		group(r, ims[k:min(k+GroupSize, len(ims))], repl, st, &opt)
		if opt.Progress != nil {
			opt.Progress(min(k+GroupSize, len(ims)), len(ims))
		}
	}
	return st, rewrite.Replace(r, out, repl)
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
func group(r *pdfread.Reader, ims []*scan.Image, repl map[int]*rewrite.Stream, st *Stats, opt *Options) {
	cs := make([]*coded, len(ims))
	parallel(len(ims), func(i int) {
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
	share(js)
	cache := &stencil.Cache{}
	parallel(len(cs), func(i int) {
		c := cs[i]
		if c.err != nil {
			return
		}
		d, data := stencil.Code(c.j, 0, 0, c.j.W, c.j.H, nil, cache)
		if d == nil {
			data = jbig2.GenericPage(jbig2.NewBitmap(c.j.W, c.j.H), 0)
		} else if p, ok := d["DecodeParms"].(pdf.Dict); ok {
			c.glob = p["JBIG2Globals"].(*stencil.Globals)
		}
		c.data, c.j = data, nil
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

// share moves the shapes found in more than one image into a dictionary
// the images share.
func share(js []*jb2.Image) {
	seed := maphash.MakeSeed()
	type class struct {
		b      *jb2.Bitmap
		images int // in how many images
		last   int // the last image counted
		dict   int // index in the dictionary, or -1
	}
	var classes []*class
	byHash := map[uint64][]int{}
	ids := make([][]int, len(js)) // shape to class, by image
	for k, j := range js {
		ids[k] = make([]int, len(j.Shapes))
		for i, s := range j.Shapes {
			h := maphash.Bytes(seed, s.Pix) ^ uint64(s.W)<<32 ^ uint64(s.H)
			id := -1
			for _, c := range byHash[h] {
				if b := classes[c].b; b.W == s.W && b.H == s.H && bytes.Equal(b.Pix, s.Pix) {
					id = c
					break
				}
			}
			if id < 0 {
				id = len(classes)
				classes = append(classes, &class{b: s, last: -1, dict: -1})
				byHash[h] = append(byHash[h], id)
			}
			if c := classes[id]; c.last != k {
				c.images++
				c.last = k
			}
			ids[k][i] = id
		}
	}
	d := &jb2.Dict{}
	for _, c := range classes {
		if c.images > 1 {
			c.dict = len(d.Shapes)
			d.Shapes = append(d.Shapes, c.b)
			d.Refs = append(d.Refs, jb2.Ref{Shape: -1})
		}
	}
	if len(d.Shapes) == 0 {
		return
	}
	for k, j := range js {
		// New numbering: the dictionary, then the image's other shapes.
		num := make([]int, len(j.Shapes))
		var own []*jb2.Bitmap
		for i, s := range j.Shapes {
			if c := classes[ids[k][i]]; c.dict >= 0 {
				num[i] = c.dict
			} else {
				num[i] = len(d.Shapes) + len(own)
				own = append(own, s)
			}
		}
		for b := range j.Blits {
			j.Blits[b].Shape = num[j.Blits[b].Shape]
		}
		j.Dict, j.NDict, j.Shapes = d, len(d.Shapes), own
		j.Refs = make([]jb2.Ref, len(own))
		for i := range j.Refs {
			j.Refs[i] = jb2.Ref{Shape: -1}
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
