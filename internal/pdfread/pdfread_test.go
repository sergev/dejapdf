// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package pdfread

import (
	"bytes"

	"bufio"
	"github.com/sergev/dejapdf/internal/pdf"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func openFile(t *testing.T, path string) *Reader {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	r, err := Open(data)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return r
}

// TestTestdata checks page counts against testdata/pdf/npages.txt.
func TestTestdata(t *testing.T) {
	f, err := os.Open("../../testdata/pdf/npages.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		n, name, _ := strings.Cut(sc.Text(), " ")
		want, _ := strconv.Atoi(n)
		r := openFile(t, "../../testdata/pdf/"+name)
		pages, err := r.Pages()
		if err != nil || len(pages) != want {
			t.Errorf("%s: %d pages, %v; want %d", name, len(pages), err, want)
		}
		if r.Repaired {
			t.Errorf("%s: repaired", name)
		}
		checkObjects(t, name, r)
	}
}

// checkObjects loads every object and decodes every stream we can.
func checkObjects(t *testing.T, name string, r *Reader) {
	for _, n := range r.Objects() {
		if s, ok := r.Get(n).(*Stream); ok {
			if _, err := r.Decode(s); err != nil && !strings.Contains(err.Error(), "unsupported") {
				t.Errorf("%s: object %d: %v", name, n, err)
			}
		}
	}
}

// TestBooks checks the files in $PDF_BOOKS (a glob) against qpdf.
func TestBooks(t *testing.T) {
	files, _ := filepath.Glob(os.Getenv("PDF_BOOKS"))
	if len(files) == 0 {
		t.Skip("PDF_BOOKS not set")
	}
	if _, err := exec.LookPath("qpdf"); err != nil {
		t.Skip("no qpdf")
	}
	for _, path := range files {
		out, err := exec.Command("qpdf", "--show-npages", path).Output()
		want, err2 := strconv.Atoi(strings.TrimSpace(string(out)))
		if err != nil && len(out) == 0 || err2 != nil {
			t.Logf("%s: qpdf fails, skipped", path)
			continue
		}
		r := openFile(t, path)
		pages, err := r.Pages()
		if err != nil || len(pages) != want {
			t.Errorf("%s: %d pages, %v; want %d", path, len(pages), err, want)
		}
		if r.Repaired {
			t.Logf("%s: repaired", path)
		}
	}
}

// TestStreamsQpdf compares the Flate streams of the test files, and of
// $PDF_BOOKS, with qpdf's decoding.
func TestStreamsQpdf(t *testing.T) {
	if _, err := exec.LookPath("qpdf"); err != nil {
		t.Skip("no qpdf")
	}
	files, _ := filepath.Glob("../../testdata/pdf/*.pdf")
	books, _ := filepath.Glob(os.Getenv("PDF_BOOKS"))
	files = append(files, books...)
	for _, path := range files {
		r := openFile(t, path)
		checked := 0
		for _, n := range r.Objects() {
			s, ok := r.Get(n).(*Stream)
			if !ok || checked >= 20 {
				continue
			}
			f := r.Resolve(s.Dict["Filter"])
			if f != nil && f != pdf.Name("FlateDecode") || s.Dict["Type"] == pdf.Name("XRef") {
				continue
			}
			obj := strconv.Itoa(n)
			if e := r.xref[n]; e.kind == 1 && e.gen != 0 {
				obj += "," + strconv.Itoa(e.gen)
			}
			want, err := exec.Command("qpdf", "--show-object="+obj,
				"--filtered-stream-data", path).Output()
			if err != nil {
				continue
			}
			got, err := r.Decode(s)
			if err != nil && strings.Contains(err.Error(), "unsupported") {
				continue
			}
			if err != nil || !bytes.Equal(got, want) {
				t.Errorf("%s: object %d: %d bytes, %v; want %d", path, n, len(got), err, len(want))
			}
			checked++
		}
		t.Logf("%s: %d streams", filepath.Base(path), checked)
	}
}
