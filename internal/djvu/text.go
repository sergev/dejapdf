package djvu

import (
	"errors"
	"fmt"

	"github.com/sergev/dejapdf/internal/bzz"
	"github.com/sergev/dejapdf/internal/iff"
)

// ZoneType is the kind of a hidden-text zone.
type ZoneType int

const (
	ZPage ZoneType = 1 + iota
	ZColumn
	ZRegion
	ZParagraph
	ZLine
	ZWord
	ZChar
)

var zoneNames = [...]string{"", "page", "column", "region", "para", "line", "word", "char"}

// String returns the name djvused uses.
func (t ZoneType) String() string {
	if t >= ZPage && t <= ZChar {
		return zoneNames[t]
	}
	return fmt.Sprintf("zone%d", int(t))
}

// Zone is a hidden-text zone. Coordinates are pixels from the bottom
// left of the unrotated page; X1, Y1 are exclusive.
type Zone struct {
	Type           ZoneType
	X0, Y0, X1, Y1 int
	Text           string // the zone's slice of the page text, separators included
	Children       []*Zone
}

const zoneSize = 17

// ParseText decodes uncompressed TXTa/TXTz data (spec 8.3.5).
func ParseText(b []byte) (*Zone, error) {
	if len(b) < 3 {
		return nil, errors.New("djvu: text too short")
	}
	n := u24(b)
	if 3+n+1 > len(b) {
		return nil, errors.New("djvu: text truncated")
	}
	t := &textParser{text: b[3 : 3+n], b: b[3+n+1:]}
	if len(t.b) == 0 {
		return nil, nil // text without zones
	}
	z, err := t.zone(nil, nil, 0)
	if err != nil {
		return nil, err
	}
	return z.Zone, nil
}

type textParser struct {
	text []byte
	b    []byte
}

// placed is a zone with its text offset.
type placed struct {
	*Zone
	start int
}

const maxZoneDepth = 32

// zone reads a zone and its children. Zones are placed relative to
// the parent's upper-left corner (first child) or to the previous
// sibling: page, paragraph and line from its lower-left corner, y
// down; column, region, word and character from its lower-right
// corner, y up.
func (t *textParser) zone(parent, prev *placed, depth int) (*placed, error) {
	if depth > maxZoneDepth {
		return nil, errors.New("djvu: text zones nested too deep")
	}
	if len(t.b) < zoneSize {
		return nil, errors.New("djvu: text zone truncated")
	}
	r := t.b[:zoneSize]
	t.b = t.b[zoneSize:]
	typ := ZoneType(r[0])
	x, y, w, h, start := s16(r[1:]), s16(r[3:]), s16(r[5:]), s16(r[7:]), s16(r[9:])
	length, nkids := u24(r[11:]), u24(r[14:])
	switch {
	case prev != nil:
		start += prev.start + len(prev.Text)
		if typ == ZColumn || typ == ZRegion || typ == ZWord || typ == ZChar {
			x += prev.X1
			y += prev.Y0
		} else {
			x += prev.X0
			y = prev.Y0 - y - h
		}
	case parent != nil:
		start += parent.start
		x += parent.X0
		y = parent.Y1 - y - h
	}
	if start < 0 || length < 0 || start+length > len(t.text) {
		return nil, fmt.Errorf("djvu: text zone [%d,+%d) outside %d bytes", start, length, len(t.text))
	}
	if nkids*zoneSize > len(t.b) {
		return nil, errors.New("djvu: text zone children truncated")
	}
	z := &placed{&Zone{Type: typ, X0: x, Y0: y, X1: x + w, Y1: y + h,
		Text: string(t.text[start : start+length])}, start}
	var k *placed
	for range nkids {
		var err error
		if k, err = t.zone(z, k, depth+1); err != nil {
			return nil, err
		}
		z.Children = append(z.Children, k.Zone)
	}
	return z, nil
}

// Text returns the page's hidden text, or nil if it has none.
func (p *Page) Text() (*Zone, error) {
	return textOf(append([]*iff.Chunk{p.Form}, p.Includes...)...)
}

func textOf(forms ...*iff.Chunk) (*Zone, error) {
	for _, f := range forms {
		for _, c := range f.Children {
			switch c.ID {
			case "TXTa":
				return ParseText(c.Data)
			case "TXTz":
				b, err := bzz.Decode(c.Data)
				if err != nil {
					return nil, fmt.Errorf("djvu: TXTz: %w", err)
				}
				return ParseText(b)
			}
		}
	}
	return nil, nil
}

func u24(b []byte) int { return int(b[0])<<16 | int(b[1])<<8 | int(b[2]) }

func s16(b []byte) int { return int(b[0])<<8 | int(b[1]) - 0x8000 }
