package pdf

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
)

// Writer writes a PDF file in one pass: objects as they come, then the
// page tree, catalog, cross-reference table and trailer.
type Writer struct {
	w       *bufio.Writer
	pos     int
	offsets []int // by object number; -1 = allocated, not written
	pages   []Ref
	tree    Ref   // root of the page tree
	nodes   []Ref // its kids, pageFanout pages each
	err     error
	buf     []byte
}

// NewWriter starts a PDF 1.5 file.
func NewWriter(w io.Writer) *Writer {
	p := &Writer{w: bufio.NewWriterSize(w, 1<<16), offsets: []int{0}}
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
	p.offsets = append(p.offsets, -1)
	return Ref(len(p.offsets) - 1)
}

func (p *Writer) begin(r Ref) {
	if int(r) <= 0 || int(r) >= len(p.offsets) || p.offsets[r] >= 0 {
		panic(fmt.Sprintf("pdf: object %d not allocated or written twice", r))
	}
	p.offsets[r] = p.pos
	p.raw(fmt.Appendf(p.buf[:0], "%d 0 obj\n", int(r)))
}

// Object writes object r.
func (p *Writer) Object(r Ref, v any) {
	p.begin(r)
	p.buf = appendObject(p.buf[:0], v)
	p.buf = append(p.buf, "\nendobj\n"...)
	p.raw(p.buf)
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
	z, _ := zlib.NewWriterLevel(&b, zlib.DefaultCompression)
	z.Write(data)
	z.Close()
	return b.Bytes()
}

// Page is one page. Width and Height are in points. Content is the
// unencoded content stream.
type Page struct {
	Width, Height float64
	Rotate        int
	Resources     Dict
	Content       []byte
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
	r := p.Add(d)
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
	trailer := Dict{"Root": root}
	if info != nil {
		trailer["Info"] = p.Add(info)
	}
	for i, off := range p.offsets[1:] {
		if off < 0 {
			return fmt.Errorf("pdf: object %d allocated but not written", i+1)
		}
	}
	xref := p.pos
	b := fmt.Appendf(nil, "xref\n0 %d\n0000000000 65535 f \n", len(p.offsets))
	for _, off := range p.offsets[1:] {
		b = fmt.Appendf(b, "%010d 00000 n \n", off)
	}
	trailer["Size"] = len(p.offsets)
	b = append(b, "trailer\n"...)
	b = appendObject(b, trailer)
	b = fmt.Appendf(b, "\nstartxref\n%d\n%%%%EOF\n", xref)
	p.raw(b)
	if p.err == nil {
		p.err = p.w.Flush()
	}
	return p.err
}
