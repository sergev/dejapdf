// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package convert

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf16"

	"github.com/sergev/dejapdf/internal/djvu"
)

// text adds the page's hidden text as invisible text (render mode 3),
// in pixel units. Words of a line share its baseline and height; each
// is stretched (Tz) to reach the next word, to which a space joins it.
func text(b *pageBuilder, pg *djvu.Page) error {
	z, err := pg.Text()
	if err != nil || z == nil {
		return err
	}
	var c []byte
	x, y, size := 0, 0, 0
	line := func(ln *djvu.Zone, ws []*djvu.Zone) {
		for i, w := range ws {
			t := strings.TrimFunc(w.Text, func(r rune) bool { return unicode.IsSpace(r) || r < ' ' })
			y0, y1 := w.Y0, w.Y1
			if ln != nil {
				y0, y1 = ln.Y0, ln.Y1
			}
			if t == "" || w.X1 <= w.X0 || y1 <= y0 {
				continue
			}
			x1 := w.X1
			if i+1 < len(ws) && ws[i+1].X0 > w.X1 {
				t += " "
				x1 = ws[i+1].X0
			}
			u := utf16.Encode([]rune(t))
			if h := y1 - y0; h != size {
				c = fmt.Appendf(c, "/Tx %d Tf ", h)
				size = h
			}
			// Glyphs are size/2 wide.
			tz := (200*(x1-w.X0) + size*len(u)/2) / (size * len(u))
			c = fmt.Appendf(c, "%d %d Td %d Tz (", w.X0-x, y0-y, tz)
			x, y = w.X0, y0
			for _, v := range u {
				c = appendEscaped(c, byte(v>>8))
				c = appendEscaped(c, byte(v))
			}
			c = append(c, ")Tj\n"...)
		}
	}
	var walk func(z *djvu.Zone)
	walk = func(z *djvu.Zone) {
		if word(z) {
			line(nil, []*djvu.Zone{z})
			return
		}
		var ln *djvu.Zone
		if z.Type == djvu.ZLine {
			ln = z
		}
		var run []*djvu.Zone
		for _, k := range z.Children {
			if word(k) {
				run = append(run, k)
				continue
			}
			line(ln, run)
			run = nil
			walk(k)
		}
		line(ln, run)
	}
	walk(z)
	if len(c) == 0 {
		return nil
	}
	b.font = true
	s := num(72 / float64(b.in.DPI))
	b.content = fmt.Appendf(b.content, "q %s 0 0 %s 0 0 cm BT 3 Tr\n", s, s)
	b.content = append(b.content, c...)
	b.content = append(b.content, "ET Q\n"...)
	return nil
}

// appendEscaped appends a byte of a literal string.
func appendEscaped(b []byte, c byte) []byte {
	switch c {
	case '(', ')', '\\':
		return append(b, '\\', c)
	case '\r':
		return append(b, '\\', 'r')
	}
	return append(b, c)
}

// word reports whether a zone is drawn as one string.
func word(z *djvu.Zone) bool { return z.Type >= djvu.ZWord || len(z.Children) == 0 }
