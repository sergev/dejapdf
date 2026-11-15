// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package pdf

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
)

// Writer writes a PDF file in one pass: streams as they come, other
// objects packed into object streams, then the page tree, catalog and a
// cross-reference stream (7.5.7, 7.5.8).
type Writer struct {
	w       *bufio.Writer
	pos     int
	xref    []entry // by object number
	pages   []Ref
	tree    Ref   // root of the page tree
	nodes   []Ref // its kids, pageFanout pages each
	pending []packed
	err     error
	buf     []byte
}

// entry is where an object is: at an offset, or in an object stream.
type entry struct {
	written bool
	stream  Ref // object stream, 0 if at an offset
	off     int // offset, or index in the object stream
}

// packed is an object waiting for its object stream.
type packed struct {
	r    Ref
	text []byte
}

// objStmSize is the number of objects per object stream.
const objStmSize = 100

// NewWriter starts a PDF 1.5 file.
func NewWriter(w io.Writer) *Writer {
	p := &Writer{w: bufio.NewWriterSize(w, 1<<16), xref: []entry{{written: true}}}
	p.raw([]byte("%PDF-1.5\n%\xe2\xe3\xcf\xd3\n"))
	p.tree = p.Alloc()
	return p
}

func (p *Writer) raw(b []byte) {
	if p.err != nil {
		return
	}
	n, err := p.w.Write(b)
	p.pos += n
	p.err = err
}

// Alloc reserves an object number.
func (p *Writer) Alloc() Ref {
	p.xref = append(p.xref, entry{})
	return Ref(len(p.xref) - 1)
}

func (p *Writer) check(r Ref) {
	if int(r) <= 0 || int(r) >= len(p.xref) || p.xref[r].written {
		panic(fmt.Sprintf("pdf: object %d not allocated or written twice", r))
	}
	p.xref[r].written = true
}

func (p *Writer) begin(r Ref) {
	p.check(r)
	p.xref[r].off = p.pos
	p.raw(fmt.Appendf(p.buf[:0], "%d 0 obj\n", int(r)))
}

// Object writes object r into an object stream.
func (p *Writer) Object(r Ref, v any) {
	p.check(r)
	p.pending = append(p.pending, packed{r, appendObject(nil, v)})
	if len(p.pending) == objStmSize {
		p.flushObjects()
	}
}

// flushObjects writes the pending objects as an object stream.
func (p *Writer) flushObjects() {
	if len(p.pending) == 0 {
		return
	}
	s := p.Alloc()
	var head, body []byte
	for i, o := range p.pending {
		head = fmt.Appendf(head, "%d %d ", int(o.r), len(body))
		body = append(append(body, o.text...), '\n')
		p.xref[o.r].stream, p.xref[o.r].off = s, i
	}
	d := Dict{"Type": Name("ObjStm"), "N": len(p.pending), "First": len(head),
		"Filter": Name("FlateDecode")}
	p.Stream(s, d, Flate(append(head, body...)))
	p.pending = p.pending[:0]
}

// Stream writes stream object r with already encoded data; d gets its
// Length.
func (p *Writer) Stream(r Ref, d Dict, data []byte) {
	p.begin(r)
	d["Length"] = len(data)
	p.buf = appendObject(p.buf[:0], d)
	delete(d, "Length")
	p.buf = append(p.buf, "\nstream\n"...)
	p.raw(p.buf)
	p.raw(data)
	p.raw([]byte("\nendstream\nendobj\n"))
}

// Add allocates and writes an object.
func (p *Writer) Add(v any) Ref {
	r := p.Alloc()
	p.Object(r, v)
	return r
}

// AddStream allocates and writes a stream.
func (p *Writer) AddStream(d Dict, data []byte) Ref {
	r := p.Alloc()
	p.Stream(r, d, data)
	return r
}

// Flate compresses data for /FlateDecode.
func Flate(data []byte) []byte {
	var b bytes.Buffer
	z, _ := zlib.NewWriterLevel(&b, zlib.BestCompression)
	z.Write(data)
	z.Close()
	return b.Bytes()
}

// Page is one page. Width and Height are in points. Content is the
// unencoded content stream. Ref, if not 0, is the page's allocated
// object; Annots are references to its annotations.
type Page struct {
	Ref           Ref
	Width, Height float64
	Rotate        int
	Resources     Dict
	Content       []byte
	Annots        Array
}

