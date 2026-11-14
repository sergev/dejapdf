// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package djvu

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sergev/dejapdf/internal/bzz"
	"github.com/sergev/dejapdf/internal/iff"
)

// seeds adds the data of the corpus chunks with the given ids,
// BZZ-decoded if z.
func seeds(f *testing.F, z bool, ids ...string) {
	files, _ := filepath.Glob(filepath.Join(corpus, "*.djvu"))
	for _, name := range files {
		b, err := os.ReadFile(name)
		if err != nil {
			f.Fatal(err)
		}
		root, err := iff.Parse(b)
		if err != nil {
			f.Fatal(err)
		}
		var walk func(c *iff.Chunk)
		walk = func(c *iff.Chunk) {
			for _, k := range c.Children {
				walk(k)
			}
			for _, id := range ids {
				if c.ID != id {
					continue
				}
				d := c.Data
				if z {
					if d, err = bzz.Decode(d); err != nil {
						f.Fatal(err)
					}
				}
				f.Add(d)
			}
		}
		walk(root)
	}
}

func FuzzParseText(f *testing.F) {
	seeds(f, true, "TXTz")
	f.Fuzz(func(t *testing.T, b []byte) { ParseText(b) })
}

func FuzzParseOutline(f *testing.F) {
	seeds(f, true, "NAVM")
	f.Fuzz(func(t *testing.T, b []byte) { ParseOutline(b) })
}

func FuzzParseSexps(f *testing.F) {
	seeds(f, true, "ANTz")
	f.Fuzz(func(t *testing.T, b []byte) {
		for _, s := range ParseSexps(b) {
			_ = s.String()
		}
	})
}

func FuzzParseDir(f *testing.F) {
	seeds(f, false, "DIRM")
	f.Fuzz(func(t *testing.T, b []byte) { ParseDir(b) })
}

func FuzzParsePalette(f *testing.F) {
	seeds(f, false, "FGbz")
	f.Fuzz(func(t *testing.T, b []byte) { parsePalette(b) })
}

func FuzzParseInfo(f *testing.F) {
	seeds(f, false, "INFO")
	f.Fuzz(func(t *testing.T, b []byte) { ParseInfo(b) })
}

// A whole document: structure, pages, text, annotations, outline and
// palette; the image layers have their own fuzz tests.
func FuzzParse(f *testing.F) {
	for _, n := range []string{"bundled.djvu", "pdf2djvu.djvu", "shared-dict.djvu", "palette.djvu"} {
		b, err := os.ReadFile(filepath.Join(corpus, n))
		if err != nil {
			f.Fatal(err)
		}
		f.Add(b)
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		d, err := Parse(b, nil)
		if err != nil {
			return
		}
		d.Outline()
		for _, p := range d.Pages {
			p.Text()
			p.Annotations()
			p.Palette()
		}
	})
}

func FuzzDecodeMMR(f *testing.F) {
	seeds(f, false, "Smmr")
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) >= 8 && (int(b[4])<<8|int(b[5]))*(int(b[6])<<8|int(b[7])) > 1<<20 {
			return // keep runs fast
		}
		DecodeMMR(b)
	})
}
