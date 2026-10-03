package chart

import (
	"math"
	"testing"
)

func TestLinearTicksNicePositions(t *testing.T) {
	ticks := LinearTicks(0, 100, 5)
	if len(ticks) == 0 {
		t.Fatal("no ticks")
	}
	for _, tk := range ticks {
		if tk.Value < -1e-9 || tk.Value > 100+1e-9 {
			t.Fatalf("tick %v out of range", tk.Value)
		}
		// Step must be 1/2/5-scaled: values must be "round".
		if math.Abs(tk.Value-math.Round(tk.Value/25)*25) > 1e-9 &&
			math.Abs(tk.Value-math.Round(tk.Value/20)*20) > 1e-9 &&
			math.Abs(tk.Value-math.Round(tk.Value/50)*50) > 1e-9 {
			t.Fatalf("tick %v not at a nice position", tk.Value)
		}
		if tk.Label == "" {
			t.Fatal("empty label")
		}
	}
}

func TestLinearTicksDegenerate(t *testing.T) {
	// All data identical.
	ticks := LinearTicks(5, 5, 5)
	if len(ticks) == 0 {
		t.Fatal("no ticks for degenerate range")
	}
	// Empty/invalid inputs fall back without crashing.
	for _, r := range [][2]float64{{0, 0}, {math.NaN(), 1}, {1, math.Inf(1)}, {10, 1}} {
		ticks := LinearTicks(r[0], r[1], 5)
		if len(ticks) == 0 {
			t.Fatalf("no ticks for range %v", r)
		}
	}
}

func TestLinearTicksDeterministic(t *testing.T) {
	a := LinearTicks(-3.7, 912.3, 6)
	b := LinearTicks(-3.7, 912.3, 6)
	if len(a) != len(b) {
		t.Fatal("nondeterministic tick count")
	}
	for i := range a {
		if a[i].Value != b[i].Value || a[i].Label != b[i].Label {
			t.Fatalf("tick %d differs", i)
		}
	}
}

func TestLogTicksPowersOfTen(t *testing.T) {
	ticks := LogTicks(0.5, 5000, 6)
	if len(ticks) < 3 {
		t.Fatalf("too few log ticks: %v", ticks)
	}
	for _, tk := range ticks {
		if tk.Value <= 0 {
			t.Fatalf("non-positive log tick %v", tk.Value)
		}
	}
	// Wide ranges use pure powers of ten.
	for _, tk := range ticks {
		e := math.Log10(tk.Value)
		if math.Abs(e-math.Round(e)) > 1e-9 {
			t.Fatalf("wide-range tick %v not a power of ten", tk.Value)
		}
	}
}

func TestLogTicksSubDecade(t *testing.T) {
	ticks := LogTicks(2, 8, 6)
	if len(ticks) < 2 {
		t.Fatalf("sub-decade range needs intermediate ticks: %v", ticks)
	}
}

func TestLogTicksFallback(t *testing.T) {
	ticks := LogTicks(-5, 0, 6)
	if len(ticks) == 0 {
		t.Fatal("log ticks must fall back for non-positive range")
	}
}

func TestTimeTicksAlignedUTC(t *testing.T) {
	// One day span.
	min := 1700000000.0
	max := min + 86400
	ticks := TimeTicks(min, max, 6)
	if len(ticks) < 2 {
		t.Fatalf("too few time ticks: %v", ticks)
	}
	for _, tk := range ticks {
		if tk.Value < min-1 || tk.Value > max+1 {
			t.Fatalf("time tick %v out of range", tk.Value)
		}
		if tk.Label == "" {
			t.Fatal("empty time label")
		}
	}
	// Determinism.
	again := TimeTicks(min, max, 6)
	for i := range ticks {
		if ticks[i] != again[i] {
			t.Fatal("time ticks nondeterministic")
		}
	}
}

func TestTimeTicksYearSpan(t *testing.T) {
	min := 1600000000.0
	ticks := TimeTicks(min, min+400*86400, 6)
	if len(ticks) < 2 {
		t.Fatal("year-span ticks missing")
	}
}
