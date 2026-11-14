package jpx

import (
	"math"
	"math/rand"
	"testing"
)

func TestDWTRoundTrip(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for _, sz := range [][3]int{{1, 1, 0}, {2, 3, 1}, {17, 9, 3}, {64, 64, 5}, {213, 275, 5}} {
		w, h, nl := sz[0], sz[1], sz[2]
		p := make([]float32, w*h)
		q := make([]float32, w*h)
		for i := range p {
			p[i] = float32(r.Intn(256) - 128)
		}
		copy(q, p)
		forward(q, w, h, nl)
		inverse(q, w, h, nl)
		for i := range p {
			if math.Abs(float64(p[i]-q[i])) > 1e-3 {
				t.Fatalf("%dx%d: sample %d: %v, want %v", w, h, i, q[i], p[i])
			}
		}
	}
}

// TestNorms compares the 2-D synthesis norms with OpenJPEG's table, which
// scales high-pass bands by 2 (its decoder uses 2/K, not 1/K).
func TestNorms(t *testing.T) {
	ll := []float64{1.965, 4.177, 8.403, 16.90, 33.84}
	hl := []float64{2.022, 3.989, 8.355, 17.04, 34.27}
	hh := []float64{2.080, 3.865, 8.307, 17.18, 34.71}
	for n := 1; n <= 5; n++ {
		lo, hi := norms97(n)
		for _, c := range []struct {
			name      string
			got, want float64
		}{{"LL", lo * lo, ll[n-1]}, {"HL", 2 * lo * hi, hl[n-1]}, {"HH", 4 * hi * hi, hh[n-1]}} {
			if math.Abs(c.got-c.want) > 0.02*c.want {
				t.Errorf("level %d %s: %.3f, want %.3f", n, c.name, c.got, c.want)
			}
		}
	}
}
