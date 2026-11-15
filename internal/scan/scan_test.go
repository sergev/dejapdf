// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package scan

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/sergev/dejapdf/internal/pdf"
	"github.com/sergev/dejapdf/internal/pdfread"
)

// row is a line of pdfimages -list, without the size columns.
type row struct {
	page          int
	typ           string
	width, height int
	color         string
	comps, bpc    int
	enc           string
	object        string
	xppi, yppi    int
}

func (r row) key() string {
	return fmt.Sprintf("%s %dx%d %s %d %d %s %s", r.typ, r.width, r.height, r.color, r.comps, r.bpc, r.enc, r.object)
}

// pdfimagesRows runs pdfimages -list.
func pdfimagesRows(t *testing.T, path string) []row {
	out, err := exec.Command("pdfimages", "-list", path).Output()
	if err != nil {
		t.Fatalf("pdfimages: %v", err)
	}
	var rows []row
	for _, line := range strings.Split(string(out), "\n")[2:] {
		f := strings.Fields(line)
		if len(f) < 14 {
			continue
		}
		atoi := func(s string) int { n, _ := strconv.Atoi(s); return n }
		r := row{page: atoi(f[0]), typ: f[2], width: atoi(f[3]), height: atoi(f[4]), color: f[5],
			comps: atoi(f[6]), bpc: atoi(f[7]), enc: f[8]}
		if f[10] == "[inline]" {
			r.object = "inline"
			r.xppi, r.yppi = atoi(f[11]), atoi(f[12])
		} else {
			r.object = f[10]
			r.xppi, r.yppi = atoi(f[12]), atoi(f[13])
		}
		rows = append(rows, r)
	}
	return rows
}

// ourRows lists the images of doc as pdfimages does: each drawing, then
// its soft mask or mask.
func ourRows(r *pdfread.Reader, doc *Document) []row {
	var rows []row
	w := &walker{r: r, doc: doc}
	add := func(p int, u Use, im *Image, typ string) {
		obj := "inline"
		if !u.Inline {
			obj = strconv.Itoa(int(im.Ref))
		}
		x := float64(im.Width) * 72 / math.Hypot(u.Matrix[0], u.Matrix[1])
		y := float64(im.Height) * 72 / math.Hypot(u.Matrix[2], u.Matrix[3])
		rows = append(rows, row{p, typ, im.Width, im.Height, im.Color, im.Comps, im.BPC, im.Enc,
			obj, int(x + 0.5), int(y + 0.5)})
	}
	for _, p := range doc.Pages {
		for _, u := range p.Uses {
			im := u.Image
			typ := "image"
			if im.Stencil {
				typ = "stencil"
			}
			add(p.Num, u, im, typ)
			for _, m := range []struct {
				ref any
				typ string
			}{{im.SMask, "smask"}, {im.Mask, "mask"}} {
				ref, _ := m.ref.(pdf.Ref)
				if ref == 0 {
					continue
				}
				if s, ok := r.Resolve(ref).(*pdfread.Stream); ok {
					mi := w.image(s.Dict, nil)
					mi.Ref = im.Ref // pdfimages shows the image's number
					if m.typ == "mask" {
						mi.Color, mi.Comps = "-", 1
					}
					add(p.Num, u, mi, m.typ)
				}
			}
		}
	}
	return rows
}

// compare reports the rows that differ, by page; ppi may differ by 1.
func compare(t *testing.T, name string, got, want []row) int {
	bad := 0
	byPage := func(rows []row) map[int][]row {
		m := map[int][]row{}
		for _, r := range rows {
			m[r.page] = append(m[r.page], r)
		}
		return m
	}
	g, w := byPage(got), byPage(want)
	var pages []int
	for p := range g {
		pages = append(pages, p)
	}
	for p := range w {
		if _, ok := g[p]; !ok {
			pages = append(pages, p)
		}
	}
	slices.Sort(pages)
	near := func(a, b int) bool { return a-b <= 1 && b-a <= 1 }
	for _, p := range pages {
		left := slices.Clone(w[p])
		var extra []row
		for _, r := range g[p] {
			i := slices.IndexFunc(left, func(x row) bool {
				return x.key() == r.key() && near(x.xppi, r.xppi) && near(x.yppi, r.yppi)
			})
			if i < 0 {
				extra = append(extra, r)
				continue
			}
			left = slices.Delete(left, i, i+1)
		}
		if len(extra) > 0 || len(left) > 0 {
			if bad < 10 {
				t.Errorf("%s: page %d: ours %v, pdfimages %v", name, p, extra, left)
			}
			bad++
		}
	}
	return bad
}

func checkFile(t *testing.T, path string) *Document {
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	r, err := pdfread.Open(data)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := Find(r)
	if err != nil {
		t.Fatal(err)
	}
	if bad := compare(t, filepath.Base(path), ourRows(r, doc), pdfimagesRows(t, path)); bad > 0 {
		t.Errorf("%s: %d pages differ from pdfimages", filepath.Base(path), bad)
	}
	return doc
}

// TestPdfimages compares the images found in the test files, and in
// $PDF_BOOKS, with pdfimages -list.
func TestPdfimages(t *testing.T) {
	if _, err := exec.LookPath("pdfimages"); err != nil {
		t.Skip("no pdfimages")
	}
	files, _ := filepath.Glob("../../testdata/pdf/*.pdf")
	books, _ := filepath.Glob(os.Getenv("PDF_BOOKS"))
	for _, path := range append(files, books...) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()
			doc := checkFile(t, path)
			scanned, text, over := 0, 0, 0
			for _, p := range doc.Pages {
				if p.Scanned {
					scanned++
					if p.Text > 0 {
						text++
					}
					if p.TextOver > 0 {
						over++
					}
				}
			}
			t.Logf("%d of %d pages scanned, %d with visible text, %d over the image", scanned, len(doc.Pages), text, over)
		})
	}
}

// TestPlan checks what would be done with the scans of the test files.
func TestPlan(t *testing.T) {
	want := map[string]string{
		"g4": "JBIG2", "g4-book": "JBIG2 JBIG2", "g3": "JBIG2", "g3-align": "JBIG2",
		"g3-2d": "keep", "imagemask": "JBIG2", "flate1": "JBIG2", "flate8": "separate",
		"jpeg-gray": "separate", "jpeg-rgb": "separate", "jpeg-cmyk": "separate",
		"indexed": "separate", "inline": "keep", "ocr": "JBIG2 separate JBIG2",
		"rotated": "JBIG2 separate JBIG2", "links": "JBIG2 separate separate JBIG2",
		"incremental": "JBIG2 JBIG2", "filters": "separate separate",
		"encrypt-rc4-40": "JBIG2", "encrypt-aes-256": "JBIG2",
	}
	for name, w := range want {
		data, err := os.ReadFile("../../testdata/pdf/" + name + ".pdf")
		if err != nil {
			t.Fatal(err)
		}
		r, err := pdfread.Open(data)
		if err != nil {
			t.Fatal(err)
		}
		doc, err := Find(r)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for i := range doc.Pages {
			p := &doc.Pages[i]
			for _, u := range p.Uses {
				if a, _ := doc.Plan(r, p, u); a != None {
					got = append(got, a.String())
				}
			}
		}
		if s := strings.Join(got, " "); s != w {
			t.Errorf("%s: %s, want %s", name, s, w)
		}
	}
}
