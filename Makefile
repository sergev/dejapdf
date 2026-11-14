.PHONY: all build test lint corpus clean

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

clean:
	rm -f dejapdf
