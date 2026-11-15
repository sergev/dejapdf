// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package reencode

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sergev/dejapdf/internal/jb2"
	"github.com/sergev/dejapdf/internal/pdf"
	"github.com/sergev/dejapdf/internal/pdfread"
	"github.com/sergev/dejapdf/internal/raster"
	"github.com/sergev/dejapdf/internal/scan"
	"github.com/sergev/dejapdf/internal/segment"
	"github.com/sergev/dejapdf/internal/shapes"
	"github.com/sergev/dejapdf/internal/stencil"
)

// TestBackground codes a gray page with text on a gradient and a white
// one: the first as JPX that opj_decompress decodes to the paper, the
// second as a fill.
func TestBackground(t *testing.T) {
	const w, h, dpi = 600, 450, 300
	p := &raster.Pixmap{W: w, H: h, N: 1, Pix: make([]byte, w*h)}
	for y := range h {
		for x := range w {
			v := byte(180 + 60*x/w)
			if y%45 >= 15 && y%45 < 30 && x%10 < 3 {
				v = 20
			}
			p.Pix[y*w+x] = v
		}
	}
	m := segment.Mask(p, dpi)
	l, err := background(p, m, dpi, 100000)
	if err != nil || l.uniform {
		t.Fatalf("background: %v, uniform %v", err, l != nil && l.uniform)
	}
	if l.dict["Width"] != 200 || l.dict["Height"] != 150 || l.dict["ColorSpace"] != pdf.Name("DeviceGray") {
		t.Errorf("dictionary %v", l.dict)
	}
	t.Logf("%d bytes", len(l.data))
	if _, err := exec.LookPath("opj_decompress"); err == nil {
		dir := t.TempDir()
		in, out := filepath.Join(dir, "b.jp2"), filepath.Join(dir, "b.pgm")
		os.WriteFile(in, l.data, 0o644)
		if b, err := exec.Command("opj_decompress", "-i", in, "-o", out).CombinedOutput(); err != nil {
			t.Fatalf("opj_decompress: %v\n%s", err, b)
		}
		data, _ := os.ReadFile(out)
		pix := data[len(data)-200*150:]
		worst := 0
		for y := range 150 {
			for x := range 200 {
				want := 180 + 60*(3*x+1)/w
				worst = max(worst, abs(int(pix[y*200+x])-want))
			}
		}
		if worst > 6 {
			t.Errorf("background off the paper by %d", worst)
		}
	}

	for i := range p.Pix {
		p.Pix[i] = 250
	}
	l, err = background(p, segment.Mask(p, dpi), dpi, 1000)
	if err != nil || !l.uniform || l.fill != (segment.Color{250, 250, 250}) {
		t.Errorf("white page: %v %+v", err, l)
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// TestSeparate reports what gray and colour scans of PDF_BOOKS come to
// as a mask (in groups, as for bilevel scans) and a background: pages
// PAGES (first-last, default 1-20).
func TestSeparate(t *testing.T) {
	glob := os.Getenv("PDF_BOOKS")
	if glob == "" {
		t.Skip("set PDF_BOOKS")
	}
	first, last := 1, 20
	if s := os.Getenv("PAGES"); s != "" {
		fmt.Sscanf(strings.ReplaceAll(s, "-", " "), "%d %d", &first, &last)
	}
	files, _ := filepath.Glob(glob)
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		r, err := pdfread.Open(data)
		if err != nil {
			t.Fatal(err)
		}
		doc, err := scan.Find(r)
		if err != nil {
			t.Fatal(err)
		}
		var js []*jb2.Image
		var orig, bg, fills int
		for pg := first; pg <= min(last, len(doc.Pages)); pg++ {
			p := &doc.Pages[pg-1]
			for _, u := range p.Uses {
				if a, _ := doc.Plan(r, p, u); a != scan.Separate {
					continue
				}
				v, err := raster.Decode(r, u.Image)
				if err != nil {
					t.Fatal(err)
				}
				px := v.(*raster.Pixmap)
				n := len(r.Raw(r.Get(int(u.Image.Ref)).(*pdfread.Stream)))
				m := segment.Mask(px, u.DPIX)
				js = append(js, shapes.Split(m))
				l, err := background(px, m, u.DPIX, int(DefaultQuality*float64(n)))
				if err != nil {
					t.Fatal(err)
				}
				orig += n
				bg += len(l.data)
				if l.uniform {
					fills++
				}
			}
		}
		masks := map[bool]int{}
		for _, lossy := range []bool{false, true} {
			cp := make([]*jb2.Image, len(js))
			for i, j := range js {
				c := *j
				c.Shapes = append([]*jb2.Bitmap(nil), j.Shapes...)
				c.Blits = append([]jb2.Blit(nil), j.Blits...)
				cp[i] = &c
			}
			globs := map[*stencil.Globals]bool{}
			for k := 0; k < len(cp); k += GroupSize {
				g := cp[k:min(k+GroupSize, len(cp))]
				share(g, lossy)
				cache := &stencil.Cache{}
				for _, j := range g {
					d, gl := code(j, cache)
					masks[lossy] += len(d)
					if gl != nil && !globs[gl] {
						globs[gl] = true
						masks[lossy] += len(gl.Data())
					}
				}
			}
		}
		ratio := func(n int) float64 { return float64(orig) / float64(max(n, 1)) }
		t.Logf("%s: %d images %d KB; background %d KB (%d fills); mask %d KB lossless (%.1fx), %d KB lossy (%.1fx)",
			filepath.Base(f), len(js), orig>>10, bg>>10, fills, masks[false]>>10, ratio(masks[false]+bg),
			masks[true]>>10, ratio(masks[true]+bg))
	}
}
