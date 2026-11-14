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
	if err := convertFile(flag.Arg(0), flag.Arg(1)); err != nil {
		fmt.Fprintln(os.Stderr, "dejapdf:", err)
		os.Exit(1)
	}
}

func convertFile(in, out string) error {
	doc, err := djvu.Open(in)
	if err != nil {
		return err
	}
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	err = convert.Convert(doc, f, convert.Options{Producer: "dejapdf " + version})
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
