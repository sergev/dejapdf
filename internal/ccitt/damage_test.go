// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package ccitt

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Tests of what dejapdf changed: fill bits before an EOL, and the row
// a damaged stream breaks off in.

const gopherW, gopherH, gopherStride = 153, 55, 20

func readFile(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func decodeAll(data []byte, sf SubFormat, h int) ([]byte, error) {
	return io.ReadAll(NewReader(bytes.NewReader(data), MSB, sf, gopherW, h, nil))
}

// white reports whether pixel x of a packed row is white.
func white(row []byte, x int) bool { return row[x>>3]>>(7-x&7)&1 != 0 }

// A stream cut short gives its whole rows, then the row it broke off
// in as far as it decoded, the rest white, and an error.
func TestCutRow(t *testing.T) {
	for _, tc := range []struct {
		name string
		sf   SubFormat
	}{
		{"bw-gopher.ccitt_group3", Group3},
		{"bw-gopher.ccitt_group4", Group4},
	} {
		data := readFile(t, tc.name)
		want, err := decodeAll(data, tc.sf, gopherH)
		if err != nil {
			t.Fatal(err)
		}
		partial, kept := 0, 0
		for n := 1; n < len(data); n++ {
			got, err := decodeAll(data[:n], tc.sf, gopherH)
			if len(got) == len(want) && err == nil {
				if !bytes.Equal(got, want) { // cut in the final EOLs
					t.Errorf("%s cut at %d: image differs", tc.name, n)
				}
				continue
			}
			if err == nil {
				t.Errorf("%s cut at %d: no error", tc.name, n)
			}
			if len(got)%gopherStride != 0 {
				t.Fatalf("%s cut at %d: %d bytes, not whole rows", tc.name, n, len(got))
			}
			rows := len(got) / gopherStride
			if rows == 0 {
				continue
			}
			last := (rows - 1) * gopherStride
			if !bytes.Equal(got[:last], want[:last]) {
				t.Fatalf("%s cut at %d: rows before the cut differ", tc.name, n)
			}
			g, w := got[last:], want[last:last+gopherStride]
			if bytes.Equal(g, w) {
				continue
			}
			partial++
			x := 0
			for x < gopherW && white(g, x) == white(w, x) {
				x++
			}
			for k := x; k < gopherW; k++ {
				if !white(g, k) {
					t.Fatalf("%s cut at %d: row %d black at %d, past where it decoded (%d)",
						tc.name, n, rows-1, k, x)
				}
			}
			for k := range x {
				if !white(g, k) {
					kept++
					break
				}
			}
		}
		// The cuts must reach rows broken off with some ink decoded.
		if partial == 0 || kept == 0 {
			t.Errorf("%s: %d rows broken off, %d with ink kept", tc.name, partial, kept)
		}
	}
}

// Fill bits, zeros before an EOL as T.4 allows, are skipped: up to 36,
// so the bits read still fit the 64 that can be unread.
func TestFillBits(t *testing.T) {
	data := readFile(t, "bw-gopher.ccitt_group3")
	want, err := decodeAll(data, Group3, gopherH)
	if err != nil {
		t.Fatal(err)
	}
	var bits strings.Builder
	for _, b := range data {
		for i := 7; i >= 0; i-- {
			bits.WriteByte('0' + b>>i&1)
		}
	}
	// Eleven zeros and a one are only ever an EOL.
	const eol = "000000000001"
	parts := strings.Split(bits.String(), eol)
	if len(parts) <= gopherH {
		t.Fatalf("%d EOLs, want over %d", len(parts)-1, gopherH)
	}
	var filled strings.Builder
	for i, p := range parts {
		if i > 0 {
			filled.WriteString(strings.Repeat("0", i%37))
			filled.WriteString(eol)
		}
		filled.WriteString(p)
	}
	s := filled.String()
	packed := make([]byte, (len(s)+7)/8)
	for i := range len(s) {
		packed[i/8] |= (s[i] - '0') << (7 - i%8)
	}
	for _, h := range []int{gopherH, AutoDetectHeight} {
		got, err := decodeAll(packed, Group3, h)
		if err != nil {
			t.Errorf("height %d: %v", h, err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("height %d: image differs", h)
		}
	}
}

// Damaged streams decode without panics.
func FuzzDecode(f *testing.F) {
	names, err := filepath.Glob("testdata/*.ccitt_group*")
	if err != nil || len(names) == 0 {
		f.Fatal("no testdata")
	}
	for _, name := range names {
		b, err := os.ReadFile(name)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(b, strings.HasSuffix(name, "4"), strings.Contains(name, "aligned"), uint16(gopherW))
	}
	f.Fuzz(func(t *testing.T, data []byte, g4, align bool, w uint16) {
		if w == 0 || w > 4096 {
			return
		}
		sf := Group3
		if g4 {
			sf = Group4
		}
		for _, h := range []int{gopherH, AutoDetectHeight} {
			r := NewReader(bytes.NewReader(data), MSB, sf, int(w), h, &Options{Align: align})
			io.Copy(io.Discard, io.LimitReader(r, 1<<22))
		}
	})
}
