// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package convert

import (
	"slices"
	"sync"

	"github.com/sergev/dejapdf/internal/jb2"
	"github.com/sergev/dejapdf/internal/jbig2"
	"github.com/sergev/dejapdf/internal/pdf"
)

// Text region layout: strips of rows and the expected gap between
// neighbours (docs/JBIG2_Coding.md §9).
const (
	sbStrips   = 2
	sbDSOffset = 3
)

// minRefined is the fewest refined shapes worth a refinement dictionary.
const minRefined = 100

// shared is a Djbz dictionary coded once as a JBIG2Globals stream.
type shared struct {
	data []byte
	ids  []int           // symbol number of each shape, -1 for an empty one
	syms []*jbig2.Bitmap // the symbols by number
	segs int             // segments in data
	ref  pdf.Ref
}

// sharedDicts caches the document's coded Djbz dictionaries.
type sharedDicts struct {
	mu sync.Mutex
	m  map[*jb2.Dict]*shared
}

func (sd *sharedDicts) get(d *jb2.Dict) *shared {
	sd.mu.Lock()
	defer sd.mu.Unlock()
	if s, ok := sd.m[d]; ok {
		return s
	}
	var want []int
	for i, b := range d.Shapes {
		if b.W > 0 && b.H > 0 {
			want = append(want, i)
		}
	}
	set := codeShapes(want, func(i int) *jb2.Bitmap { return d.Shapes[i] },
		func(i int) jb2.Ref { return d.Refs[i] }, nil, nil, 0, 0)
	s := &shared{data: set.segs, ids: make([]int, len(d.Shapes)), syms: set.syms, segs: set.nsegs}
	for i := range s.ids {
		s.ids[i] = -1
	}
	for i, id := range set.ids {
		s.ids[i] = id
	}
	if sd.m == nil {
		sd.m = map[*jb2.Dict]*shared{}
	}
	sd.m[d] = s
	return s
}

// symbolSet is shapes coded as up to two symbol dictionary segments:
// the directly coded shapes, then the refined ones.
type symbolSet struct {
	segs  []byte
	nsegs int
	ids   map[int]int     // shape → symbol number, after the inputs
	syms  []*jbig2.Bitmap // inputs then the new symbols, by number
	refs  []uint32        // the segments a text region refers to
}

// codeShapes codes shapes want (in increasing order, refinement
// references included) as segments numbered from first, of page page.
// Shapes refined from outside the set refer to the input symbols: ext
// gives their numbers, extSyms their bitmaps, extSegs the segments that
// define them.
func codeShapes(want []int, shape func(int) *jb2.Bitmap, ref func(int) jb2.Ref,
	ext func(int) int, extSyms []*jbig2.Bitmap, first uint32, page uint32, extSegs ...uint32) *symbolSet {
	set := &symbolSet{ids: map[int]int{}, syms: append([]*jbig2.Bitmap(nil), extSyms...),
		refs: append([]uint32(nil), extSegs...)}
	in := map[int]bool{}
	for _, i := range want {
		in[i] = true
	}
	refined := func(i int) bool {
		r := ref(i)
		return r.Shape >= 0 && (in[r.Shape] || ext != nil && r.Shape >= 0 && ext(r.Shape) >= 0)
	}
	var direct, refd []int
	for _, i := range want {
		if refined(i) {
			refd = append(refd, i)
		} else {
			direct = append(direct, i)
		}
	}
	// A few refinements code smaller as direct symbols: the refinement
	// contexts would start cold.
	if len(refd) < minRefined {
		direct, refd = want, nil
	}
	n := uint32(first)
	if len(direct) > 0 {
		bms := make([]*jbig2.Bitmap, len(direct))
		for k, i := range direct {
			bms[k] = pack(shape(i))
		}
		data, ids := jbig2.SymbolDict(bms)
		base := len(set.syms)
		set.syms = append(set.syms, make([]*jbig2.Bitmap, len(bms))...)
		for k, i := range direct {
			set.ids[i] = base + ids[k]
			set.syms[base+ids[k]] = bms[k]
		}
		seg := jbig2.Segment{Number: n, Type: jbig2.TypeSymbolDict, Page: page, Data: data}
		set.segs = seg.Append(set.segs)
		set.refs = append(set.refs, n)
		n++
	}
	if len(refd) > 0 {
		inputs := set.syms
		bms := make([]*jbig2.Bitmap, len(refd))
		refs := make([]jbig2.Ref, len(refd))
		for k, i := range refd {
			r := ref(i)
			id, ok := set.ids[r.Shape]
			if !ok {
				id = ext(r.Shape)
			}
			bms[k] = pack(shape(i))
			refs[k] = jbig2.Ref{ID: id, DX: r.DX, DY: r.DY}
			set.ids[i] = len(inputs) + k
		}
		data := jbig2.RefineDict(bms, refs, inputs)
		set.syms = append(append([]*jbig2.Bitmap(nil), inputs...), bms...)
		seg := jbig2.Segment{Number: n, Type: jbig2.TypeSymbolDict, Refs: set.refs, Page: page, Data: data}
		set.segs = seg.Append(set.segs)
		set.refs = append(set.refs, n)
		n++
	}
	set.nsegs = int(n - first)
	return set
}

