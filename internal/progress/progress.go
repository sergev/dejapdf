// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

// Package progress measures how far a conversion is through its phases.
package progress

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// Status is what a progress line shows.
type Status struct {
	Phase     string  // "reading", "finding images", "images", "pages", "writing"
	Unit      string  // what Count counts, in the main phase; "" otherwise
	Count, Of int     // units done, of all
	Frac      float64 // the whole done, 0 to 1
}

// PhaseTime is how long a phase took.
type PhaseTime struct {
	Phase string
	D     time.Duration
}

// Meter adds up the work done in phases. Each phase takes a share of
// what the earlier ones left, so the fraction done never goes back.
// It is safe for concurrent use; a nil Meter does nothing.
type Meter struct {
	mu          sync.Mutex
	fn          func(Status)
	base, span  float64 // the phase covers base to base+span
	units, done int64
	st          Status
	start       time.Time
	times       []PhaseTime
}

// New returns a meter telling fn (if not nil) of each change.
func New(fn func(Status)) *Meter {
	return &Meter{fn: fn}
}

// Start ends the current phase and starts one taking share of what is
// left, done in units of work. A non-empty unit is what Count counts.
func (m *Meter) Start(phase, unit string, share float64, units int64) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.end()
	m.base += m.span
	m.span = min(max(share, 0), 1) * (1 - m.base)
	m.units, m.done = max(units, 1), 0
	m.st = Status{Phase: phase, Unit: unit, Frac: m.base}
	m.start = time.Now()
	m.report()
}

// Add counts units of work done in the current phase.
func (m *Meter) Add(units int64) {
	if m == nil || units == 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.done = min(m.done+units, m.units)
	m.st.Frac = m.base + m.span*float64(m.done)/float64(m.units)
	m.report()
}

// Set says done of units of the current phase's work are done; units
// replaces the count given to Start.
func (m *Meter) Set(done, units int64) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.units = max(units, 1)
	m.done = min(max(done, 0), m.units)
	m.st.Frac = m.base + m.span*float64(m.done)/float64(m.units)
	m.report()
}

// Count sets what the progress line counts.
func (m *Meter) Count(n, of int) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.st.Count, m.st.Of = n, of
	m.report()
}

// Finish ends the last phase.
func (m *Meter) Finish() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.end()
	m.base, m.span = 1, 0
	m.st.Frac = 1
	m.report()
}

// Times returns how long each phase took, those of one name added up.
func (m *Meter) Times() []PhaseTime {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]PhaseTime(nil), m.times...)
}

// String is like "reading 0.2s, images 7.1s, writing 0.6s".
func (m *Meter) String() string {
	var s []string
	for _, t := range m.Times() {
		s = append(s, fmt.Sprintf("%s %.1fs", t.Phase, t.D.Seconds()))
	}
	return strings.Join(s, ", ")
}

func (m *Meter) end() {
	if m.start.IsZero() {
		return
	}
	d := time.Since(m.start)
	m.start = time.Time{}
	for i := range m.times {
		if m.times[i].Phase == m.st.Phase {
			m.times[i].D += d
			return
		}
	}
	m.times = append(m.times, PhaseTime{m.st.Phase, d})
}

func (m *Meter) report() {
	if m.fn != nil {
		m.fn(m.st)
	}
}
