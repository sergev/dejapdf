// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package convert

import (
	"github.com/sergev/dejapdf/internal/iw44"
	"github.com/sergev/dejapdf/internal/jb2"
)

// fillUnused replaces the foreground pixels that no mask pixel shows
// with the average of their nearest shown neighbours, spreading out
// breadth-first. Shown colours are unchanged, so a nearest-neighbour
// render is too, but a viewer that interpolates the upscaled layer no
// longer bleeds unrelated colours into the shapes, and the layer
// codes smaller. Each fg pixel covers k×k pixels of m, from the top left.
func fillUnused(fg *iw44.Image, m *jb2.Bitmap, k int) {
	w, h := fg.W, fg.H
	n := 3
	if fg.Gray {
		n = 1
	}
	done := make([]bool, w*h)
	var queue []int
	for y := range h {
		for x := range w {
		cell:
			for r := y * k; r < min((y+1)*k, m.H); r++ {
				for _, v := range m.Pix[r*m.W+x*k : r*m.W+min((x+1)*k, m.W)] {
					if v != 0 {
						done[y*w+x] = true
						queue = append(queue, y*w+x)
						break cell
					}
				}
			}
		}
	}
	if len(queue) == 0 {
		return
	}
	// Each round assigns the unfilled pixels next to the filled ones.
	var next []int
	seen := append([]bool(nil), done...)
	for len(queue) > 0 {
		next = next[:0]
		for _, i := range queue {
			x, y := i%w, i/w
			for _, d := range [4][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
				nx, ny := x+d[0], y+d[1]
				if nx < 0 || ny < 0 || nx >= w || ny >= h || seen[ny*w+nx] {
					continue
				}
				seen[ny*w+nx] = true
				next = append(next, ny*w+nx)
			}
		}
		for _, i := range next {
			x, y := i%w, i/w
			var sum [3]int
			cnt := 0
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					nx, ny := x+dx, y+dy
					if nx < 0 || ny < 0 || nx >= w || ny >= h || !done[ny*w+nx] {
						continue
					}
					for c := range n {
						sum[c] += int(fg.Pix[n*(ny*w+nx)+c])
					}
					cnt++
				}
			}
			for c := range n {
				fg.Pix[n*i+c] = byte((sum[c] + cnt/2) / cnt)
			}
		}
		for _, i := range next {
			done[i] = true
		}
		queue, next = next, queue
	}
}
