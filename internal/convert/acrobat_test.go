// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

//go:build unix

package convert

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

	"github.com/sergev/dejapdf/internal/djvu"
)

// Adobe Reader 9 (acroread), where installed, prints every corpus page
// with refinement dictionaries forced on, and the masks match ddjvu's
// pixel for pixel. Reader fails or hangs on a stream it cannot decode.
func TestAcrobat(t *testing.T) {
	need(t, "acroread", "gs", "ddjvu")
	if testing.Short() {
		t.Skip("slow")
	}
	defer func(n int) { minRefined = n }(minRefined)
	minRefined = 1
	files, _ := filepath.Glob(filepath.Join(corpus, "*.djvu"))
	dir := t.TempDir()
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".djvu")
		doc, err := djvu.Open(f)
		if err != nil {
			t.Fatal(err)
		}
		out := filepath.Join(dir, name+".pdf")
		g, err := os.Create(out)
		if err != nil {
			t.Fatal(err)
		}
		if err := Convert(doc, g, Options{Producer: "test", MaskOnly: true}); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		g.Close()
		ps := filepath.Join(dir, name+".ps")
		if err := acroread(dir, out); err != nil || !exists(ps) {
			t.Errorf("%s: Reader printed nothing: %v", name, err)
			continue
		}
		for i, pg := range doc.Pages {
			n := fmt.Sprint(i + 1)
			got, want := filepath.Join(dir, "got.pgm"), filepath.Join(dir, "want.pgm")
			run(t, "gs", "-q", "-dSAFER", "-dBATCH", "-dNOPAUSE", "-dUseFastColor", "-sDEVICE=pgmraw",
				"-dFirstPage="+n, "-dLastPage="+n, fmt.Sprintf("-r%d", pg.Info.DPI), "-sOutputFile="+got, ps)
			run(t, "ddjvu", "-mode=mask", "-format=pgm", "-page="+n, f, want)
			gw, gh, gp := readPNM(t, got)
			ww, wh, wp := readPNM(t, want)
			if gw != ww || gh != wh || !bytes.Equal(gp, wp) {
				t.Errorf("%s page %s: Reader's render differs from ddjvu mask", name, n)
			}
		}
	}
}

// acroread prints pdf to PostScript next to it, killing Reader (a script
// and its binary) after a minute.
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
	case <-time.After(time.Minute):
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-done
		return fmt.Errorf("timed out")
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
