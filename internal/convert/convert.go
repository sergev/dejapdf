// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package convert

import (
	"fmt"
	"io"

	"github.com/sergev/dejapdf/internal/djvu"
	"github.com/sergev/dejapdf/internal/jb2"
	"github.com/sergev/dejapdf/internal/pdf"
)

// Options control the conversion.
type Options struct {
	Producer string // document info /Producer
}

// Convert writes doc as PDF to w.
func Convert(doc *djvu.Document, w io.Writer, opt Options) error {
	p := pdf.NewWriter(w)
	for i, pg := range doc.Pages {
		if err := page(p, doc, pg); err != nil {
			return fmt.Errorf("page %d: %w", i+1, err)
		}
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

func page(p *pdf.Writer, doc *djvu.Document, pg *djvu.Page) error {
	in := pg.Info
	if in.Width <= 0 || in.Height <= 0 {
		return fmt.Errorf("bad size %dx%d", in.Width, in.Height)
	}
	w := float64(in.Width) * 72 / float64(in.DPI)
	h := float64(in.Height) * 72 / float64(in.DPI)
	xobj := pdf.Dict{}
	var content []byte
	full := func(name string) {
		content = fmt.Appendf(content, "q %s 0 0 %s 0 0 cm /%s Do Q\n", num(w), num(h), name)
	}
	mask, err := doc.Mask(pg)
	if err != nil {
		return err
	}
	if mask != nil {
		xobj["Sm"] = stencil(p, mask.Render())
		content = append(content, "0 g\n"...)
		full("Sm")
	}
	res := pdf.Dict{}
	if len(xobj) > 0 {
		res["XObject"] = xobj
	}
	p.AddPage(pdf.Page{Width: w, Height: h, Rotate: in.Rotate, Resources: res, Content: content})
	return nil
}

// stencil writes a bitmap as a Flate-coded image mask that paints its
// black pixels.
func stencil(p *pdf.Writer, b *jb2.Bitmap) pdf.Ref {
	stride := (b.W + 7) / 8
	data := make([]byte, stride*b.H)
	for r := range b.H {
		row := data[r*stride:]
		for c, v := range b.Pix[r*b.W : (r+1)*b.W] {
			row[c>>3] |= v << (7 - c&7)
		}
	}
	return p.AddStream(pdf.Dict{
		"Type":             pdf.Name("XObject"),
		"Subtype":          pdf.Name("Image"),
		"Width":            b.W,
		"Height":           b.H,
		"ImageMask":        true,
		"BitsPerComponent": 1,
		"Decode":           pdf.Array{1, 0}, // 1 = black paints
		"Filter":           pdf.Name("FlateDecode"),
	}, pdf.Flate(data))
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
