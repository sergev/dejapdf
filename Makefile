# Copyright (c) 2026 Serge Vakulenko
# SPDX-License-Identifier: MIT

.PHONY: all build install test lint fuzz corpus docs docs-pdf clean

BINDIR ?= $(HOME)/.local/bin

all: build

build:
	go build -o dejapdf ./cmd/dejapdf

install: build
	install -d $(BINDIR)
	install -m 755 dejapdf $(BINDIR)/dejapdf

test:
	go test ./...

lint:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi
	go vet ./...

# Run every fuzz target for FUZZTIME. Few workers and a memory cap:
# one worker per CPU can exhaust memory. macOS cannot limit virtual memory
# (ulimit -v), so there only GOMEMLIMIT applies.
FUZZTIME ?= 60s
fuzz:
	@for pkg in $$(go list ./internal/...); do \
		for t in $$(go test $$pkg -list '^Fuzz' | grep '^Fuzz'); do \
			echo "$$pkg $$t"; \
			out=$$( ulimit -v 16000000 2>/dev/null; GOMEMLIMIT=1GiB go test $$pkg -run '^$$' -fuzz "^$$t$$" \
				-fuzztime $(FUZZTIME) -parallel 4 2>&1 ) || { echo "$$out"; exit 1; }; \
			echo "$$out" | tail -1; \
		done; \
	done

# Regenerate testdata/corpus; needs djvulibre-bin and ghostscript.
corpus:
	testdata/mkcorpus.sh

# docs: the SVG figures and the articles' PDFs; docs-pdf: only the PDFs
# (pandoc with typst). The PDFs are committed.
docs:
	$(MAKE) -C docs

docs-pdf:
	$(MAKE) -C docs pdf

clean:
	rm -f dejapdf
