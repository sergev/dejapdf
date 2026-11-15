// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

// Package rewrite writes a PDF read by pdfread anew: the objects
// reachable from the catalog and the document information, renumbered,
// non-stream objects packed into object streams, and no encryption.
// Streams keep their encoded data.
package rewrite

import (
	"io"
	"maps"
	"math"
	"slices"
	"strconv"

	"github.com/sergev/dejapdf/internal/pdf"
	"github.com/sergev/dejapdf/internal/pdfread"
)

type copier struct {
	r     *pdfread.Reader
	w     *pdf.Writer
	refs  map[int]pdf.Ref // old object number to new
	queue []int

	rootNum int
	catalog pdf.Dict // the catalog, if changed
	repl    map[int]*Stream
	drop    map[int]bool     // pages and page tree nodes left out
	page    map[int]pdf.Dict // pages kept, in a new page tree
	step    func(int64)      // told the bytes of each stream copied
}

func (c *copier) stepped(n int64) {
	if c.step != nil {
		c.step(n)
	}
}

// parent is the /Parent of a kept page: the new page tree's root.
type parent struct{}

// Stream is a stream to write in place of an object, or as a new object
// where a dictionary refers to it. Its dictionary is in the numbering
// of the source.
type Stream struct {
	Dict pdf.Dict
	Data []byte // encoded

	ref pdf.Ref
}

// Copy writes the document of r to out.
func Copy(r *pdfread.Reader, out io.Writer) error {
	return Replace(r, out, nil)
}

// Replace is Copy with the stream objects in repl, by object number,
// replaced.
func Replace(r *pdfread.Reader, out io.Writer, repl map[int]*Stream) error {
	return Select(r, out, repl, nil, nil)
}

// Select is Replace keeping only the pages given, by index from 0, in
// that order (all if nil), in a page tree of one level. References to
// the pages left out become null; page labels, which count pages, are
// dropped. Step, if not nil, is told the bytes of each stream copied.
func Select(r *pdfread.Reader, out io.Writer, repl map[int]*Stream, pages []int, step func(int64)) error {
	c := &copier{r: r, w: pdf.NewWriter(out), refs: map[int]pdf.Ref{}, repl: repl, step: step}
	tr := r.Trailer()

	// The root of the page tree goes where the writer reserved it.
	cat, _ := r.Resolve(tr["Root"]).(pdf.Dict)
	if pages != nil {
		c.selectPages(cat, pages)
	} else if pr, ok := cat["Pages"].(pdf.Ref); ok && r.Get(int(pr)) != nil {
		c.refs[int(pr)] = c.w.PageTree()
		c.queue = append(c.queue, int(pr))
	} else {
		// A direct or missing page tree: written in its place.
		pages, _ := r.Resolve(cat["Pages"]).(pdf.Dict)
		if pages == nil {
			pages = pdf.Dict{"Type": pdf.Name("Pages"), "Kids": pdf.Array{}, "Count": 0}
		}
		c.w.Object(c.w.PageTree(), c.value(pages))
		c.catalog = cloneDict(cat)
		delete(c.catalog, "Pages") // set after renumbering
	}
	// The writer's header says 1.5; a newer version goes into the
	// catalog (7.7.2), unless it has a newer one.
	if v := r.Version(); v > "1.5" {
		if cv, _ := r.Resolve(cat["Version"]).(pdf.Name); string(cv) < v {
			if c.catalog == nil {
				c.catalog = cloneDict(cat)
			}
			c.catalog["Version"] = pdf.Name(v)
		}
	}
	var root pdf.Ref
	if rr, ok := tr["Root"].(pdf.Ref); ok {
		c.rootNum = int(rr)
		root = c.ref(c.rootNum)
	} else {
		root = c.w.Add(c.catalogValue(cat))
	}
	var info pdf.Ref
	switch v := tr["Info"].(type) {
	case pdf.Ref:
		if _, ok := r.Resolve(v).(pdf.Dict); ok {
			info = c.ref(int(v))
		}
	case pdf.Dict:
		info = c.w.Add(c.value(v))
	}
	for len(c.queue) > 0 {
		n := c.queue[0]
		c.queue = c.queue[1:]
		c.write(n)
	}
	return c.w.Finish(root, info)
}

// cloneDict copies a dictionary shallowly.
func cloneDict(d pdf.Dict) pdf.Dict {
	out := make(pdf.Dict, len(d))
	for k, v := range d {
		out[k] = v
	}
	return out
}

// ref returns the new number of object n, queueing it on first use;
// 0 for a missing object, which reads as null.
func (c *copier) ref(n int) pdf.Ref {
	if r, ok := c.refs[n]; ok {
		return r
	}
	if c.r.Get(n) == nil {
		return 0
	}
	r := c.w.Alloc()
	c.refs[n] = r
	c.queue = append(c.queue, n)
	return r
}

