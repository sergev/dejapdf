// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package convert

import (
	"slices"
	"sync"

	"github.com/sergev/dejapdf/internal/djvu"
	"github.com/sergev/dejapdf/internal/iw44"
)

// colourBudget caps the colour-layer pixels of the pages being built at
// once: about 25 bytes each while decoded and re-encoded, so two
// 600 dpi colour pages, ~1 GB. A larger page runs alone.
var colourBudget int64 = 48 << 20

// scheduler hands out pages to build, the largest first so that no
// slow page is left for the end, within colourBudget.
type scheduler struct {
	mu      sync.Mutex
	cond    sync.Cond
	pending []int   // indexes into the selection, largest first
	cost    []int64 // colour pixels of each
	used    int64
}

func newScheduler(doc *djvu.Document, sel []int, weight []int64) *scheduler {
	s := &scheduler{pending: make([]int, len(sel)), cost: make([]int64, len(sel))}
	s.cond.L = &s.mu
	for k, i := range sel {
		s.pending[k] = k
		s.cost[k] = colourPixels(doc.Pages[i])
	}
	slices.SortStableFunc(s.pending, func(a, b int) int {
		switch {
		case weight[a] > weight[b]:
			return -1
		case weight[a] < weight[b]:
			return 1
		}
		return 0
	})
	return s
}

// take returns the next page to build, waiting for budget; false when
// none is left.
func (s *scheduler) take() (int, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for len(s.pending) > 0 {
		for j, k := range s.pending {
			if c := s.cost[k]; c == 0 || s.used == 0 || s.used+c <= colourBudget {
				s.pending = slices.Delete(s.pending, j, j+1)
				s.used += c
				return k, true
			}
		}
		s.cond.Wait()
	}
	return 0, false
}

// done returns page k's budget.
func (s *scheduler) done(k int) {
	s.mu.Lock()
	s.used -= s.cost[k]
	s.mu.Unlock()
	s.cond.Broadcast()
}

// colourPixels is the size of the page's IW44 layers.
func colourPixels(pg *djvu.Page) int64 {
	if pg.Form == nil {
		return 0
	}
	var n int64
	for _, id := range []string{"BG44", "FG44"} {
		if c := pg.Form.Find(id); c != nil {
			if w, h, ok := iw44.Size(c.Data); ok {
				n += int64(w) * int64(h)
			}
		}
	}
	return n
}
