// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

//go:build unix

package reencode

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/sergev/dejapdf/internal/stencil"
)

// Adobe Reader 9 (acroread), where installed, prints each re-encoded
// test file as it prints the original, page for page, and the books of
// $ACRO_BOOKS; refinement dictionaries forced on. Reader fails or hangs
// on a stream it cannot decode.
func TestAcrobat(t *testing.T) {
	for _, tool := range []string{"acroread", "gs"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip("no " + tool)
		}
	}
	if testing.Short() {
		t.Skip("slow")
	}
	defer func(n int) { stencil.MinRefined = n }(stencil.MinRefined)
	stencil.MinRefined = 1
	files, _ := filepath.Glob("../../testdata/pdf/*.pdf")
	books, _ := filepath.Glob(os.Getenv("ACRO_BOOKS"))
	dir := t.TempDir()
	for _, path := range append(files, books...) {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		r, doc := open(t, data)
		var out bytes.Buffer
		st, err := Reencode(r, doc, &out, Options{})
		if err != nil {
			t.Fatal(err)
		}
		if st.Replaced == 0 {
			continue
		}
		name := strings.TrimSuffix(filepath.Base(path), ".pdf")
		orig, got := filepath.Join(dir, "a.pdf"), filepath.Join(dir, "b.pdf")
		os.WriteFile(orig, data, 0o644)
		os.WriteFile(got, out.Bytes(), 0o644)
		var pages [2][]string
		for k, f := range []string{orig, got} {
			ps := strings.TrimSuffix(f, ".pdf") + ".ps"
			os.Remove(ps)
			if err := acroread(dir, f); err != nil || !exists(ps) {
				if k == 0 {
					t.Logf("%s: Reader cannot print the original", name)
					break
				}
				t.Fatalf("%s: Reader printed nothing: %v", name, err)
			}
			pgm := filepath.Join(dir, fmt.Sprintf("%c-%%d.pgm", 'a'+k))
			if b, err := exec.Command("gs", "-q", "-dSAFER", "-dBATCH", "-dNOPAUSE", "-sDEVICE=pgmraw", "-r100",
				"-sOutputFile="+pgm, ps).CombinedOutput(); err != nil {
				t.Fatalf("gs: %v\n%s", err, b)
			}
			pages[k], _ = filepath.Glob(strings.Replace(pgm, "%d", "*", 1))
		}
		if pages[0] == nil {
			continue
		}
		if len(pages[0]) != len(doc.Pages) || len(pages[1]) != len(pages[0]) {
			t.Errorf("%s: %d and %d pages printed, want %d", name, len(pages[0]), len(pages[1]), len(doc.Pages))
			continue
		}
		for i := range len(doc.Pages) {
			a, _ := os.ReadFile(filepath.Join(dir, fmt.Sprintf("a-%d.pgm", i+1)))
			b, _ := os.ReadFile(filepath.Join(dir, fmt.Sprintf("b-%d.pgm", i+1)))
			if !bytes.Equal(a, b) {
				t.Errorf("%s page %d: Reader prints it differently", name, i+1)
			}
		}
		for _, k := range pages {
			for _, f := range k {
				os.Remove(f)
			}
		}
		t.Logf("%s: %d pages", name, len(doc.Pages))
	}
}

// acroread prints pdf to PostScript next to it, killing Reader (a script
// and its binary) after half an hour.
func acroread(dir, pdf string) error {
	cmd := exec.Command("acroread", "-toPostScript", pdf)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(30 * time.Minute):
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-done
		return fmt.Errorf("timed out")
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
