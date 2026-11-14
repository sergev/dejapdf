// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

// Command dejapdf converts DjVu documents to layered PDF.
package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sergev/dejapdf/internal/convert"
	"github.com/sergev/dejapdf/internal/djvu"
	"github.com/sergev/dejapdf/internal/iff"
)

const version = "0.0.1"

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: dejapdf [options] input.djvu output.pdf\n")
	fmt.Fprintf(os.Stderr, "       dejapdf -dump input.djvu\n")
	flag.PrintDefaults()
}

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	dump := flag.Bool("dump", false, "print the chunk tree, like djvudump")
	quality := flag.Float64("q", convert.DefaultQuality, "colour layer size as a multiple of the DjVu size")
	pages := flag.String("p", "", "pages to convert, like 1-10,15 (default all)")
	verbose := flag.Bool("v", false, "show progress and a size report by layer")
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
	if err := convertFile(flag.Arg(0), flag.Arg(1), *quality, *pages, *verbose); err != nil {
		fmt.Fprintln(os.Stderr, "dejapdf:", err)
		os.Exit(1)
	}
}

func convertFile(in, out string, quality float64, pages string, verbose bool) error {
	start := time.Now()
	doc, err := djvu.Open(in)
	if err != nil {
		return err
	}
	opt := convert.Options{
		Producer: "dejapdf " + version,
		Quality:  quality,
		Warn: func(page int, err error) {
			if page < 0 {
				fmt.Fprintf(os.Stderr, "dejapdf: warning: %v\n", err)
				return
			}
			fmt.Fprintf(os.Stderr, "dejapdf: page %d: %v\n", page+1, err)
		},
	}
	if pages != "" {
		if opt.Pages, err = parsePages(pages, len(doc.Pages)); err != nil {
			return err
		}
	}
	if verbose {
		opt.Stats = &convert.Stats{}
		opt.Progress = func(done, total int) {
			fmt.Fprintf(os.Stderr, "\rpage %d/%d", done, total)
			if done == total {
				fmt.Fprintln(os.Stderr)
			}
		}
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
	if verbose {
		opt.Stats.Report(os.Stderr)
	}
	if fi, err := os.Stat(out); err == nil {
		fmt.Println(summary(out, fi.Size(), convert.DjVuSize(doc, opt.Pages), time.Since(start)))
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
	root, err := iff.Parse(data)
	if err != nil {
		return err
	}
	return djvu.Dump(os.Stdout, root)
}
