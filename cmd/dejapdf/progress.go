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

const progressEvery = 500 * time.Millisecond // between redraws

// progress draws a progress line on stdout, which must be a terminal.
// Warnings go through it so that they do not break the line.
type progress struct {
	mu    sync.Mutex
	name  string
	unit  string // what is counted: pages, images
	width int
	dumb  bool
	shown int // length of the line on screen, 0 if none
	p     convert.Progress
	tick  *time.Ticker
	done  bool // cleared: draw no more
}

// newProgress returns nil if stdout is not a terminal.
func newProgress(name, unit string) *progress {
	fi, err := os.Stdout.Stat()
	if err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		return nil
	}
	w, err := strconv.Atoi(os.Getenv("COLUMNS"))
	if err != nil || w <= 0 {
		w = 80
	}
	r := &progress{name: name, unit: unit, width: w - 1, dumb: os.Getenv("TERM") == "dumb", tick: time.NewTicker(progressEvery)}
	go func() {
		for range r.tick.C {
			r.mu.Lock()
			if !r.done {
				r.draw()
			}
			r.mu.Unlock()
		}
	}()
	return r
}

func (r *progress) update(p convert.Progress) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.p = p
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
	r.tick.Stop()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.done = true
	r.erase()
}

func (r *progress) draw() {
	s := progressLine(r.name, r.unit, r.p, r.width)
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

// progressLine is like "book.pdf: 45%, pages 12/27", the name cut
// to fit in width.
func progressLine(name, unit string, p convert.Progress, width int) string {
	f := 0.0
	switch {
	case p.Total > 0:
		f = float64(p.Done) / float64(p.Total)
	case p.Pages > 0:
		f = float64(p.Page) / float64(p.Pages)
	}
	f = min(max(f, 0), 1)
	tail := fmt.Sprintf(": %d%%", int(f*100))
	if p.Pages > 0 {
		tail += fmt.Sprintf(", %s %d/%d", unit, p.Page, p.Pages)
	}
	if n, room := utf8.RuneCountInString(name), width-len(tail); n > room {
		if room < 2 {
			return tail[2:]
		}
		name = "…" + string([]rune(name)[n-room+1:])
	}
	return name + tail
}
