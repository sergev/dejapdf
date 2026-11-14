// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package convert

import (
	"testing"
	"time"
)

func TestScheduler(t *testing.T) {
	old := colourBudget
	defer func() { colourBudget = old }()
	colourBudget = 10
	// Pages 0-4 largest first: 3, 0, 1, 2, 4; page 3 is over budget.
	s := &scheduler{pending: []int{3, 0, 1, 2, 4}, cost: []int64{6, 0, 6, 25, 4}}
	s.cond.L = &s.mu
	take := func(want int) {
		t.Helper()
		if k, ok := s.take(); !ok || k != want {
			t.Fatalf("take = %d, %v; want %d", k, ok, want)
		}
	}
	// waits checks that take blocks until free(), then returns want.
	waits := func(want int, free func()) {
		t.Helper()
		c := make(chan int)
		go func() { k, _ := s.take(); c <- k }()
		select {
		case k := <-c:
			t.Fatalf("take = %d without waiting", k)
		case <-time.After(50 * time.Millisecond):
		}
		free()
		if k := <-c; k != want {
			t.Fatalf("take = %d, want %d", k, want)
		}
	}
	take(3)                        // oversized, runs alone
	take(1)                        // no colour
	waits(0, func() { s.done(3) }) // 6
	take(4)                        // 6+4 fits
	waits(2, func() { s.done(0) }) // 4+6 fits
	for _, k := range []int{1, 4, 2} {
		s.done(k)
	}
	if _, ok := s.take(); ok {
		t.Error("take after the last page")
	}
	if s.used != 0 {
		t.Errorf("used %d at the end", s.used)
	}
}
