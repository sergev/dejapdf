// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package progress

import (
	"math"
	"sync"
	"testing"
)

func TestMeter(t *testing.T) {
	var got []Status
	m := New(func(st Status) { got = append(got, st) })
	last := -1.0
	check := func(want float64) {
		t.Helper()
		f := got[len(got)-1].Frac
		if math.Abs(f-want) > 1e-9 {
			t.Errorf("frac %v, want %v", f, want)
		}
		if f < last-1e-12 {
			t.Errorf("frac went back: %v after %v", f, last)
		}
		last = f
	}
	m.Start("reading", "", 0.1, 1)
	check(0)
	m.Start("finding images", "", 0.1, 4) // 0.1 of the 0.9 left
	check(0.1)
	m.Set(2, 4)
	check(0.1 + 0.045)
	m.Start("images", "images", 0.5, 10) // reading's span is used up
	check(0.19)
	m.Add(5)
	check(0.19 + 0.2025)
	m.Add(100) // capped
	check(0.595)
	m.Count(3, 7)
	if st := got[len(got)-1]; st.Count != 3 || st.Of != 7 || st.Unit != "images" {
		t.Errorf("status %+v", st)
	}
	m.Start("writing", "", 1, 2)
	check(0.595)
	m.Add(1)
	check(0.7975)
	m.Finish()
	check(1)
	var names []string
	for _, pt := range m.Times() {
		names = append(names, pt.Phase)
	}
	if len(names) != 4 || names[3] != "writing" {
		t.Errorf("times %v", names)
	}
}

func TestMeterConcurrent(t *testing.T) {
	m := New(nil)
	m.Start("images", "images", 1, 1000)
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			for range 100 {
				m.Add(1)
			}
		})
	}
	wg.Wait()
	if f := m.st.Frac; math.Abs(f-1) > 1e-9 {
		t.Errorf("frac %v, want 1", f)
	}
}

func TestNilMeter(t *testing.T) {
	var m *Meter
	m.Start("reading", "", 0.5, 1)
	m.Add(1)
	m.Set(1, 2)
	m.Count(1, 2)
	m.Finish()
	if m.Times() != nil || m.String() != "" {
		t.Error("nil meter has times")
	}
}
