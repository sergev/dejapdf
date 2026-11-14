// Copyright (c) 2026 Serge Vakulenko
// SPDX-License-Identifier: MIT

package bzz

import (
	"bytes"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Fixtures made with DjVuLibre's `bzz -e` (text.bzz: 10K blocks).
func TestFixtures(t *testing.T) {
	ins, _ := filepath.Glob("testdata/*.in")
	if len(ins) == 0 {
		t.Fatal("no fixtures")
	}
	for _, in := range ins {
		want, err := os.ReadFile(in)
		if err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(strings.TrimSuffix(in, ".in") + ".bzz")
		if err != nil {
			t.Fatal(err)
		}
		got, err := Decode(b)
		if err != nil || !bytes.Equal(got, want) {
			t.Errorf("%s: err %v, %d bytes, want %d", in, err, len(got), len(want))
		}
	}
}

func TestEmpty(t *testing.T) {
	// `bzz -e` writes nothing for empty input; a zero block size ends it.
	if got, err := Decode(nil); err != nil || len(got) != 0 {
		t.Errorf("got %d bytes, %v", len(got), err)
	}
}

// Round trip through the external encoder on varied inputs.
func TestOracle(t *testing.T) {
	bin, err := exec.LookPath("bzz")
	if err != nil {
		t.Skip("bzz (djvulibre-bin) not installed")
	}
	dir := t.TempDir()
	r := rand.New(rand.NewSource(1))
	inputs := map[string][]byte{}
	for _, n := range []int{2, 17, 1000, 70000} {
		b := make([]byte, n)
		r.Read(b)
		inputs["random"+string(rune('a'+len(inputs)))] = b
	}
	skew := make([]byte, 200000)
	for i := range skew {
		skew[i] = byte(r.ExpFloat64() * 3)
	}
	inputs["skew"] = skew
	inputs["runs"] = bytes.Repeat([]byte("aaaaaaaaaaaaaaaaaaab"), 5000)
	for name, data := range inputs {
		for _, block := range []string{"-e", "-e10"} {
			in := filepath.Join(dir, name)
			out := in + block + ".bzz"
			if err := os.WriteFile(in, data, 0o644); err != nil {
				t.Fatal(err)
			}
			if b, err := exec.Command(bin, block, in, out).CombinedOutput(); err != nil {
				t.Fatalf("bzz: %v: %s", err, b)
			}
			enc, err := os.ReadFile(out)
			if err != nil {
				t.Fatal(err)
			}
			got, err := Decode(enc)
			if err != nil || !bytes.Equal(got, data) {
				t.Errorf("%s %s: err %v, %d bytes, want %d", name, block, err, len(got), len(data))
			}
		}
	}
}

func FuzzDecode(f *testing.F) {
	for _, p := range []string{"testdata/one.bzz", "testdata/random.bzz"} {
		if b, err := os.ReadFile(p); err == nil {
			f.Add(b)
		}
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		Decode(b)
	})
}
