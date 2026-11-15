// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package convert

import (
	"encoding/binary"
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

// minRefined is the fewest refined shapes worth a refinement dictionary;
// tests lower it.
var minRefined = 100

// shared is a Djbz dictionary coded once as a JBIG2Globals stream.
// Its data is coded in the background: pages need only its numbering.
type shared struct {
	ids  []int           // symbol number of each shape, -1 for an empty one
	syms []*jbig2.Bitmap // the symbols by number
	segs int             // segments in data
	ref  pdf.Ref

	once  sync.Once
	done  chan struct{}
	coded []byte
}

// data waits for the coded segments.
func (s *shared) data() []byte {
	<-s.done
	return s.coded
}

// sharedDicts caches the document's coded Djbz dictionaries.
type sharedDicts struct {
	mu sync.Mutex
	m  map[*jb2.Dict]*shared
}

func (sd *sharedDicts) get(d *jb2.Dict) *shared {
	sd.mu.Lock()
	s, ok := sd.m[d]
	if !ok {
		s = &shared{}
		if sd.m == nil {
			sd.m = map[*jb2.Dict]*shared{}
		}
		sd.m[d] = s
	}
	sd.mu.Unlock()
	s.once.Do(func() { s.layout(d) })
	return s
}

// layout numbers d's symbols and starts coding them.
func (s *shared) layout(d *jb2.Dict) {
	var want []int
	for i, b := range d.Shapes {
		if b.W > 0 && b.H > 0 {
			want = append(want, i)
		}
	}
	set := codeShapes(want, func(i int) *jb2.Bitmap { return d.Shapes[i] },
		func(i int) jb2.Ref { return d.Refs[i] }, nil, nil, 0, 0)
	s.ids, s.syms, s.segs = make([]int, len(d.Shapes)), set.syms, set.nsegs
	for i := range s.ids {
		s.ids[i] = -1
	}
	for i, id := range set.ids {
		s.ids[i] = id
	}
	s.done = make(chan struct{})
	go func() {
		s.coded = set.codeParallel()
		close(s.done)
	}()
}

// symbolSet is shapes coded as up to two symbol dictionary segments:
// the directly coded shapes, then the refined ones.
type symbolSet struct {
	jobs  []func() []byte // each codes one segment
	nsegs int
	ids   map[int]int     // shape → symbol number, after the inputs
	syms  []*jbig2.Bitmap // inputs then the new symbols, by number
	refs  []uint32        // the segments a text region refers to
}

// code returns the segments.
func (set *symbolSet) code() []byte {
	var segs []byte
	for _, job := range set.jobs {
		segs = append(segs, job()...)
	}
	return segs
}

// codeParallel is code with the segments coded concurrently.
func (set *symbolSet) codeParallel() []byte {
	parts := make([][]byte, len(set.jobs))
	var wg sync.WaitGroup
	for k, job := range set.jobs {
		wg.Go(func() { parts[k] = job() })
	}
	wg.Wait()
	return slices.Concat(parts...)
}

// codeShapes lays out shapes want (in increasing order, refinement
// references included) as segments numbered from first, of page page;
// its jobs code them. Shapes refined from outside the set refer to the
// input symbols: ext gives their numbers, extSyms their bitmaps, extSegs
// the segments that define them.
func codeShapes(want []int, shape func(int) *jb2.Bitmap, ref func(int) jb2.Ref,
	ext func(int) int, extSyms []*jbig2.Bitmap, first uint32, page uint32, extSegs ...uint32) *symbolSet {
	set := &symbolSet{ids: map[int]int{}, syms: append([]*jbig2.Bitmap(nil), extSyms...),
		refs: append([]uint32(nil), extSegs...)}
	in := map[int]bool{}
	for _, i := range want {
		in[i] = true
	}
	// Only refinements that keep the reference's size and position:
	// Acrobat fails on the others (docs/JBIG2_Coding.md §8).
	refined := func(i int) bool {
		r := ref(i)
		if r.Shape < 0 || !in[r.Shape] && (ext == nil || ext(r.Shape) < 0) {
			return false
		}
		s, t := shape(i), shape(r.Shape)
		return s.W == t.W && s.H == t.H && r.DX == 0 && r.DY == 0
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
		ids := jbig2.SymbolIDs(bms)
		base := len(set.syms)
		set.syms = append(set.syms, make([]*jbig2.Bitmap, len(bms))...)
		for k, i := range direct {
			set.ids[i] = base + ids[k]
			set.syms[base+ids[k]] = bms[k]
		}
		num := n
		set.jobs = append(set.jobs, func() []byte {
			data, _ := jbig2.SymbolDict(bms)
			seg := jbig2.Segment{Number: num, Type: jbig2.TypeSymbolDict, Page: page, Data: data, Retain: true}
			return seg.Append(nil)
		})
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
		set.syms = append(append([]*jbig2.Bitmap(nil), inputs...), bms...)
		// The text region refers to the inputs too.
		num, segRefs := n, set.refs
		set.jobs = append(set.jobs, func() []byte {
			data := jbig2.RefineDict(bms, refs, inputs)
			seg := jbig2.Segment{Number: num, Type: jbig2.TypeSymbolDict, Refs: segRefs, Page: page, Data: data,
				Retain: true, KeepRefs: true}
			return seg.Append(nil)
		})
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
		src, dst := b.Pix[r*b.W:(r+1)*b.W], m.Pix[r*m.Stride:(r+1)*m.Stride]
		c := 0
		for ; c+8 <= len(src); c += 8 {
			// Byte i's low bit moves to bit 63−i: no two products overlap.
			v := binary.LittleEndian.Uint64(src[c:]) & 0x0101010101010101
			dst[c>>3] = byte(v * 0x8040201008040201 >> 56)
		}
		for ; c < len(src); c++ {
			dst[c>>3] |= src[c] << (7 - c&7)
		}
	}
	return m
}

// maskImage returns the blits i with keep(i) (all if keep is nil) as a
// w×h JBIG2 stencil whose bottom-left corner is mask pixel (x0, y0):
// dictionary and data, or nil if nothing would be drawn. Shapes of a Djbz
// go to its shared globals; the page's own shapes, and those they refine,
// go to page dictionaries (docs/JBIG2_Coding.md §10).
func (b *pageBuilder) maskImage(mask *jb2.Image, x0, y0, w, h int, keep func(int) bool) (pdf.Dict, []byte) {
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
	if mask.Dict == nil && len(mask.Shapes) == 1 && len(mask.Blits) == 1 && x0 == 0 && y0 == 0 {
		// One bitmap (an Smmr mask): a generic region.
		return maskDict(w, h), jbig2.GenericPage(pack(mask.RenderSome(w, h, nil)), 0)
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
	segs = append(segs, set.code()...)
	ins := make([]jbig2.Instance, len(blits))
	for j, i := range blits {
		bl := mask.Blits[i]
		id, ok := set.ids[bl.Shape]
		if !ok {
			id = glob.ids[bl.Shape]
		}
		ins[j] = jbig2.Instance{ID: id, S: bl.X - x0, T: h - 1 - (bl.Y - y0), W: mask.Shape(bl.Shape).W}
	}
	tr := jbig2.Segment{Number: n + 1 + uint32(set.nsegs), Type: jbig2.TypeTextRegion, Refs: set.refs, Page: 1,
		Data: jbig2.TextRegion(w, h, ins, len(set.syms), sbStrips, sbDSOffset)}
	segs = tr.Append(segs)
	d := maskDict(w, h)
	if useGlob {
		d["DecodeParms"] = pdf.Dict{"JBIG2Globals": glob}
	}
	return d, segs
}

func maskDict(w, h int) pdf.Dict {
	return pdf.Dict{
		"Subtype":   pdf.Name("Image"),
		"Width":     w,
		"Height":    h,
		"ImageMask": true,
		"Filter":    pdf.Name("JBIG2Decode"),
	}
}
