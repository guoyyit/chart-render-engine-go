package chart

import (
	"math"
	"math/rand"
	"testing"
)

func mkSeries(n int, f func(i int) float64) ([]float64, []*float64) {
	xs := make([]float64, n)
	ys := make([]*float64, n)
	for i := 0; i < n; i++ {
		xs[i] = float64(i)
		v := f(i)
		ys[i] = &v
	}
	return xs, ys
}

func TestDownsamplePreservesExtremaAndEnds(t *testing.T) {
	n := 10000
	xs, ys := mkSeries(n, func(i int) float64 {
		return math.Sin(float64(i)/50) * 100
	})
	// Global min/max known locations.
	loI, hiI := 117, 274 // sin min/max occur periodically; find them
	minV, maxV := math.Inf(1), math.Inf(-1)
	for i := 0; i < n; i++ {
		if *ys[i] < minV {
			minV, loI = *ys[i], i
		}
		if *ys[i] > maxV {
			maxV, hiI = *ys[i], i
		}
	}
	out := downsample(xs, ys, 400)
	if len(out) > 400 {
		t.Fatalf("output exceeds cap: %d", len(out))
	}
	if out[0].X != 0 {
		t.Fatalf("first point lost: %v", out[0].X)
	}
	if out[len(out)-1].X != float64(n-1) {
		t.Fatalf("last point lost: %v", out[len(out)-1].X)
	}
	foundMin, foundMax := false, false
	for _, p := range out {
		if p.Y == minV {
			foundMin = true
		}
		if p.Y == maxV {
			foundMax = true
		}
	}
	if !foundMin || !foundMax {
		t.Fatalf("global extrema lost (min@%d max@%d)", loI, hiI)
	}
}

func TestDownsampleDeterministic(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	n := 50000
	xs := make([]float64, n)
	ys := make([]*float64, n)
	for i := 0; i < n; i++ {
		xs[i] = float64(i)
		v := rng.NormFloat64() * 1000
		ys[i] = &v
	}
	a := downsample(xs, ys, 500)
	b := downsample(xs, ys, 500)
	if len(a) != len(b) {
		t.Fatal("nondeterministic length")
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("point %d differs", i)
		}
	}
}

func TestDownsampleGapsPreserved(t *testing.T) {
	n := 1000
	xs, ys := mkSeries(n, func(i int) float64 { return float64(i) })
	// Punch a hole of missing values.
	for i := 400; i < 600; i++ {
		ys[i] = nil
	}
	out := downsample(xs, ys, 100)
	sawGap := false
	for _, p := range out {
		if p.Gap {
			sawGap = true
		}
		if p.X >= 400 && p.X < 600 {
			t.Fatalf("missing point emitted: %v", p.X)
		}
	}
	if !sawGap {
		t.Fatal("gap marker lost after downsampling")
	}
}

func TestDownsampleSmallSeriesUnchanged(t *testing.T) {
	xs, ys := mkSeries(50, func(i int) float64 { return float64(i * i) })
	out := downsample(xs, ys, 1000)
	if len(out) != 50 {
		t.Fatalf("small series altered: %d", len(out))
	}
	for i, p := range out {
		if p.X != float64(i) || p.Y != float64(i*i) {
			t.Fatalf("point %d altered", i)
		}
	}
}

func TestDownsampleAllMissing(t *testing.T) {
	xs, ys := mkSeries(100, func(i int) float64 { return 0 })
	for i := range ys {
		ys[i] = nil
	}
	out := downsample(xs, ys, 10)
	if len(out) != 0 {
		t.Fatalf("all-missing series produced %d points", len(out))
	}
}

func TestDownsampleOutputBounded(t *testing.T) {
	for _, n := range []int{1000, 100000, 1000000} {
		xs, ys := mkSeries(n, func(i int) float64 { return float64(i % 97) })
		out := downsample(xs, ys, 200)
		if len(out) > 200 {
			t.Fatalf("n=%d: output %d exceeds cap", n, len(out))
		}
	}
}
