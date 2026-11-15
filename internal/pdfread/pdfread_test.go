// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package pdfread

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

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

// TestStreamsQpdf compares every stream of the test files, and of
// $PDF_BOOKS, with qpdf's: decoded where qpdf decodes it (the generic
// filters), as far as we decode it where qpdf leaves image filters,
// and raw (decrypted) otherwise.
func TestStreamsQpdf(t *testing.T) {
	if _, err := exec.LookPath("qpdf"); err != nil {
		t.Skip("no qpdf")
	}
	files, _ := filepath.Glob("../../testdata/pdf/*.pdf")
	books, _ := filepath.Glob(os.Getenv("PDF_BOOKS"))
	for _, path := range append(files, books...) {
		compareQpdf(t, path)
	}
}

func compareQpdf(t *testing.T, path string) {
	dir, err := os.MkdirTemp("", "qpdf")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	out, err := exec.Command("qpdf", "--json=2", "--json-key=qpdf", "--json-stream-data=file",
		"--json-stream-prefix="+filepath.Join(dir, "s"), "--decode-level=generalized", path).Output()
	if len(out) == 0 {
		t.Logf("%s: qpdf fails: %v", path, err)
		return
	}
	var j struct {
		Qpdf []json.RawMessage
	}
	var objs map[string]struct {
		Stream *struct {
			Datafile string
			Dict     map[string]any
		}
	}
	if err := json.Unmarshal(out, &j); err != nil || len(j.Qpdf) < 2 {
		t.Fatalf("%s: qpdf JSON: %v", path, err)
	}
	if err := json.Unmarshal(j.Qpdf[1], &objs); err != nil {
		t.Fatalf("%s: qpdf JSON: %v", path, err)
	}
	r := openFile(t, path)
	stats := map[string]int{}
	for key, o := range objs {
		if o.Stream == nil {
			continue
		}
		var num, gen int
		fmt.Sscanf(key, "obj:%d %d R", &num, &gen)
		s, ok := r.Get(num).(*Stream)
		if !ok || r.xref[num].kind == 1 && r.xref[num].gen != gen {
			t.Errorf("%s: object %d %d: not a stream", path, num, gen)
			continue
		}
		want, err := os.ReadFile(o.Stream.Datafile)
		if err != nil {
			t.Fatal(err)
		}
		// The filters qpdf left, which must end ours.
		var left []string
		switch f := o.Stream.Dict["/Filter"].(type) {
		case string:
			left = []string{f}
		case []any:
			for _, x := range f {
				left = append(left, fmt.Sprint(x))
			}
		}
		var all []string
		switch f := r.Resolve(s.Dict["Filter"]).(type) {
		case pdf.Name:
			all = []string{"/" + string(f)}
		case pdf.Array:
			for _, x := range f {
				n, _ := r.Resolve(x).(pdf.Name)
				all = append(all, "/"+string(n))
			}
		}
		got, err := r.Decode(s)
		var kind string
		switch {
		case len(left) == 0:
			kind = "decoded"
		case len(left) == len(all):
			kind = "raw"
			got, err = r.Raw(s), nil
		default:
			kind = "partly decoded"
			var u *UnsupportedFilterError
			if !errors.As(err, &u) || u.Index != len(all)-len(left) {
				t.Errorf("%s: object %d: %v; qpdf leaves %v of %v", path, num, err, left, all)
				continue
			}
			err = nil
		}
		stats[kind]++
		if err != nil || !bytes.Equal(got, want) {
			t.Errorf("%s: object %d (%v, %s): %d bytes, %v; want %d", path, num, all, kind, len(got), err, len(want))
		}
	}
	t.Logf("%s: %v", filepath.Base(path), stats)
}
