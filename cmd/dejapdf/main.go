// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

// Command dejapdf converts DjVu documents to layered PDF, and
// re-encodes the scan images of PDF documents.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime/pprof"
	"runtime/trace"
	"strconv"
	"strings"
	"time"

	"github.com/sergev/dejapdf/internal/convert"
	"github.com/sergev/dejapdf/internal/djvu"
	"github.com/sergev/dejapdf/internal/iff"
	"github.com/sergev/dejapdf/internal/pdfread"
	"github.com/sergev/dejapdf/internal/reencode"
	"github.com/sergev/dejapdf/internal/rewrite"
	"github.com/sergev/dejapdf/internal/scan"
)

const version = "0.0.1"

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: dejapdf [options] input.djvu|input.pdf output.pdf\n")
	fmt.Fprintf(os.Stderr, "       dejapdf -dump input.djvu|input.pdf\n")
	flag.PrintDefaults()
}

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	dump := flag.Bool("dump", false, "print the chunk tree, like djvudump; for a PDF, the scan images by page")
	quality := flag.Float64("q", 0, fmt.Sprintf("colour layer size: a multiple of the DjVu size (default %g),\nor for a PDF a fraction of the scan image (default %g)", convert.DefaultQuality, reencode.DefaultQuality))
	pages := flag.String("p", "", "pages to convert, like 1-10,15 (default all)")
	verbose := flag.Bool("v", false, "show a size report by layer")
	lossless := flag.Bool("lossless", false, "PDF input: keep the shapes of the text exact (default: alike shapes\nare drawn alike, guarded against look-alike characters)")
	quiet := flag.Bool("quiet", false, "print nothing but warnings and errors")
	cpuProfile := flag.String("cpuprofile", "", "write a CPU profile to `file`")
	traceFile := flag.String("trace", "", "write an execution trace to `file`")
	flag.Usage = usage
	flag.Parse()

	if *showVersion {
		fmt.Println("dejapdf", version)
		return
	}
	if *dump {
		if flag.NArg() != 1 {
			usage()
			os.Exit(2)
		}
		if err := dumpFile(flag.Arg(0)); err != nil {
			fmt.Fprintln(os.Stderr, "dejapdf:", err)
			os.Exit(1)
		}
		return
	}
	if flag.NArg() != 2 {
		usage()
		os.Exit(2)
	}
	stop, err := startProfiles(*cpuProfile, *traceFile)
	if err == nil {
		err = convertFile(flag.Arg(0), flag.Arg(1), *quality, *pages, *verbose, *quiet, !*lossless)
		stop()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "dejapdf:", err)
		os.Exit(1)
	}
}

// startProfiles starts the CPU profile and execution trace asked for;
// stop ends them.
func startProfiles(cpu, tr string) (stop func(), err error) {
	var files []*os.File
	stop = func() {
		if cpu != "" {
			pprof.StopCPUProfile()
		}
		if tr != "" {
			trace.Stop()
		}
		for _, f := range files {
			f.Close()
		}
	}
	open := func(name string, start func(*os.File) error) error {
		f, err := os.Create(name)
		if err != nil {
			return err
		}
		files = append(files, f)
		return start(f)
	}
	if cpu != "" {
		if err = open(cpu, func(f *os.File) error { return pprof.StartCPUProfile(f) }); err != nil {
			cpu = ""
		}
	}
	if err == nil && tr != "" {
		if err = open(tr, func(f *os.File) error { return trace.Start(f) }); err != nil {
			tr = ""
		}
	}
	if err != nil {
		stop()
	}
	return stop, err
}

func convertFile(in, out string, quality float64, pages string, verbose, quiet, lossy bool) error {
	start := time.Now()
	if data, err := os.ReadFile(in); err == nil && isPDF(data) {
		return reencodeFile(in, data, out, quality, pages, verbose, quiet, lossy, start)
	}
	doc, err := djvu.Open(in)
	if err != nil {
		return err
	}
	var pr *progress
	if !quiet {
		pr = newProgress(filepath.Base(out), "pages")
	}
	defer pr.clear()
	opt := convert.Options{
		Producer: "dejapdf " + version,
		Quality:  quality,
		Warn: func(page int, err error) {
			msg := fmt.Sprintf("dejapdf: warning: %v", err)
			if page >= 0 {
				msg = fmt.Sprintf("dejapdf: page %d: %v", page+1, err)
			}
			if pr != nil {
				pr.warn(msg)
			} else {
				fmt.Fprintln(os.Stderr, msg)
			}
		},
	}
	if pr != nil {
		opt.Progress = pr.update
	}
	if pages != "" {
		if opt.Pages, err = parsePages(pages, len(doc.Pages)); err != nil {
			return err
		}
	}
	if verbose {
		opt.Stats = &convert.Stats{}
	}
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	err = convert.Convert(doc, f, opt)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(out)
		return err
	}
	pr.clear()
	if verbose {
		opt.Stats.Report(os.Stderr)
	}
	if fi, err := os.Stat(out); err == nil && !quiet {
		fmt.Println(summary(filepath.Base(out), fi.Size(), convert.DjVuSize(doc, opt.Pages), time.Since(start)))
	}
	return nil
}

