// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package raster

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/sergev/dejapdf/internal/pdf"
	"github.com/sergev/dejapdf/internal/pdfread"
	"github.com/sergev/dejapdf/internal/scan"
)

// TestPdfimages decodes the images to re-encode in the test files, and
// in $PDF_BOOKS, and compares them with pdfimages: equal for lossless
// sources, within 1 for JPEG. CMYK images are compared with -tiff,
// which keeps CMYK; the others with -png.
func TestPdfimages(t *testing.T) {
	if _, err := exec.LookPath("pdfimages"); err != nil {
		t.Skip("no pdfimages")
	}
	files, _ := filepath.Glob("../../testdata/pdf/*.pdf")
	books, _ := filepath.Glob(os.Getenv("PDF_BOOKS"))
	for _, path := range append(files, books...) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()
			checkFile(t, path)
		})
	}
}

func checkFile(t *testing.T, path string) {
	data, err := os.ReadFile(path)
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
	// The images to re-encode.
	var todo []*scan.Image
	seen := map[pdf.Ref]bool{}
	cmyk := false
	for i := range doc.Pages {
		p := &doc.Pages[i]
		for _, u := range p.Uses {
			if a, _ := doc.Plan(r, p, u); (a == scan.JBIG2 || a == scan.Separate) && !seen[u.Image.Ref] {
				seen[u.Image.Ref] = true
				todo = append(todo, u.Image)
				cmyk = cmyk || u.Image.Comps == 4 || u.Image.Color == "index"
			}
		}
	}
	if len(todo) == 0 {
		t.Skip("nothing to re-encode")
	}
	dir := t.TempDir()
	num := listNums(t, path)
	if out, err := exec.Command("pdfimages", "-png", path, filepath.Join(dir, "p")).CombinedOutput(); err != nil {
		t.Fatalf("pdfimages: %v\n%s", err, out)
	}
	if cmyk {
		exec.Command("pdfimages", "-tiff", path, filepath.Join(dir, "t")).Run()
	}
	stats := map[string]int{}
	worst := 0
	for _, im := range todo {
		n, ok := num[int(im.Ref)]
		if !ok {
			t.Errorf("object %d: not listed by pdfimages", im.Ref)
			continue
		}
		got, err := Decode(r, im)
		if err != nil {
			t.Errorf("object %d: %v", im.Ref, err)
			continue
		}
		tol := 0
		if im.Enc == "jpeg" {
			tol = 1
		}
		var bad, maxd int
		switch g := got.(type) {
		case *Bitmap:
			want := readPNG(t, dir, "p", n)
			// For a stencil pdfimages writes the samples, black where
			// 1, whatever /Decode says; ours is what is painted.
			flip := false
			if im.Stencil {
				d, _ := r.Resolve(im.Dict["Decode"]).(pdf.Array)
				flip = len(d) < 1 || r.Resolve(d[0]) != 1
			}
			bad, maxd = compareBitmap(g, want, flip)
			stats["bitmap"]++
		case *Pixmap:
			if g.N == 4 {
				want := readTIFF(t, filepath.Join(dir, fmt.Sprintf("t-%03d.tif", n)))
				bad, maxd = compareBytes(g.Pix, want, tol)
			} else {
				bad, maxd = comparePixmap(g, readPNG(t, dir, "p", n), tol)
			}
			stats[fmt.Sprintf("%s %d-component", im.Enc, g.N)]++
		}
		worst = max(worst, maxd)
		if bad > 0 {
			t.Errorf("object %d (%dx%d %s %s): %d values differ, by up to %d", im.Ref,
				im.Width, im.Height, im.Color, im.Enc, bad, maxd)
		}
	}
	t.Logf("%v, largest difference %d", stats, worst)
}

// listNums maps object numbers to the first image number pdfimages
// gives them.
func listNums(t *testing.T, path string) map[int]int {
	out, err := exec.Command("pdfimages", "-list", path).Output()
	if err != nil {
		t.Fatal(err)
	}
	m := map[int]int{}
	for _, line := range strings.Split(string(out), "\n")[2:] {
		f := strings.Fields(line)
		if len(f) < 12 || f[10] == "[inline]" || f[2] != "image" && f[2] != "stencil" {
			continue
		}
		n, _ := strconv.Atoi(f[1])
		obj, _ := strconv.Atoi(f[10])
		if _, ok := m[obj]; !ok {
			m[obj] = n
		}
	}
	return m
}

func readPNG(t *testing.T, dir, prefix string, n int) image.Image {
	f, err := os.Open(filepath.Join(dir, fmt.Sprintf("%s-%03d.png", prefix, n)))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

// readTIFF reads pdfimages' uncompressed 8-bit TIFF.
func readTIFF(t *testing.T, path string) []byte {
	d, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var e binary.ByteOrder = binary.LittleEndian
	if string(d[:2]) == "MM" {
		e = binary.BigEndian
	}
	off := int(e.Uint32(d[4:]))
	var offs, counts []int
	for i := range int(e.Uint16(d[off:])) {
		p := off + 2 + 12*i
		tag, typ, cnt := e.Uint16(d[p:]), e.Uint16(d[p+2:]), int(e.Uint32(d[p+4:]))
		vals := func() []int {
			size := 4
			if typ == 3 {
				size = 2
			}
			src := d[p+8:]
			if cnt*size > 4 {
				src = d[e.Uint32(d[p+8:]):]
			}
			out := make([]int, cnt)
			for k := range out {
				if size == 2 {
					out[k] = int(e.Uint16(src[2*k:]))
				} else {
					out[k] = int(e.Uint32(src[4*k:]))
				}
			}
			return out
		}
		switch tag {
		case 259:
			if vals()[0] != 1 {
				t.Fatalf("%s: compressed TIFF", path)
			}
		case 273:
			offs = vals()
		case 279:
			counts = vals()
		}
	}
	var pix []byte
	for i := range offs {
		pix = append(pix, d[offs[i]:offs[i]+counts[i]]...)
	}
	return pix
}

func compareBitmap(b *Bitmap, want image.Image, flip bool) (bad, maxd int) {
	if want.Bounds().Dx() != b.W || want.Bounds().Dy() != b.H {
		return b.W * b.H, 255
	}
	for y := range b.H {
		for x := range b.W {
			ink := b.Pix[y*b.Stride+x/8]>>(7-x%8)&1 == 1
			g := color.GrayModel.Convert(want.At(x, y)).(color.Gray).Y
			if ink != (g < 128) != flip {
				bad++
				maxd = 255
			}
		}
	}
	return bad, maxd
}

func comparePixmap(p *Pixmap, want image.Image, tol int) (bad, maxd int) {
	if want.Bounds().Dx() != p.W || want.Bounds().Dy() != p.H {
		return p.W * p.H, 255
	}
	ref := make([]byte, 0, len(p.Pix))
	for y := range p.H {
		for x := range p.W {
			c := want.At(x, y)
			if p.N == 1 {
				ref = append(ref, color.GrayModel.Convert(c).(color.Gray).Y)
			} else {
				rgba := color.RGBAModel.Convert(c).(color.RGBA)
				ref = append(ref, rgba.R, rgba.G, rgba.B)
			}
		}
	}
	return compareBytes(p.Pix, ref, tol)
}

func compareBytes(got, want []byte, tol int) (bad, maxd int) {
	if len(got) != len(want) {
		return max(len(got), len(want)), 255
	}
	for i := range got {
		d := int(got[i]) - int(want[i])
		d = max(d, -d)
		maxd = max(maxd, d)
		if d > tol {
			bad++
		}
	}
	return bad, maxd
}
