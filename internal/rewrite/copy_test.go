// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package rewrite

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/sergev/dejapdf/internal/pdf"
	"github.com/sergev/dejapdf/internal/pdfread"
)

// TestCopy copies the test files, and $PDF_BOOKS, and checks the
// copies: qpdf --check, the page count, the size, and every page
// rendered by mutool equal to the original. $COPY_DPI sets the
// resolution (default 50).
func TestCopy(t *testing.T) {
	for _, tool := range []string{"qpdf", "mutool"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip("no " + tool)
		}
	}
	files, _ := filepath.Glob("../../testdata/pdf/*.pdf")
	books, _ := filepath.Glob(os.Getenv("PDF_BOOKS"))
	dpi := os.Getenv("COPY_DPI")
	if dpi == "" {
		dpi = "50"
	}
	for _, path := range append(files, books...) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()
			checkCopy(t, path, dpi)
		})
	}
}

func checkCopy(t *testing.T, path, dpi string) {
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	r, err := pdfread.Open(data)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	out := filepath.Join(dir, "copy.pdf")
	var b bytes.Buffer
	if err := Copy(r, &b); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	if b.Len() > len(data)+len(data)/100 {
		t.Errorf("copy is %d bytes, original %d (%+.1f%%)", b.Len(), len(data),
			100*float64(b.Len()-len(data))/float64(len(data)))
	}
	// qpdf --check exits 0 (clean), 3 (warnings) or 2 (errors); the copy
	// may keep the original's warnings but must not be worse.
	if want, got, msg := check(path), check(out), ""; got != 0 && (want == 0 || got == 2 && want != 2) {
		msg2, _ := exec.Command("qpdf", "--check", out).CombinedOutput()
		msg = lastLines(msg2, 5)
		t.Errorf("qpdf --check: exit %d, original %d\n%s", got, want, msg)
	}
	n1 := npages(t, path)
	n2 := npages(t, out)
	if n1 != n2 {
		t.Fatalf("%d pages, want %d", n2, n1)
	}
	// Render both; pages the original fails to render are skipped.
	render := func(f, name string, pages ...string) string {
		d := filepath.Join(dir, name)
		os.Mkdir(d, 0o755)
		args := append([]string{"draw", "-q", "-r", dpi, "-o", filepath.Join(d, "%d.ppm"), f}, pages...)
		exec.Command("mutool", args...).Run()
		return d
	}
	orig, cp := render(path, "orig"), render(out, "copy")
	same := func(d1, d2 string, p int) (ok, rendered bool) {
		a, err := os.ReadFile(filepath.Join(d1, strconv.Itoa(p)+".ppm"))
		if err != nil {
			return false, false
		}
		c, err := os.ReadFile(filepath.Join(d2, strconv.Itoa(p)+".ppm"))
		return err == nil && bytes.Equal(a, c), true
	}
	var differ []string
	compared := 0
	for p := 1; p <= n1; p++ {
		ok, rendered := same(orig, cp, p)
		if rendered {
			compared++
		}
		if rendered && !ok {
			differ = append(differ, strconv.Itoa(p))
		}
	}
	// A damaged original can render differently from any rewrite of it
	// (MuPDF recovers from some syntax errors with side effects): such
	// pages must then match qpdf's rewrite.
	if len(differ) > 0 {
		q := filepath.Join(dir, "qpdf.pdf")
		exec.Command("qpdf", "--stream-data=preserve", path, q).Run()
		qd := render(q, "qpdf", strings.Join(differ, ","))
		bad := 0
		for _, ps := range differ {
			p, _ := strconv.Atoi(ps)
			if ok, _ := same(qd, cp, p); !ok {
				if bad < 5 {
					t.Errorf("page %d renders differently", p)
				}
				bad++
			}
		}
		if bad > 0 {
			t.Errorf("%d of %d pages differ", bad, n1)
		}
		t.Logf("%d pages differ from the original but match qpdf's rewrite", len(differ)-bad)
	}
	t.Logf("%d -> %d bytes (%+.1f%%), %d of %d pages compared", len(data), b.Len(),
		100*float64(b.Len()-len(data))/float64(len(data)), compared, n1)
}

// check returns the exit status of qpdf --check.
func check(path string) int {
	err := exec.Command("qpdf", "--check", path).Run()
	if e, ok := err.(*exec.ExitError); ok {
		return e.ExitCode()
	}
	return 0
}

func npages(t *testing.T, path string) int {
	out, _ := exec.Command("qpdf", "--show-npages", path).Output()
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		t.Fatalf("%s: qpdf --show-npages: %v", path, err)
	}
	return n
}

func lastLines(b []byte, n int) string {
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	return strings.Join(lines[max(0, len(lines)-n):], "\n")
}

// TestCopyEdges: a direct page tree, a reference to a missing object,
// a real that must not be rounded, and a header version above 1.5.
func TestCopyEdges(t *testing.T) {
	src := "%PDF-1.7\n" +
		"1 0 obj <</Type/Catalog/Pages<</Type/Pages/Kids[2 0 R]/Count 1>>/Missing 9 0 R>> endobj\n" +
		"2 0 obj <</Type/Page/MediaBox[0 0 612 792]/UserUnit 0.00048828125/Contents 3 0 R/X[9 0 R]>> endobj\n" +
		"3 0 obj <</Length 3>>stream\nq Q\nendstream endobj\n" +
		"trailer <</Root 1 0 R>>\n"
	r, err := pdfread.Open([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err := Copy(r, &b); err != nil {
		t.Fatal(err)
	}
	c, err := pdfread.Open(b.Bytes())
	if err != nil || c.Repaired {
		t.Fatalf("copy: %v, repaired %v", err, c.Repaired)
	}
	cat := c.Catalog()
	if cat["Version"] != pdf.Name("1.7") || cat["Missing"] != nil {
		t.Errorf("catalog %v", cat)
	}
	pages, err := c.Pages()
	if err != nil || len(pages) != 1 {
		t.Fatalf("%d pages, %v", len(pages), err)
	}
	p := pages[0].Dict
	if p["UserUnit"] != 0.00048828125 || len(p["X"].(pdf.Array)) != 1 || p["X"].(pdf.Array)[0] != nil {
		t.Errorf("page %v", p)
	}
	s, ok := c.Resolve(p["Contents"]).(*pdfread.Stream)
	if !ok || string(s.Raw) != "q Q" {
		t.Errorf("contents %v", s)
	}
}

// The output is the same every time.
func TestDeterministic(t *testing.T) {
	files, _ := filepath.Glob("../../testdata/pdf/*.pdf")
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var first []byte
		for range 5 {
			r, err := pdfread.Open(data)
			if err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			if err := Copy(r, &out); err != nil {
				t.Fatal(err)
			}
			if first == nil {
				first = out.Bytes()
			} else if !bytes.Equal(out.Bytes(), first) {
				t.Errorf("%s: output differs between runs", filepath.Base(path))
				break
			}
		}
	}
}