// reencodeFile re-encodes the scan images of a PDF.
func reencodeFile(in string, data []byte, out string, quality float64, pages string, verbose, quiet, lossy bool,
	start time.Time) error {
	r, err := pdfread.Open(data)
	if errors.Is(err, pdfread.ErrPassword) {
		return fmt.Errorf("%s is password-protected; remove the password first, e.g. qpdf --decrypt --password=... in.pdf out.pdf", in)
	}
	if err != nil {
		return err
	}
	doc, err := scan.Find(r)
	if err != nil {
		return err
	}
	var sel []int
	if pages != "" {
		if sel, err = parsePages(pages, len(doc.Pages)); err != nil {
			return err
		}
	}
	var pr *progress
	if !quiet {
		pr = newProgress(filepath.Base(out), "images")
	}
	defer pr.clear()
	opt := reencode.Options{
		Lossy:   lossy,
		Quality: quality,
		Pages:   sel,
		Warn: func(err error) {
			msg := fmt.Sprintf("dejapdf: warning: %v", err)
			if pr != nil {
				pr.warn(msg)
			} else {
				fmt.Fprintln(os.Stderr, msg)
			}
		},
	}
	if pr != nil {
		opt.Progress = func(done, all int) {
			pr.update(convert.Progress{Page: done / 2, Pages: all / 2, Done: int64(done), Total: int64(all)})
		}
	}
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	st, err := reencode.Reencode(r, doc, f, opt)
	var none *reencode.NothingError
	if errors.As(err, &none) {
		// Written unchanged: the whole input, or its selected pages.
		if sel == nil {
			_, err = f.Write(data)
		} else {
			err = rewrite.Select(r, f, nil, sel)
		}
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(out)
		return err
	}
	pr.clear()
	if none != nil {
		if verbose {
			fmt.Fprintln(os.Stderr, none.Reason)
		}
		if fi, err := os.Stat(out); err == nil && !quiet {
			fmt.Printf("%s: %d kbytes, nothing to re-encode\n", filepath.Base(out), (fi.Size()+512)/1024)
		}
		return nil
	}
	// Never grow a file: the whole of it, re-encoded, not smaller than
	// it was is written as it was.
	if fi, err := os.Stat(out); err == nil && sel == nil && fi.Size() >= int64(len(data)) {
		if err := os.WriteFile(out, data, 0o644); err != nil {
			return err
		}
		if !quiet {
			fmt.Printf("%s: copied as is: re-encoding saved nothing\n", filepath.Base(out))
		}
		return nil
	}
	if verbose {
		fmt.Fprintf(os.Stderr, "%d of %d scan images re-encoded: %d to %d kbytes\n",
			st.Replaced, st.Images, (st.Before+512)/1024, (st.After+512)/1024)
	}
	if fi, err := os.Stat(out); err == nil && !quiet {
		fmt.Println(summary(filepath.Base(out), fi.Size(), int64(len(data)), time.Since(start)))
	}
	return nil
}

// summary is the line printed when a conversion is done.
func summary(name string, size, djvuSize int64, d time.Duration) string {
	s := fmt.Sprintf("%s: %d kbytes", name, (size+512)/1024)
	if djvuSize > 0 {
		s += fmt.Sprintf(", %.2fx", float64(size)/float64(djvuSize))
	}
	return s + fmt.Sprintf(" in %.1fs", d.Seconds())
}

// parsePages reads a list like "1-10,15,20-" into page indexes from 0.
func parsePages(s string, n int) ([]int, error) {
	var r []int
	seen := make([]bool, n)
	for _, part := range strings.Split(s, ",") {
		lo, hi, isRange := strings.Cut(part, "-")
		a, b := 1, n
		var err error
		if lo != "" || !isRange {
			if a, err = strconv.Atoi(lo); err != nil {
				return nil, fmt.Errorf("bad page range %q", part)
			}
			b = a
		}
		if isRange {
			b = n
			if hi != "" {
				if b, err = strconv.Atoi(hi); err != nil {
					return nil, fmt.Errorf("bad page range %q", part)
				}
			}
		}
		if a < 1 || b > n || a > b {
			return nil, fmt.Errorf("page range %q outside 1-%d", part, n)
		}
		for p := a; p <= b; p++ {
			if !seen[p-1] {
				seen[p-1] = true
				r = append(r, p-1)
			}
		}
	}
	return r, nil
}

func dumpFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if isPDF(data) {
		r, err := pdfread.Open(data)
		if err != nil {
			return err
		}
		doc, err := scan.Find(r)
		if err != nil {
			return err
		}
		scan.Dump(os.Stdout, r, doc)
		return nil
	}
	root, err := iff.Parse(data)
	if err != nil {
		return err
	}
	return djvu.Dump(os.Stdout, root)
}

// isPDF reports whether data starts like a PDF file; a header may follow
// up to 1 KB of junk.
func isPDF(data []byte) bool {
	return bytes.Contains(data[:min(len(data), 1024)], []byte("%PDF-"))
}
