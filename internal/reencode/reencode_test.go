// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package reencode

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sergev/dejapdf/internal/pdf"
	"github.com/sergev/dejapdf/internal/pdfread"
	"github.com/sergev/dejapdf/internal/raster"
	"github.com/sergev/dejapdf/internal/scan"
	"github.com/sergev/dejapdf/internal/shapes"
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
			check(t, path, false)
		})
	}
}

// TestLossy is TestJbig2dec with alike shapes substituted: each image
// decodes to its original within 2 pixels, both ways.
func TestLossy(t *testing.T) {
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
			check(t, path, true)
		})
	}
}

func check(t *testing.T, path string, lossy bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	r, doc := open(t, data)
	var out bytes.Buffer
	st, err := Reencode(r, doc, &out, Options{Lossy: lossy, Warn: func(err error) { t.Error(err) }})
	if _, ok := err.(*NothingError); ok {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	if st.Images == 0 {
		t.Skip("no scans")
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

	// Each replaced image, found by the object graph.
	r2, doc2 := open(t, out.Bytes())
	if len(doc2.Pages) != len(doc.Pages) {
		t.Fatalf("%d pages, want %d", len(doc2.Pages), len(doc.Pages))
	}
	objs, m := sameGraph(t, r, r2)
	done := map[pdf.Ref]bool{}
	n := 0
	for i := range doc.Pages {
		p := &doc.Pages[i]
		sep := false
		for _, u := range p.Uses {
			im := u.Image
			ref2, ok := m[im.Ref]
			if !ok || im.Ref == 0 {
				continue
			}
			s2, _ := r2.Get(int(ref2)).(*pdfread.Stream)
			if s2 == nil {
				continue
			}
			if s2.Dict["Subtype"] == pdf.Name("Form") {
				sep = true
				if !done[ref2] {
					done[ref2] = true
					checkForm(t, dir, r2, s2)
					n++
				}
				continue
			}
			if s2.Dict["Filter"] != pdf.Name("JBIG2Decode") || im.Enc == "jbig2" || done[ref2] {
				continue
			}
			done[ref2] = true
			v, err := raster.Decode(r, im)
			if err != nil {
				t.Fatal(err)
			}
			bm := v.(*raster.Bitmap)
			if s2.Dict["Width"] != im.Width || s2.Dict["Height"] != im.Height || (s2.Dict["ImageMask"] == true) != im.Stencil {
				t.Errorf("object %d: changed from %dx%d stencil %v", ref2, im.Width, im.Height, im.Stencil)
			}
			if got, want := decodeArray(r2, s2.Dict), bm.Decode; got != want {
				t.Errorf("object %d: /Decode %v, want %v", ref2, got, want)
			}
			got := jbig2dec(t, dir, r2, ref2)
			if lossy {
				if d := farthest(got, bm); d > 2 {
					t.Errorf("object %d (page %d): a pixel %d away from the original", ref2, i+1, d)
				}
			} else if !bytes.Equal(got, pbm(bm)) {
				t.Errorf("object %d (page %d): jbig2dec output differs", ref2, i+1)
			}
			n++
		}
		if sep {
			samePage(t, dir, path, name, i+1)
		}
	}
	if n != st.Replaced {
		t.Errorf("%d images checked, %d replaced", n, st.Replaced)
	}
	t.Logf("%d of %d images replaced, %d to %d bytes; file %d to %d; %d objects the same", st.Replaced, st.Images,
		st.Before, st.After, len(data), out.Len(), objs)
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
func jbig2dec(t *testing.T, dir string, r *pdfread.Reader, ref pdf.Ref) []byte {
	t.Helper()
	s := r.Get(int(ref)).(*pdfread.Stream)
	args := []string{"-q", "-e", "-t", "pbm"}
	out := filepath.Join(dir, fmt.Sprintf("%d.pbm", ref))
	args = append(args, "-o", out)
	if p, ok := r.Resolve(s.Dict["DecodeParms"]).(pdf.Dict); ok {
		if g, ok := r.Resolve(p["JBIG2Globals"]).(*pdfread.Stream); ok {
			gd, err := r.Decode(g)
			if err != nil {
				t.Fatal(err)
			}
			name := filepath.Join(dir, fmt.Sprintf("%d.glob", ref))
			os.WriteFile(name, gd, 0o644)
			args = append(args, name)
		}
	}
	name := filepath.Join(dir, fmt.Sprintf("%d.jb2", ref))
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

// sameGraph checks that the objects reachable from the trailers of r
// and r2 are the same but for numbering, and for the replaced images'
// encoding; streams have the same data. It returns the number of
// objects compared.
func sameGraph(t *testing.T, r, r2 *pdfread.Reader) (int, map[pdf.Ref]pdf.Ref) {
	t.Helper()
	m := map[pdf.Ref]pdf.Ref{}
	var queue [][2]pdf.Ref
	errs := 0
	fail := func(path, f string, args ...any) {
		if errs++; errs <= 10 {
			t.Errorf("%s: %s", path, fmt.Sprintf(f, args...))
		}
	}
	var same func(a, b any, path string, root bool)
	same = func(a, b any, path string, root bool) {
		if ra, ok := a.(pdf.Ref); ok {
			if r.Get(int(ra)) == nil {
				a = nil // a missing object reads as null
			} else {
				rb, ok := b.(pdf.Ref)
				if !ok {
					fail(path, "%v became %v", a, b)
					return
				}
				if old, ok := m[ra]; ok {
					if old != rb {
						fail(path, "%v is %v and %v", ra, old, rb)
					}
					return
				}
				m[ra] = rb
				queue = append(queue, [2]pdf.Ref{ra, rb})
				return
			}
		}
		switch av := a.(type) {
		case pdf.Dict:
			bv, ok := b.(pdf.Dict)
			if !ok {
				fail(path, "dictionary became %T", b)
				return
			}
			for k, v := range av {
				if v != nil {
					same(v, bv[k], path+"/"+string(k), false)
				}
			}
			for k, v := range bv {
				if _, ok := av[k]; !ok && v != nil && !(root && k == "Version") {
					fail(path, "new key %s", k)
				}
			}
		case pdf.Array:
			bv, ok := b.(pdf.Array)
			if !ok || len(bv) != len(av) {
				fail(path, "array became %v", b)
				return
			}
			for i := range av {
				same(av[i], bv[i], fmt.Sprintf("%s[%d]", path, i), false)
			}
		case *pdfread.Stream:
			fail(path, "stream as a direct object")
		default:
			if fmt.Sprint(a) != fmt.Sprint(b) {
				fail(path, "%v became %v", a, b)
			}
		}
	}
	tr, tr2 := r.Trailer(), r2.Trailer()
	same(tr["Root"], tr2["Root"], "Root", false)
	if _, ok := r.Resolve(tr["Info"]).(pdf.Dict); ok {
		same(tr["Info"], tr2["Info"], "Info", false)
	}
	root, _ := tr["Root"].(pdf.Ref)
	for len(queue) > 0 {
		q := queue[0]
		queue = queue[1:]
		path := fmt.Sprintf("%d→%d", q[0], q[1])
		a, b := r.Get(int(q[0])), r2.Get(int(q[1]))
		sa, ok := a.(*pdfread.Stream)
		if !ok {
			same(a, b, path, q[0] == root)
			continue
		}
		sb, ok := b.(*pdfread.Stream)
		if !ok {
			fail(path, "stream became %T", b)
			continue
		}
		da, db := pdf.Dict{}, pdf.Dict{}
		for k, v := range sa.Dict {
			da[k] = v
		}
		for k, v := range sb.Dict {
			db[k] = v
		}
		delete(da, "Length")
		delete(db, "Length")
		if db["Subtype"] == pdf.Name("Form") && da["Subtype"] == pdf.Name("Image") {
			// A gray or colour image, now layers in a form.
			if fmt.Sprint(da["OC"]) != fmt.Sprint(db["OC"]) && da["OC"] != nil {
				same(da["OC"], db["OC"], path+"/OC", false)
			}
			continue
		}
		if db["Filter"] == pdf.Name("JBIG2Decode") && da["Filter"] != pdf.Name("JBIG2Decode") {
			// A replaced image: the rest of its dictionary is kept.
			for _, k := range []string{"Filter", "DecodeParms", "Decode", "BitsPerComponent", "DL"} {
				delete(da, k)
				delete(db, k)
			}
			same(da, db, path, false)
			continue
		}
		dropCrypt(da)
		same(da, db, path, false)
		if !bytes.Equal(r.Raw(sa), r2.Raw(sb)) {
			fail(path, "stream data differs")
		}
	}
	return len(m), m
}

// dropCrypt removes an identity /Crypt filter, which the copy drops.
func dropCrypt(d pdf.Dict) {
	if f, ok := d["Filter"].(pdf.Name); ok && f == "Crypt" {
		delete(d, "Filter")
		delete(d, "DecodeParms")
	}
	if f, ok := d["Filter"].(pdf.Array); ok && len(f) > 0 && f[0] == pdf.Name("Crypt") {
		d["Filter"] = f[1:]
		if p, ok := d["DecodeParms"].(pdf.Array); ok && len(p) > 0 {
			d["DecodeParms"] = p[1:]
		}
	}
}

// TestWide codes near-empty pages as large as McKusick's spreads
// (7000 pixels wide) and checks jbig2dec's decode.
func TestWide(t *testing.T) {
	if _, err := exec.LookPath("jbig2dec"); err != nil {
		t.Skip("no jbig2dec")
	}
	const w, h = 7003, 5000
	set := func(b *raster.Bitmap, x, y int) { b.Pix[y*b.Stride+x/8] |= 0x80 >> (x % 8) }
	pages := map[string]func(b *raster.Bitmap){
		"blank":  func(b *raster.Bitmap) {},
		"corner": func(b *raster.Bitmap) { set(b, w-1, h-1) },
		"origin": func(b *raster.Bitmap) { set(b, 0, 0) },
		"rule": func(b *raster.Bitmap) {
			for x := range w {
				set(b, x, h/2)
			}
		},
		"edges": func(b *raster.Bitmap) {
			for x := 0; x < w; x += 97 {
				set(b, x, 0)
				set(b, x, h-1)
			}
			for y := 0; y < h; y += 89 {
				set(b, 0, y)
				set(b, w-1, y)
			}
		},
	}
	dir := t.TempDir()
	for name, draw := range pages {
		b := &raster.Bitmap{W: w, H: h, Stride: (w + 7) / 8}
		b.Pix = make([]byte, b.Stride*h)
		draw(b)
		data := encode(b)
		in, out := filepath.Join(dir, name+".jb2"), filepath.Join(dir, name+".pbm")
		os.WriteFile(in, data, 0o644)
		if msg, err := exec.Command("jbig2dec", "-q", "-e", "-t", "pbm", "-o", out, in).CombinedOutput(); err != nil {
			t.Fatalf("%s: jbig2dec: %v\n%s", name, err, msg)
		}
		got, _ := os.ReadFile(out)
		if !bytes.Equal(got, pbm(b)) {
			t.Errorf("%s: jbig2dec output differs", name)
		}
		t.Logf("%s: %d bytes", name, len(data))
	}
}

// encode codes a bitmap alone, as Reencode does.
func encode(b *raster.Bitmap) []byte {
	data, _ := code(shapes.Split(b), nil)
	return data
}

// farthest is how far (in the 8-neighbour metric, up to 3) a pixel of
// either image is from the nearest of the other: got, a PBM, and b.
func farthest(got []byte, b *raster.Bitmap) int {
	want := pbm(b)
	if len(got) != len(want) {
		return 99
	}
	hdr := len(want) - b.H*((b.W+7)/8)
	stride := (b.W + 7) / 8
	at := func(p []byte, x, y int) bool {
		return x >= 0 && y >= 0 && x < b.W && y < b.H && p[hdr+y*stride+x/8]>>(7-x%8)&1 != 0
	}
	// dist is whether every ink pixel of p lies within d of one of q.
	within := func(p, q []byte, d int) bool {
		for y := range b.H {
			for x := range b.W {
				if !at(p, x, y) || at(q, x, y) {
					continue
				}
				near := false
				for dy := -d; dy <= d && !near; dy++ {
					for dx := -d; dx <= d && !near; dx++ {
						near = at(q, x+dx, y+dy)
					}
				}
				if !near {
					return false
				}
			}
		}
		return true
	}
	for d := 0; d < 3; d++ {
		if within(got, want, d) && within(want, got, d) {
			return d
		}
	}
	return 3
}

// checkForm checks a gray or colour image's replacement: a unit form
// whose JPX background opj_decompress decodes and whose stencils
// jbig2dec decodes.
func checkForm(t *testing.T, dir string, r *pdfread.Reader, s *pdfread.Stream) {
	t.Helper()
	if fmt.Sprint(r.Resolve(s.Dict["BBox"])) != "[0 0 1 1]" {
		t.Errorf("form BBox %v", s.Dict["BBox"])
	}
	res, _ := r.Resolve(s.Dict["Resources"]).(pdf.Dict)
	xobj, _ := r.Resolve(res["XObject"]).(pdf.Dict)
	for name, v := range xobj {
		ref, _ := v.(pdf.Ref)
		x, _ := r.Get(int(ref)).(*pdfread.Stream)
		if x == nil {
			t.Errorf("form XObject %s: %v", name, v)
			continue
		}
		switch x.Dict["Filter"] {
		case pdf.Name("JBIG2Decode"):
			if x.Dict["ImageMask"] != true {
				t.Errorf("%s: not a stencil", name)
			}
			jbig2dec(t, dir, r, ref)
		case pdf.Name("JPXDecode"):
			if _, err := exec.LookPath("opj_decompress"); err != nil {
				continue
			}
			in, out := filepath.Join(dir, "bg.jp2"), filepath.Join(dir, "bg.pgm")
			if x.Dict["ColorSpace"] == pdf.Name("DeviceRGB") {
				out = filepath.Join(dir, "bg.ppm")
			}
			os.WriteFile(in, r.Raw(x), 0o644)
			if b, err := exec.Command("opj_decompress", "-i", in, "-o", out).CombinedOutput(); err != nil {
				t.Errorf("%s: opj_decompress: %v\n%s", name, err, b)
			}
		default:
			t.Errorf("%s: filter %v", name, x.Dict["Filter"])
		}
	}
}

// samePage renders page n of the original and the re-encoded file
// with mutool, gray at 50 dpi, and checks they look alike blurred over
// 3×3 pixels: layers in place, nothing missing, the colours right;
// the weight of the strokes may differ.
func samePage(t *testing.T, dir, orig, got string, n int) {
	t.Helper()
	if _, err := exec.LookPath("mutool"); err != nil {
		return
	}
	var pix [2][]byte
	var w, h int
	for k, f := range []string{orig, got} {
		out := filepath.Join(dir, fmt.Sprintf("p%d.pgm", k))
		if b, err := exec.Command("mutool", "draw", "-q", "-r", "50", "-c", "gray", "-o", out, f,
			fmt.Sprint(n)).CombinedOutput(); err != nil {
			t.Fatalf("mutool: %v\n%s", err, b)
		}
		data, _ := os.ReadFile(out)
		var g []byte
		if w, h, g = readPGM(data); g == nil {
			t.Fatalf("bad render of page %d", n)
		}
		pix[k] = blur(g, w, h)
	}
	if len(pix[0]) != len(pix[1]) {
		t.Errorf("page %d: rendered %d and %d bytes", n, len(pix[0]), len(pix[1]))
		return
	}
	psnr := blurPSNR(pix[0], pix[1])
	t.Logf("page %d: %.1f dB", n, psnr)
	if psnr < minPSNR {
		t.Errorf("page %d: %.1f dB from the original", n, psnr)
	}
}

// blurPSNR compares two blurred renders.
func blurPSNR(a, b []byte) float64 {
	sq := 0.0
	for i := range a {
		d := float64(a[i]) - float64(b[i])
		sq += d * d
	}
	return 10 * math.Log10(255*255/max(sq/float64(len(a)), 1e-9))
}

// blur is the 3×3 mean.
func blur(g []byte, w, h int) []byte {
	out := make([]byte, len(g))
	for y := range h {
		for x := range w {
			s, n := 0, 0
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if X, Y := x+dx, y+dy; X >= 0 && Y >= 0 && X < w && Y < h {
						s += int(g[Y*w+X])
						n++
					}
				}
			}
			out[y*w+x] = byte(s / n)
		}
	}
	return out
}

// minPSNR is how close, blurred, a page with separated layers renders
// to the original at 50 dpi.
const minPSNR = 26

// readPGM parses a binary PGM, comments allowed.
func readPGM(data []byte) (w, h int, pix []byte) {
	var f [4]int
	i := 2
	for k := 0; k < 3 && i < len(data); {
		switch c := data[i]; {
		case c == '#':
			for i < len(data) && data[i] != '\n' {
				i++
			}
		case c >= '0' && c <= '9':
			for i < len(data) && data[i] >= '0' && data[i] <= '9' {
				f[k] = 10*f[k] + int(data[i]-'0')
				i++
			}
			k++
			continue
		}
		i++
	}
	i++ // the one blank after the maximum
	w, h = f[0], f[1]
	if len(data) < 2 || string(data[:2]) != "P5" || i+w*h > len(data) {
		return 0, 0, nil
	}
	return w, h, data[i : i+w*h]
}

// TestNothing: a document with nothing to re-encode says why, and
// writes nothing; -p keeps the pages asked for.
func TestNothing(t *testing.T) {
	for _, c := range []struct {
		file, reason string
	}{{"inline.pdf", "inline image"}, {"g3-2d.pdf", "CCITT G3 2-D"}} {
		data, err := os.ReadFile(filepath.Join("../../testdata/pdf", c.file))
		if err != nil {
			t.Fatal(err)
		}
		r, doc := open(t, data)
		var out bytes.Buffer
		_, err = Reencode(r, doc, &out, Options{})
		if e, ok := err.(*NothingError); !ok || !strings.Contains(e.Error(), c.reason) || out.Len() != 0 {
			t.Errorf("%s: %v, %d bytes written", c.file, err, out.Len())
		}
	}
	data, _ := os.ReadFile("../../testdata/pdf/digits.pdf")
	r, doc := open(t, data)
	var out bytes.Buffer
	st, err := Reencode(r, doc, &out, Options{Pages: []int{5, 2}})
	if err != nil {
		t.Fatal(err)
	}
	_, doc2 := open(t, out.Bytes())
	if len(doc2.Pages) != 2 || st.Images != 2 {
		t.Errorf("%d pages, %d images", len(doc2.Pages), st.Images)
	}
}
