// Command dejapdf converts DjVu documents to layered PDF.
package main

import (
	"flag"
	"fmt"
	"os"

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
	fmt.Fprintln(os.Stderr, "dejapdf: conversion not implemented yet")
	os.Exit(1)
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