// pack converts a byte-per-pixel bitmap.
func pack(b *jb2.Bitmap) *jbig2.Bitmap {
	m := jbig2.NewBitmap(b.W, b.H)
	for r := range b.H {
		row := m.Pix[r*m.Stride:]
		for c, v := range b.Pix[r*b.W : (r+1)*b.W] {
			row[c>>3] |= v << (7 - c&7)
		}
	}
	return m
}

// maskImage returns the blits i with keep(i) (all if keep is nil) as a
// w×h JBIG2 stencil, its bottom-left corner the mask's: dictionary and
// data, or nil if nothing would be drawn. Shapes of a Djbz go to its
// shared globals; the page's own shapes, and those they refine, go to
// page dictionaries (docs/JBIG2_Coding.md §10).
func (b *pageBuilder) maskImage(mask *jb2.Image, w, h int, keep func(int) bool) (pdf.Dict, []byte) {
	empty := func(i int) bool { s := mask.Shape(i); return s.W == 0 || s.H == 0 }
	var blits []int
	useGlob := false
	need := map[int]bool{} // page shapes to define
	for i, bl := range mask.Blits {
		if (keep == nil || keep(i)) && !empty(bl.Shape) {
			blits = append(blits, i)
			if bl.Shape < mask.NDict {
				useGlob = true
			} else {
				need[bl.Shape] = true
			}
		}
	}
	if len(blits) == 0 {
		return nil, nil
	}
	var glob *shared
	var ext func(int) int
	var extSyms []*jbig2.Bitmap
	var extSegs []uint32
	n := uint32(0)
	if useGlob {
		glob = b.shared.get(mask.Dict)
		ext = func(i int) int {
			if i < mask.NDict {
				return glob.ids[i]
			}
			return -1
		}
		extSyms = glob.syms
		for k := range glob.segs {
			extSegs = append(extSegs, uint32(k))
		}
		n = uint32(glob.segs)
	}
	want := make([]int, 0, len(need))
	for i := range need {
		want = append(want, i)
	}
	slices.Sort(want)
	pi := jbig2.Segment{Number: n, Type: jbig2.TypePageInfo, Page: 1, Data: jbig2.PageInfo(w, h, 0)}
	segs := pi.Append(nil)
	set := codeShapes(want, mask.Shape, mask.ShapeRef, ext, extSyms, n+1, 1, extSegs...)
	segs = append(segs, set.segs...)
	ins := make([]jbig2.Instance, len(blits))
	for j, i := range blits {
		bl := mask.Blits[i]
		id, ok := set.ids[bl.Shape]
		if !ok {
			id = glob.ids[bl.Shape]
		}
		ins[j] = jbig2.Instance{ID: id, S: bl.X, T: h - 1 - bl.Y, W: mask.Shape(bl.Shape).W}
	}
	tr := jbig2.Segment{Number: n + 1 + uint32(set.nsegs), Type: jbig2.TypeTextRegion, Refs: set.refs, Page: 1,
		Data: jbig2.TextRegion(w, h, ins, len(set.syms), sbStrips, sbDSOffset)}
	segs = tr.Append(segs)
	d := pdf.Dict{
		"Type":             pdf.Name("XObject"),
		"Subtype":          pdf.Name("Image"),
		"Width":            w,
		"Height":           h,
		"ImageMask":        true,
		"BitsPerComponent": 1,
		"Filter":           pdf.Name("JBIG2Decode"),
	}
	if useGlob {
		d["DecodeParms"] = pdf.Dict{"JBIG2Globals": glob}
	}
	return d, segs
}
