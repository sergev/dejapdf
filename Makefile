# Copyright (c) 2026 Serge Vakulenko
# SPDX-License-Identifier: MIT

.PHONY: all build test lint corpus docs docs-pdf clean

all: build

build:
	go build -o dejapdf ./cmd/dejapdf

test:
	go test ./...

lint:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi
	go vet ./...

# Regenerate testdata/corpus; needs djvulibre-bin and ghostscript.
corpus:
	testdata/mkcorpus.sh

# docs: the SVG figures and the articles' PDFs; docs-pdf: only the PDFs
# (pandoc with typst). The PDFs are git-ignored.
docs:
	$(MAKE) -C docs

docs-pdf:
	$(MAKE) -C docs pdf

clean:
	rm -f dejapdf
