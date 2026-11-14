// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

// Command dejapdf converts DjVu documents to layered PDF.
package main

import (
	"flag"
	"fmt"
	"os"

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
	if err := convertFile(flag.Arg(0), flag.Arg(1), *quality); err != nil {
		fmt.Fprintln(os.Stderr, "dejapdf:", err)
		os.Exit(1)
	}
}

func convertFile(in, out string, quality float64) error {
	doc, err := djvu.Open(in)
	if err != nil {
		return err
	}
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	err = convert.Convert(doc, f, convert.Options{Producer: "dejapdf " + version, Quality: quality})
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(out)
	}
	return err
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