// write writes object n.
func (c *copier) write(n int) {
	r := c.refs[n]
	v := c.r.Get(n)
	if n == c.rootNum {
		c.w.Object(r, c.catalogValue(v))
		return
	}
	if d, ok := c.page[n]; ok {
		c.w.Object(r, c.value(d))
		return
	}
	if s, ok := c.repl[n]; ok {
		if _, ok := v.(*pdfread.Stream); ok {
			c.w.Stream(r, c.value(s.Dict).(pdf.Dict), s.Data)
			c.stepped(int64(len(s.Data)))
			return
		}
	}
	switch v := v.(type) {
	case *pdfread.Stream:
		d := c.value(v.Dict).(pdf.Dict)
		delete(d, "Length")
		dropCrypt(d)
		raw := c.r.Raw(v)
		c.w.Stream(r, d, raw)
		c.stepped(int64(len(raw)))
	default:
		c.w.Object(r, c.value(v))
	}
}

// catalogValue copies the catalog with the changes made to it.
func (c *copier) catalogValue(cat any) any {
	if c.catalog == nil {
		return c.value(cat)
	}
	d := c.value(c.catalog).(pdf.Dict)
	if _, ok := c.catalog["Pages"]; !ok {
		d["Pages"] = c.w.PageTree()
	}
	return d
}

// dropCrypt removes a /Crypt filter: the output is not encrypted.
func dropCrypt(d pdf.Dict) {
	switch f := d["Filter"].(type) {
	case pdf.Name:
		if f == "Crypt" {
			delete(d, "Filter")
			delete(d, "DecodeParms")
		}
	case pdf.Array:
		if len(f) > 0 && f[0] == pdf.Name("Crypt") {
			d["Filter"] = f[1:]
			if p, ok := d["DecodeParms"].(pdf.Array); ok && len(p) > 0 {
				d["DecodeParms"] = p[1:]
			}
		}
	}
}

// value deep-copies a direct value, renumbering references. Reals are
// written exactly, not rounded as the writer does.
func (c *copier) value(v any) any {
	switch v := v.(type) {
	case pdf.Ref:
		if c.drop[int(v)] {
			return nil
		}
		if r := c.ref(int(v)); r != 0 {
			return r
		}
		return nil
	case float64:
		if math.IsInf(v, 0) || math.IsNaN(v) {
			v = math.Copysign(math.MaxFloat32, v)
		}
		return pdf.Raw(strconv.FormatFloat(v, 'f', -1, 64))
	case pdf.Array:
		out := make(pdf.Array, len(v))
		for i, e := range v {
			out[i] = c.value(e)
		}
		return out
	case pdf.Dict:
		// Sorted, so that objects are numbered the same every time.
		out := make(pdf.Dict, len(v))
		for _, k := range slices.Sorted(maps.Keys(v)) {
			out[k] = c.value(v[k])
		}
		return out
	case bool, int, pdf.Name, pdf.String:
		return v
	case parent:
		return c.w.PageTree()
	case *Stream:
		if v.ref == 0 {
			v.ref = c.w.AddStream(c.value(v.Dict).(pdf.Dict), v.Data)
		}
		return v.ref
	}
	// A stream where a direct object should be, or anything else.
	return nil
}

// selectPages writes a page tree of the pages given and arranges for
// the rest of the old one to be left out. Each kept page gets the
// attributes it inherited.
func (c *copier) selectPages(cat pdf.Dict, sel []int) {
	type leaf struct {
		ref  pdf.Ref
		dict pdf.Dict
	}
	var leaves []leaf
	c.drop = map[int]bool{}
	c.page = map[int]pdf.Dict{}
	inheritable := []string{"Resources", "MediaBox", "CropBox", "Rotate"}
	var walk func(v any, in pdf.Dict, depth int)
	walk = func(v any, in pdf.Dict, depth int) {
		ref, _ := v.(pdf.Ref)
		if ref != 0 {
			if c.drop[int(ref)] || depth > 64 {
				return
			}
			c.drop[int(ref)] = true
		}
		d, ok := c.r.Resolve(v).(pdf.Dict)
		if !ok {
			return
		}
		kids, isNode := c.r.Resolve(d["Kids"]).(pdf.Array)
		if !isNode || d["Type"] == pdf.Name("Page") {
			p := cloneDict(d)
			for _, k := range inheritable {
				if _, ok := p[k]; !ok && in[k] != nil {
					p[k] = in[k]
				}
			}
			p["Parent"] = parent{}
			leaves = append(leaves, leaf{ref, p})
			return
		}
		next := cloneDict(in)
		for _, k := range inheritable {
			if v, ok := d[k]; ok {
				next[k] = v
			}
		}
		for _, k := range kids {
			walk(k, next, depth+1)
		}
	}
	walk(cat["Pages"], pdf.Dict{}, 0)
	kids := pdf.Array{}
	for _, i := range sel {
		if i < 0 || i >= len(leaves) {
			continue
		}
		l := leaves[i]
		if l.ref == 0 {
			kids = append(kids, c.w.Add(c.value(l.dict)))
			continue
		}
		delete(c.drop, int(l.ref))
		c.page[int(l.ref)] = l.dict
		kids = append(kids, c.ref(int(l.ref)))
	}
	c.w.Object(c.w.PageTree(), pdf.Dict{"Type": pdf.Name("Pages"), "Kids": kids, "Count": len(kids)})
	c.catalog = cloneDict(cat)
	delete(c.catalog, "Pages")
	delete(c.catalog, "PageLabels")
}
