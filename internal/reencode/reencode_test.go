// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package reencode

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/sergev/dejapdf/internal/pdf"
	"github.com/sergev/dejapdf/internal/pdfread"
	"github.com/sergev/dejapdf/internal/raster"
	"github.com/sergev/dejapdf/internal/scan"
)

// TestJbig2dec re-encodes the test files, and $PDF_BOOKS, and checks
// that the output passes qpdf --check, is smaller, and that each
// replaced image decodes in jbig2dec to the original's bitmap, with the
// /Decode that draws it the same.
func TestJbig2dec(t *testing.T) {
	for _, tool := range []string{"jbig2dec", "qpdf"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip("no " + tool)
		}
	}
	files, _ := filepath.Glob("../../testdata/pdf/*.pdf")
	books, _ := filepath.Glob(os.Getenv("PDF_BOOKS"))
	for _, path := range append(files, books...) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()
			check(t, path)
		})
	}
}

func check(t *testing.T, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	r, doc := open(t, data)
	var out bytes.Buffer
	st, err := Reencode(r, doc, &out, Options{Warn: func(err error) { t.Error(err) }})
	if err != nil {
		t.Fatal(err)
	}
	if st.Images == 0 {
		t.Skip("no bilevel scans")
	}
	if out.Len() > len(data) {
		t.Errorf("grew from %d to %d", len(data), out.Len())
	}
	dir := t.TempDir()
	name := filepath.Join(dir, "out.pdf")
	os.WriteFile(name, out.Bytes(), 0o644)
	if b, err := exec.Command("qpdf", "--check", name).CombinedOutput(); err != nil {
		// Some originals have warnings too: no worse is enough.
		if exec.Command("qpdf", "--check", path).Run() == nil {
			t.Errorf("qpdf --check: %v\n%s", err, b)
		}
	}

	// Pages and their image uses correspond, in order.
	r2, doc2 := open(t, out.Bytes())
	if len(doc2.Pages) != len(doc.Pages) {
		t.Fatalf("%d pages, want %d", len(doc2.Pages), len(doc.Pages))
	}
	done := map[pdf.Ref]bool{}
	n := 0
	for i := range doc.Pages {
		p, p2 := &doc.Pages[i], &doc2.Pages[i]
		if len(p.Uses) != len(p2.Uses) {
			t.Fatalf("page %d: %d images, want %d", i+1, len(p2.Uses), len(p.Uses))
		}
		for k, u := range p.Uses {
			im, im2 := u.Image, p2.Uses[k].Image
			if im2.Enc != "jbig2" || im.Enc == "jbig2" || done[im2.Ref] {
				continue
			}
			done[im2.Ref] = true
			v, err := raster.Decode(r, im)
			if err != nil {
				t.Fatal(err)
			}
			bm := v.(*raster.Bitmap)
			if im2.Width != im.Width || im2.Height != im.Height || im2.Stencil != im.Stencil {
				t.Errorf("object %d: changed from %dx%d stencil %v", im2.Ref, im.Width, im.Height, im.Stencil)
			}
			if got, want := decodeArray(r2, im2.Dict), bm.Decode; got != want {
				t.Errorf("object %d: /Decode %v, want %v", im2.Ref, got, want)
			}
			if got := jbig2dec(t, dir, r2, im2); !bytes.Equal(got, pbm(bm)) {
				t.Errorf("object %d (page %d): jbig2dec output differs", im2.Ref, i+1)
			}
			n++
		}
	}
	if n != st.Replaced {
		t.Errorf("%d images checked, %d replaced", n, st.Replaced)
	}
	t.Logf("%d of %d images replaced, %d to %d bytes; file %d to %d", st.Replaced, st.Images,
		st.Before, st.After, len(data), out.Len())
}

func open(t *testing.T, data []byte) (*pdfread.Reader, *scan.Document) {
	t.Helper()
	r, err := pdfread.Open(data)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := scan.Find(r)
	if err != nil {
		t.Fatal(err)
	}
	return r, doc
}

func decodeArray(r *pdfread.Reader, d pdf.Dict) [2]float64 {
	a, _ := r.Resolve(d["Decode"]).(pdf.Array)
	if len(a) != 2 {
		return [2]float64{0, 1}
	}
	var out [2]float64
	for i := range out {
		switch v := r.Resolve(a[i]).(type) {
		case int:
			out[i] = float64(v)
		case float64:
			out[i] = v
		}
	}
	return out
}

// jbig2dec decodes an image's JBIG2 stream, with its globals, to PBM.
func jbig2dec(t *testing.T, dir string, r *pdfread.Reader, im *scan.Image) []byte {
	t.Helper()
	s := r.Get(int(im.Ref)).(*pdfread.Stream)
	args := []string{"-q", "-e", "-t", "pbm"}
	out := filepath.Join(dir, fmt.Sprintf("%d.pbm", im.Ref))
	args = append(args, "-o", out)
	if p, ok := r.Resolve(s.Dict["DecodeParms"]).(pdf.Dict); ok {
		if g, ok := r.Resolve(p["JBIG2Globals"]).(*pdfread.Stream); ok {
			gd, err := r.Decode(g)
			if err != nil {
				t.Fatal(err)
			}
			name := filepath.Join(dir, fmt.Sprintf("%d.glob", im.Ref))
			os.WriteFile(name, gd, 0o644)
			args = append(args, name)
		}
	}
	name := filepath.Join(dir, fmt.Sprintf("%d.jb2", im.Ref))
	os.WriteFile(name, r.Raw(s), 0o644)
	if b, err := exec.Command("jbig2dec", append(args, name)...).CombinedOutput(); err != nil {
		t.Fatalf("jbig2dec: %v\n%s", err, b)
	}
	d, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	os.Remove(out)
	return d
}

func pbm(b *raster.Bitmap) []byte {
	out := fmt.Appendf(nil, "P4\n%d %d\n", b.W, b.H)
	for y := range b.H {
		out = append(out, b.Pix[y*b.Stride:y*b.Stride+(b.W+7)/8]...)
	}
	return out
}
