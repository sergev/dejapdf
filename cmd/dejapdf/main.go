// Command dejapdf converts DjVu documents to layered PDF.
package main

import (
	"flag"
	"fmt"
	"os"
)

const version = "0.0.1"

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: dejapdf [options] input.djvu output.pdf\n")
	flag.PrintDefaults()
}

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Usage = usage
	flag.Parse()

	if *showVersion {
		fmt.Println("dejapdf", version)
		return
	}
	if flag.NArg() != 2 {
		usage()
		os.Exit(2)
	}
	fmt.Fprintln(os.Stderr, "dejapdf: conversion not implemented yet")
	os.Exit(1)
}
