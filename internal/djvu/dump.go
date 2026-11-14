package djvu

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/sergev/dejapdf/internal/iff"
)

// Dump writes the chunk tree of a file in the format of djvudump.
func Dump(w io.Writer, root *iff.Chunk) error {
	var (
		dir     *Dir
		dirFull bool
	)
	if root.Name() == "FORM:DJVM" {
		if c := root.Find("DIRM"); c != nil {
			var err error
			dir, err = ParseDir(c.Data)
			dirFull = err == nil
		}
	}
	// Bundled components by offset, with their page numbers.
	comps := map[int]*Component{}
	pageNo := map[int]int{}
	if dir != nil && dir.Bundled && dirFull {
		n := 0
		for i := range dir.Components {
			c := &dir.Components[i]
			comps[c.Offset] = c
			if c.Type == PageComp {
				n++
				pageNo[c.Offset] = n
			}
		}
	}

	bw := bufio.NewWriter(w)
	var walk func(c *iff.Chunk, parent string, depth int)
	walk = func(c *iff.Chunk, parent string, depth int) {
		indent := strings.Repeat("  ", depth+1)
		head := fmt.Sprintf("%s [%d] ", c.Name(), c.Size())
		if c.Composite() {
			if k := comps[c.Offset]; k != nil {
				head += "{" + printable(k.ID) + "} [" + kindTag(k.Type, pageNo[c.Offset]) + "]"
				if k.HasTitle {
					head += " (" + printable(k.Title) + ")"
				}
			}
			fmt.Fprintf(bw, "%s%s\n", indent, head)
			for _, k := range c.Children {
				walk(k, c.Type, depth+1)
			}
			return
		}
		d := describe(c, parent)
		if c.ID == "DIRM" && dir != nil {
			kind := "indirect"
			if dir.Bundled {
				kind = "bundled"
			}
			d = fmt.Sprintf("Document directory (%s, %d files", kind, len(dir.Components))
			if dirFull {
				d += fmt.Sprintf(" %d pages", dir.NumPages())
			}
			d += ")"
		}
		if d == "" {
			fmt.Fprintf(bw, "%s%s\n", indent, head)
		} else {
			fmt.Fprintf(bw, "%s%-18s%s\n", indent, head, d)
		}
		if c.ID == "DIRM" && dir != nil && !dir.Bundled && dirFull {
			for _, k := range dir.Components {
				fmt.Fprintf(bw, "%s  %s -> %s\n", indent, printable(k.ID), printable(k.Name))
			}
		}
	}
	walk(root, "", 0)
	return bw.Flush()
}

// printable replaces each byte of invalid UTF-8 with '?', as djvudump does.
func printable(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && n == 1 {
			b.WriteByte('?')
		} else {
			b.WriteString(s[i : i+n])
		}
		i += n
	}
	return b.String()
}

func kindTag(t ComponentType, page int) string {
	switch t {
	case PageComp:
		return fmt.Sprintf("P%d", page)
	case Thumbnails:
		return "T"
	case SharedAnno:
		return "S"
	}
	return "I"
}

// describe returns the djvudump description of a data chunk.
func describe(c *iff.Chunk, parent string) string {
	b := c.Data
	switch c.ID {
	case "INFO":
		in, err := ParseInfo(b)
		if err != nil {
			return ""
		}
		return fmt.Sprintf("DjVu %dx%d, v%d, %d dpi, gamma=%d.%d",
			in.Width, in.Height, in.Version, in.DPI, in.Gamma/10, in.Gamma%10)
	case "Sjbz":
		return "JB2 bilevel data"
	case "Djbz":
		return "JB2 shared dictionary"
	case "Smmr":
		return "G4/MMR stencil data"
	case "FGbz":
		if len(b) < 3 {
			return ""
		}
		return fmt.Sprintf("JB2 colors data, v%d, %d colors", b[0]&0x7f, binary.BigEndian.Uint16(b[1:]))
	case "BG44", "FG44":
		return describeIW44(b)
	case "PM44", "BM44":
		if parent == c.ID {
			return describeIW44(b)
		}
	case "TH44":
		if parent == "THUM" {
			return "Thumbnail icon"
		}
	case "ANTa", "ANTz":
		return "Page annotation (hyperlinks, etc.)"
	case "TXTa", "TXTz":
		return "Hidden text (text, etc.)"
	case "INCL":
		return "Indirection chunk --> {" + string(b) + "}"
	}
	return ""
}

// describeIW44 summarises the header of an IW44 chunk.
func describeIW44(b []byte) string {
	if len(b) < 2 {
		return ""
	}
	s := fmt.Sprintf("IW4 data #%d, %d slices", int(b[0])+1, b[1])
	if b[0] == 0 && len(b) >= 8 {
		color := "color"
		if b[2]&0x80 != 0 {
			color = "b&w"
		}
		s += fmt.Sprintf(", v%d.%d (%s), %dx%d", b[2]&0x7f, b[3],
			color, binary.BigEndian.Uint16(b[4:]), binary.BigEndian.Uint16(b[6:]))
	}
	return s
}
