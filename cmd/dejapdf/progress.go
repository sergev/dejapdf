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

	"github.com/sergev/dejapdf/internal/progress"
)

const progressEvery = 500 * time.Millisecond // between redraws

// statusLine draws a progress line on stdout, which must be a terminal.
// Warnings go through it so that they do not break the line.
type statusLine struct {
	mu    sync.Mutex
	name  string
	width int
	dumb  bool
	shown int // length of the line on screen, 0 if none
	st    progress.Status
	tick  *time.Ticker
	done  bool // cleared: draw no more
}

// newStatusLine returns nil if stdout is not a terminal.
func newStatusLine(name string) *statusLine {
	fi, err := os.Stdout.Stat()
	if err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		return nil
	}
	w, err := strconv.Atoi(os.Getenv("COLUMNS"))
	if err != nil || w <= 0 {
		w = 80
	}
	r := &statusLine{name: name, width: w - 1, dumb: os.Getenv("TERM") == "dumb", tick: time.NewTicker(progressEvery)}
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

// meter returns a progress meter updating the line, if any.
func (r *statusLine) meter() *progress.Meter {
	if r == nil {
		return progress.New(nil)
	}
	return progress.New(r.update)
}

func (r *statusLine) update(st progress.Status) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.st = st
}

// warn prints a message on stderr, above the line if there is one.
func (r *statusLine) warn(msg string) {
	if r == nil {
		fmt.Fprintln(os.Stderr, msg)
		return
	}
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
func (r *statusLine) clear() {
	if r == nil {
		return
	}
	r.tick.Stop()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.done = true
	r.erase()
}

func (r *statusLine) draw() {
	s := progressLine(r.name, r.st, r.width)
	pad := ""
	if n := utf8.RuneCountInString(s); n < r.shown {
		pad = strings.Repeat(" ", r.shown-n)
	}
	fmt.Print("\r" + s + pad)
	r.shown = utf8.RuneCountInString(s)
}

func (r *statusLine) erase() {
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

// progressLine is like "book.pdf: 45%, pages 12/27" in the main phase,
// "book.pdf: 2%, reading" in others; the name cut to fit in width.
func progressLine(name string, st progress.Status, width int) string {
	tail := fmt.Sprintf(": %d%%", int(min(max(st.Frac, 0), 1)*100))
	switch {
	case st.Unit != "":
		tail += fmt.Sprintf(", %s %d/%d", st.Unit, st.Count, st.Of)
	case st.Phase != "":
		tail += ", " + st.Phase
	}
	if n, room := utf8.RuneCountInString(name), width-len(tail); n > room {
		if room < 2 {
			return tail[2:]
		}
		name = "…" + string([]rune(name)[n-room+1:])
	}
	return name + tail
}
