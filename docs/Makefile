PYTHON ?= python3
PANDOC ?= pandoc

# Each figures/NAME.py prints figures/NAME.svg; svg.py is the shared helper.
# The SVGs are kept in git so that GitHub shows them.
FIGURES = $(patsubst %.py,%.svg,$(filter-out figures/svg.py,$(wildcard figures/*.py)))

# Articles to PDF via pandoc and typst. The PDFs are git-ignored (docs/*.pdf);
# only these are made or removed, never the downloaded specs.
ARTICLES = $(filter-out README.md,$(wildcard *.md))
PDFS = $(patsubst %.md,%.pdf,$(ARTICLES))
PANDOCFLAGS = -f markdown+gfm_auto_identifiers --pdf-engine=typst --columns=200 \
	--include-in-header=style.typ -V papersize=a4 -V margin.x=2cm -V margin.y=2cm

all: figures pdf

figures: $(FIGURES)

pdf: $(PDFS)

figures/%.svg: figures/%.py figures/svg.py
	cd figures && $(PYTHON) -B $*.py > $*.svg

%.pdf: %.md style.typ $(FIGURES)
	$(PANDOC) $(PANDOCFLAGS) $< -o $@

clean:
	rm -f $(FIGURES) $(PDFS)

.PHONY: all figures pdf clean
