// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/sergev/dejapdf/internal/convert"
)

const (
	progressDelay = 300 * time.Millisecond // nothing shown before this
	progressEvery = 100 * time.Millisecond // between redraws
)

// progress draws a progress line on stdout, which must be a terminal.
// Warnings go through it so that they do not break the line.
type progress struct {
	mu    sync.Mutex
	name  string
	width int
	dumb  bool
	start time.Time
	last  time.Time
	shown int // length of the line on screen, 0 if none
	p     convert.Progress
}

// newProgress returns nil if stdout is not a terminal.
func newProgress(name string) *progress {
	fi, err := os.Stdout.Stat()
	if err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		return nil
	}
	w, err := strconv.Atoi(os.Getenv("COLUMNS"))
	if err != nil || w <= 0 {
		w = 80
	}
	return &progress{name: name, width: w - 1, dumb: os.Getenv("TERM") == "dumb", start: time.Now()}
}

func (r *progress) update(p convert.Progress) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.p = p
	now := time.Now()
	if now.Sub(r.start) < progressDelay || (now.Sub(r.last) < progressEvery && p.Page < p.Pages) {
		return
	}
	r.last = now
	r.draw()
}

// warn prints a message on stderr above the progress line.
func (r *progress) warn(msg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	shown := r.shown > 0
	r.erase()
	fmt.Fprintln(os.Stderr, msg)
	if shown {
		r.draw()
	}
}

// clear removes the line, before the summary or an error.
func (r *progress) clear() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.erase()
}

func (r *progress) draw() {
	s := progressLine(r.name, r.p, r.width, r.dumb)
	pad := ""
	if n := utf8.RuneCountInString(s); n < r.shown {
		pad = strings.Repeat(" ", r.shown-n)
	}
	fmt.Print("\r" + s + pad)
	r.shown = utf8.RuneCountInString(s)
}

func (r *progress) erase() {
	if r.shown == 0 {
		return
	}
	if r.dumb {
		fmt.Print("\r" + strings.Repeat(" ", r.shown) + "\r")
	} else {
		fmt.Print("\r\033[K")
	}
	r.shown = 0
}

// progressLine is like "book.pdf  [████▌     ]  45%  12/27  1.2 MB",
// fitted in width: the bar shrinks, then goes, then the name is cut.
func progressLine(name string, p convert.Progress, width int, dumb bool) string {
	f := 0.0
	switch {
	case p.Total > 0:
		f = float64(p.Done) / float64(p.Total)
	case p.Pages > 0:
		f = float64(p.Page) / float64(p.Pages)
	}
	f = min(max(f, 0), 1)
	// Fixed width, so that the bar does not move.
	tail := fmt.Sprintf("%3d%%  %*d/%d  %6.1f MB", int(f*100), len(strconv.Itoa(p.Pages)), p.Page, p.Pages,
		float64(p.Written)/(1<<20))
	n := utf8.RuneCountInString(name)
	if bw := width - n - len(tail) - 6; bw >= 10 {
		return name + "  [" + bar(f, min(bw, 40), dumb) + "]  " + tail
	}
	if room := width - len(tail) - 2; n > room {
		if room < 4 {
			return tail
		}
		name = "…" + string([]rune(name)[n-room+1:])
	}
	return name + "  " + tail
}

// bar is a progress bar of width cells, in eighths of a cell.
func bar(f float64, width int, dumb bool) string {
	if dumb {
		k := int(f * float64(width))
		return strings.Repeat("#", k) + strings.Repeat(" ", width-k)
	}
	e := int(f * float64(width*8))
	s := strings.Repeat("█", e/8)
	if e%8 > 0 {
		s += string([]rune("▏▎▍▌▋▊▉")[e%8-1])
	}
	return s + strings.Repeat(" ", width-utf8.RuneCountInString(s))
}
