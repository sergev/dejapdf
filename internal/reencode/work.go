// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package reencode

import (
	"sync"

	"github.com/sergev/dejapdf/internal/pdfread"
	"github.com/sergev/dejapdf/internal/progress"
	"github.com/sergev/dejapdf/internal/scan"
)

// passCost is the time of each pass over an image, in nanoseconds per
// pixel (on all CPUs): decode and split (and for gray and colour,
// segment and code the background), share the shapes of a group, code
// the stencils.
type passCost struct{ decode, share, code float64 }

// Measured on scanned books (16 CPUs); they only pace the progress line.
var (
	bilevelCost  = passCost{0.3, 0.8, 0.07}
	separateCost = passCost{25, 2, 0.15}
)

// copyCost is the time of writing the output, in nanoseconds per byte
// copied.
const copyCost = 10.0

// work tells the meter of the images re-encoded, by pass.
type work struct {
	r      *pdfread.Reader
	m      *progress.Meter
	mu     sync.Mutex
	n, all int
	copied int64 // bytes the output will copy, about
}

// newWork starts the images phase, its share of the rest set by the
// cost of the images against that of writing.
func newWork(r *pdfread.Reader, m *progress.Meter, ims, seps []*scan.Image) *work {
	w := &work{r: r, m: m, all: len(ims) + len(seps), copied: r.Size()}
	var total float64
	for _, c := range []struct {
		ims  []*scan.Image
		cost passCost
	}{{ims, bilevelCost}, {seps, separateCost}} {
		for _, im := range c.ims {
			total += pixels(im) * (c.cost.decode + c.cost.share + c.cost.code)
			if s, ok := r.Get(int(im.Ref)).(*pdfread.Stream); ok {
				w.copied -= int64(len(s.Raw))
			}
		}
	}
	w.copied = max(w.copied, 1)
	write := float64(w.copied) * copyCost
	m.Start("images", "images", total/(total+write), int64(total))
	m.Count(0, w.all)
	return w
}

func pixels(im *scan.Image) float64 { return float64(im.Width) * float64(im.Height) }

// add counts a pass over im.
func (w *work) add(im *scan.Image, cost float64) {
	w.m.Add(int64(pixels(im) * cost))
}

// coded counts the last pass over im.
func (w *work) coded(im *scan.Image, cost float64) {
	w.add(im, cost)
	w.mu.Lock()
	w.n++
	n := w.n
	w.mu.Unlock()
	w.m.Count(n, w.all)
}

// sharing returns a step for shareStep over the group ims, and a
// function counting what is left when it is done.
func (w *work) sharing(ims []*scan.Image, cost float64) (step func(done, all int), shared func()) {
	var units, added int64
	for _, im := range ims {
		units += int64(pixels(im) * cost)
	}
	step = func(done, all int) {
		u := units * int64(done) / int64(max(all, 1))
		w.m.Add(u - added)
		added = u
	}
	return step, func() { w.m.Add(units - added) }
}

// writing starts the phase writing the output.
func (w *work) writing() {
	w.m.Start("writing", "", 1, w.copied)
}