// pageFanout is the number of pages per intermediate page tree node.
const pageFanout = 64

// parent returns the page tree node of the next page.
func (p *Writer) parent() Ref {
	if len(p.pages)%pageFanout == 0 {
		p.nodes = append(p.nodes, p.Alloc())
	}
	return p.nodes[len(p.nodes)-1]
}

// AddPage writes a page and its content stream.
func (p *Writer) AddPage(pg Page) Ref {
	c := p.AddStream(Dict{"Filter": Name("FlateDecode")}, Flate(pg.Content))
	res := pg.Resources
	if res == nil {
		res = Dict{}
	}
	d := Dict{
		"Type":      Name("Page"),
		"Parent":    p.parent(),
		"MediaBox":  Array{0, 0, pg.Width, pg.Height},
		"Resources": res,
		"Contents":  c,
	}
	if pg.Rotate != 0 {
		d["Rotate"] = pg.Rotate
	}
	if len(pg.Annots) > 0 {
		d["Annots"] = pg.Annots
	}
	r := pg.Ref
	if r == 0 {
		r = p.Alloc()
	}
	p.Object(r, d)
	p.pages = append(p.pages, r)
	return r
}

// Close writes the page tree, the catalog, info (if not nil) and the
// cross-reference table. catalog holds extra catalog entries.
func (p *Writer) Close(catalog, info Dict) error {
	// Two levels: the root, then a node per pageFanout pages.
	var top Array
	for i, n := range p.nodes {
		pages := p.pages[i*pageFanout : min((i+1)*pageFanout, len(p.pages))]
		kids := make(Array, len(pages))
		for k, r := range pages {
			kids[k] = r
		}
		p.Object(n, Dict{"Type": Name("Pages"), "Parent": p.tree, "Kids": kids, "Count": len(pages)})
		top = append(top, n)
	}
	if top == nil {
		top = Array{}
	}
	p.Object(p.tree, Dict{"Type": Name("Pages"), "Kids": top, "Count": len(p.pages)})
	cat := Dict{"Type": Name("Catalog"), "Pages": p.tree}
	for k, v := range catalog {
		cat[k] = v
	}
	root := p.Add(cat)
	var ir Ref
	if info != nil {
		ir = p.Add(info)
	}
	return p.Finish(root, ir)
}

// PageTree returns the object reserved for the root of the page tree.
// A caller that writes its own page tree writes it there and ends with
// Finish rather than Close.
func (p *Writer) PageTree() Ref { return p.tree }

// Finish writes the cross-reference stream and trailer with catalog
// root and info (if not 0); every allocated object must be written.
func (p *Writer) Finish(root, info Ref) error {
	trailer := Dict{"Root": root}
	if info != 0 {
		trailer["Info"] = info
	}
	p.flushObjects()
	x := p.Alloc()
	p.xref[x].written = true
	p.xref[x].off = p.pos
	for i, e := range p.xref[1:] {
		if !e.written {
			return fmt.Errorf("pdf: object %d allocated but not written", i+1)
		}
	}
	// Rows of W [1 4 2]: type, offset or object stream, generation or index.
	rows := []byte{0, 0, 0, 0, 0, 0xff, 0xff}
	for _, e := range p.xref[1:] {
		if e.stream != 0 {
			rows = append(rows, 2, byte(e.stream>>24), byte(e.stream>>16), byte(e.stream>>8), byte(e.stream),
				byte(e.off>>8), byte(e.off))
		} else {
			rows = append(rows, 1, byte(e.off>>24), byte(e.off>>16), byte(e.off>>8), byte(e.off), 0, 0)
		}
	}
	trailer["Type"] = Name("XRef")
	trailer["Size"] = len(p.xref)
	trailer["W"] = Array{1, 4, 2}
	trailer["Filter"] = Name("FlateDecode")
	data := Flate(rows)
	trailer["Length"] = len(data)
	b := fmt.Appendf(nil, "%d 0 obj\n", int(x))
	b = appendObject(b, trailer)
	b = append(b, "\nstream\n"...)
	b = append(b, data...)
	b = fmt.Appendf(b, "\nendstream\nendobj\nstartxref\n%d\n%%%%EOF\n", p.xref[x].off)
	p.raw(b)
	if p.err == nil {
		p.err = p.w.Flush()
	}
	return p.err
}
