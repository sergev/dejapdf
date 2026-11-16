// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package convert

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sergev/dejapdf/internal/djvu"
)

const corpus = "../../testdata/corpus"

// Converted pages, rendered by mutool, match ddjvu -mode=mask
// (masks.sha256), and qpdf accepts the files.
func TestCorpusRender(t *testing.T) {
	for _, tool := range []string{"mutool", "qpdf"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip("no " + tool)
		}
	}
	f, err := os.Open(filepath.Join(corpus, "masks.sha256"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	dir := t.TempDir()
	pdfs := map[string]*djvu.Document{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		w := strings.Fields(sc.Text())
		out := filepath.Join(dir, strings.TrimSuffix(w[1], ".djvu")+".pdf")
		doc := pdfs[w[1]]
		if doc == nil {
			if doc, err = djvu.Open(filepath.Join(corpus, w[1])); err != nil {
				t.Fatal(err)
			}
			pdfs[w[1]] = doc
			g, err := os.Create(out)
			if err != nil {
				t.Fatal(err)
			}
			if err := Convert(doc, g, Options{Producer: "test", MaskOnly: true}); err != nil {
				t.Fatalf("%s: %v", w[1], err)
			}
			g.Close()
			if b, err := exec.Command("qpdf", "--check", out).CombinedOutput(); err != nil {
				t.Errorf("%s: qpdf: %v\n%s", w[1], err, b)
			}
		}
		var page int
		fmt.Sscan(w[2], &page)
		dpi := doc.Pages[page-1].Info.DPI
		pbm := filepath.Join(dir, "page.pbm")
		if b, err := exec.Command("mutool", "draw", "-q", "-r", fmt.Sprint(dpi), "-c", "mono",
			"-o", pbm, out, w[2]).CombinedOutput(); err != nil {
			t.Fatalf("mutool: %v\n%s", err, b)
		}
		b, err := os.ReadFile(pbm)
		if err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(b)); got != w[0] {
			t.Errorf("%s page %d: render differs from ddjvu mask", w[1], page)
		}
	}
}

// readPNM returns the pixels of a P5 or P6 file, skipping comments.
func readPNM(t *testing.T, path string) (w, h int, pix []byte) {
	t.Helper()
	d, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var f []string
	i := 0
	for len(f) < 4 && i < len(d) {
		for i < len(d) && (d[i] == ' ' || d[i] == '\n' || d[i] == '\r' || d[i] == '\t') {
			i++
		}
		if i < len(d) && d[i] == '#' {
			for i < len(d) && d[i] != '\n' {
				i++
			}
			continue
		}
		j := i
		for j < len(d) && d[j] > ' ' {
			j++
		}
		f = append(f, string(d[i:j]))
		i = j
	}
	fmt.Sscan(f[1], &w)
	fmt.Sscan(f[2], &h)
	return w, h, d[i+1:]
}

func psnr(a, b []byte) float64 {
	se := 0.0
	for i := range a {
		e := float64(a[i]) - float64(b[i])
		se += e * e
	}
	if se == 0 {
		return math.Inf(1)
	}
	return 10 * math.Log10(255*255*float64(len(a))/se)
}

// Converted pages, rendered by mutool, are close to ddjvu's renders. Most
// of the difference is on the foreground: viewers interpolate an
// upscaled FG layer, ddjvu does not.
func TestCorpusColour(t *testing.T) {
	t.Parallel()
	for _, tool := range []string{"mutool", "qpdf", "ddjvu"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip("no " + tool)
		}
	}
	_, err := exec.LookPath("pdftoppm")
	poppler := err == nil
	files, _ := filepath.Glob(filepath.Join(corpus, "*.djvu"))
	for _, f := range files {
		name := filepath.Base(f)
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			colour(t, f, poppler)
		})
	}
}

// colour renders each page of a converted corpus file in MuPDF (and
// Poppler) and compares it with ddjvu.
func colour(t *testing.T, f string, poppler bool) {
	name := filepath.Base(f)
	dir := t.TempDir()
	doc, err := djvu.Open(f)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out.pdf")
	g, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	if err := Convert(doc, g, Options{}); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	g.Close()
	if b, err := exec.Command("qpdf", "--check", out).CombinedOutput(); err != nil {
		t.Errorf("%s: qpdf: %v\n%s", name, err, b)
	}
	for p := range doc.Pages {
		ref, got := filepath.Join(dir, "d.ppm"), filepath.Join(dir, "m.ppm")
		pg := fmt.Sprint(p + 1)
		if b, err := exec.Command("ddjvu", "-format=ppm", "-page="+pg, f, ref).CombinedOutput(); err != nil {
			t.Fatalf("ddjvu: %v\n%s", err, b)
		}
		w, h, a := readPNM(t, ref)
		if b, err := exec.Command("mutool", "draw", "-q", "-w", fmt.Sprint(w), "-h", fmt.Sprint(h),
			"-c", "rgb", "-o", got, out, pg).CombinedOutput(); err != nil {
			t.Fatalf("mutool: %v\n%s", err, b)
		}
		w2, h2, b := readPNM(t, got)
		if w2 != w || h2 != h || len(a) != len(b) {
			t.Errorf("%s page %s: %dx%d, want %dx%d", name, pg, w2, h2, w, h)
			continue
		}
		if v := psnr(a, b); v < 28 {
			t.Errorf("%s page %s: MuPDF %.1f dB from ddjvu", name, pg, v)
		} else {
			t.Logf("%s page %s: MuPDF %.1f dB", name, pg, v)
		}
		if !poppler {
			continue
		}
		pp := filepath.Join(dir, "p")
		if b, err := exec.Command("pdftoppm", "-f", pg, "-l", pg, "-singlefile",
			"-scale-to-x", fmt.Sprint(w), "-scale-to-y", fmt.Sprint(h), out, pp).CombinedOutput(); err != nil {
			t.Fatalf("pdftoppm: %v\n%s", err, b)
		}
		if _, _, c := readPNM(t, pp+".ppm"); len(c) == len(a) {
			if v := psnr(a, c); v < 28 {
				t.Errorf("%s page %s: Poppler %.1f dB from ddjvu", name, pg, v)
			} else {
				t.Logf("%s page %s: Poppler %.1f dB", name, pg, v)
			}
		}
	}
}
