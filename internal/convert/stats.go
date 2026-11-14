// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package convert

import (
	"fmt"
	"io"

	"github.com/sergev/dejapdf/internal/djvu"
	"github.com/sergev/dejapdf/internal/iff"
	"github.com/sergev/dejapdf/internal/pdf"
)

// Layer kinds of the size report.
const (
	Mask   = iota // JB2, Djbz, FGbz, MMR / JBIG2, CCITT
	Colour        // IW44, JPEG / JPX, DCT
	Text          // TXTz, TXTa / text operators
	Other         // the rest
	nKinds
)

var kindNames = [nKinds]string{"mask", "colour", "text", "other"}

// Stats are bytes by layer kind, in the DjVu input and the PDF output.
type Stats struct {
	DjVu, PDF [nKinds]int64
}

// Report prints the sizes as a table.
func (s *Stats) Report(w io.Writer) {
	var dt, pt int64
	fmt.Fprintf(w, "%-8s %12s %12s %6s\n", "layer", "djvu", "pdf", "ratio")
	for k := range nKinds {
		dt += s.DjVu[k]
		pt += s.PDF[k]
		fmt.Fprintf(w, "%-8s %12d %12d %6s\n", kindNames[k], s.DjVu[k], s.PDF[k], ratio(s.PDF[k], s.DjVu[k]))
	}
	fmt.Fprintf(w, "%-8s %12d %12d %6s\n", "total", dt, pt, ratio(pt, dt))
}

func ratio(a, b int64) string {
	if b == 0 {
		return "-"
	}
	return fmt.Sprintf("%.2f", float64(a)/float64(b))
}

var chunkKinds = map[string]int{
	"Sjbz": Mask, "Djbz": Mask, "FGbz": Mask, "Smmr": Mask,
	"BG44": Colour, "FG44": Colour, "BGjp": Colour, "FGjp": Colour, "BG2k": Colour, "FG2k": Colour,
	"TXTz": Text, "TXTa": Text,
}

// djvuSizes counts the chunks of the selected pages, each shared chunk
// once, and the document-level chunks (DIRM, NAVM) as other.
func djvuSizes(doc *djvu.Document, sel []int, s *Stats) {
	seen := map[*iff.Chunk]bool{}
	var walk func(c *iff.Chunk)
	walk = func(c *iff.Chunk) {
		if seen[c] {
			return
		}
		seen[c] = true
		if c.Composite() {
			s.DjVu[Other] += 12
			for _, k := range c.Children {
				walk(k)
			}
			return
		}
		n := int64(8 + len(c.Data) + len(c.Data)&1)
		if k, ok := chunkKinds[c.ID]; ok {
			s.DjVu[k] += n
		} else {
			s.DjVu[Other] += n
		}
	}
	s.DjVu[Other] += 4 // AT&T
	if doc.Root.Type == "DJVM" {
		s.DjVu[Other] += 12
		for _, c := range doc.Root.Children {
			if !c.Composite() {
				walk(c)
			}
		}
	}
	for _, i := range sel {
		if doc.Pages[i].Form != nil {
			walk(doc.Pages[i].Form)
		}
		for _, f := range doc.Pages[i].Includes {
			walk(f)
		}
	}
}

// streamKind is the layer kind of a stream written with dictionary d.
func streamKind(d pdf.Dict) int {
	switch d["Filter"] {
	case pdf.Name("JBIG2Decode"), pdf.Name("CCITTFaxDecode"):
		return Mask
	case pdf.Name("JPXDecode"), pdf.Name("DCTDecode"):
		return Colour
	}
	return Other
}

// counter counts bytes written.
type counter struct {
	w io.Writer
	n int64
}

func (c *counter) Write(b []byte) (int, error) {
	n, err := c.w.Write(b)
	c.n += int64(n)
	return n, err
}
