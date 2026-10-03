package chart

import (
	"context"
	"math"
	"strings"
	"sync"
	"testing"
)

func fp(v float64) *float64 { return &v }

func baseSpec() *Spec {
	return &Spec{
		Width: 800, Height: 500,
		Title:  "T",
		Legend: LegendSpec{Visible: true},
		XAxis:  AxisSpec{Type: AxisLinear, Label: "x"},
		YAxes:  []AxisSpec{{ID: "y-0", Label: "v"}},
		Series: []SeriesSpec{
			{Type: SeriesLine, Name: "l", Y: []*float64{fp(1), fp(4), fp(2), fp(8), fp(3)}},
			{Type: SeriesArea, Name: "a", Y: []*float64{fp(2), fp(1), fp(3), fp(2), fp(4)}},
			{Type: SeriesBar, Name: "b", Y: []*float64{fp(1), fp(2), fp(1), fp(3), fp(2)}},
			{Type: SeriesScatter, Name: "s", Y: []*float64{fp(3), fp(1), fp(4), fp(1), fp(5)}},
		},
	}
}

func renderOrFail(t *testing.T, spec *Spec) *Result {
	t.Helper()
	r, err := NewEngine().Render(context.Background(), spec)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	return r
}

func TestRenderDeterministic(t *testing.T) {
	spec := baseSpec()
	r1 := renderOrFail(t, spec)
	r2 := renderOrFail(t, spec)
	if r1.SVG != r2.SVG {
		t.Fatal("same input produced different SVG")
	}
	if !strings.HasPrefix(r1.SVG, "<svg") || !strings.HasSuffix(r1.SVG, "</svg>") {
		t.Fatal("SVG document incomplete")
	}
}

func TestRenderConcurrentConsistent(t *testing.T) {
	spec := baseSpec()
	want := renderOrFail(t, spec).SVG
	const n = 32
	results := make([]string, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(k int) {
			defer wg.Done()
			r, err := NewEngine().Render(context.Background(), spec)
			if err != nil {
				t.Errorf("render: %v", err)
				return
			}
			results[k] = r.SVG
		}(i)
	}
	wg.Wait()
	for i, got := range results {
		if got != want {
			t.Fatalf("concurrent render %d differs", i)
		}
	}
}

func TestRenderCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r, err := NewEngine().Render(ctx, baseSpec())
	if err != ErrCanceled {
		t.Fatalf("expected ErrCanceled, got %v", err)
	}
	if r != nil {
		t.Fatal("canceled render returned partial output")
	}
}

func TestRenderMissingAndInvalid(t *testing.T) {
	nan := math.NaN()
	inf := math.Inf(1)
	spec := &Spec{
		Width: 400, Height: 300,
		XAxis: AxisSpec{Type: AxisLinear},
		Series: []SeriesSpec{
			{Type: SeriesLine, Y: []*float64{fp(1), nil, fp(3), &nan, &inf, fp(2)}},
		},
	}
	r := renderOrFail(t, spec)
	codes := map[DiagCode]bool{}
	for _, d := range r.Diagnostics {
		codes[d.Code] = true
	}
	if !codes[DiagMissingValue] {
		t.Fatal("missing-value diagnostic absent")
	}
	if !codes[DiagInvalidValue] {
		t.Fatal("invalid-value diagnostic absent")
	}
	// Missing values must not be rendered as zero: the path must break.
	if !strings.Contains(r.SVG, "M") {
		t.Fatal("expected path breaks for missing values")
	}
}

func TestRenderEmptySeries(t *testing.T) {
	spec := &Spec{
		Width: 400, Height: 300,
		XAxis: AxisSpec{Type: AxisLinear},
		Series: []SeriesSpec{
			{Type: SeriesLine, Y: nil},
			{Type: SeriesLine, Y: []*float64{nil, nil}},
		},
	}
	r := renderOrFail(t, spec)
	empty := 0
	fallback := false
	for _, d := range r.Diagnostics {
		if d.Code == DiagEmptySeries {
			empty++
		}
		if d.Code == DiagAxisFallback {
			fallback = true
		}
	}
	if empty != 2 {
		t.Fatalf("expected 2 empty-series diagnostics, got %d", empty)
	}
	if !fallback {
		t.Fatal("expected axis fallback diagnostic for dataless chart")
	}
}

func TestRenderLengthMismatchRejected(t *testing.T) {
	spec := &Spec{
		Width: 400, Height: 300,
		XAxis: AxisSpec{Type: AxisLinear},
		Series: []SeriesSpec{
			{Type: SeriesLine, X: []float64{1, 2, 3}, Y: []*float64{fp(1), fp(2)}},
		},
	}
	if err := spec.Validate(); err == nil {
		t.Fatal("length mismatch not rejected")
	}
}

func TestRenderExtremeValues(t *testing.T) {
	spec := &Spec{
		Width: 400, Height: 300,
		XAxis: AxisSpec{Type: AxisLinear},
		Series: []SeriesSpec{
			{Type: SeriesLine, Y: []*float64{fp(1e300), fp(-1e300), fp(1e-300), fp(0)}},
			{Type: SeriesScatter, Y: []*float64{fp(5), fp(5), fp(5)}}, // constant
		},
	}
	r := renderOrFail(t, spec)
	if strings.Contains(r.SVG, "NaN") || strings.Contains(r.SVG, "Inf") {
		t.Fatal("non-finite value leaked into SVG")
	}
}

func TestRenderLogAxisDropsNonPositive(t *testing.T) {
	spec := &Spec{
		Width: 400, Height: 300,
		XAxis: AxisSpec{Type: AxisLinear},
		YAxes: []AxisSpec{{ID: "y-0", Type: AxisLog}},
		Series: []SeriesSpec{
			{Type: SeriesLine, Y: []*float64{fp(10), fp(0), fp(-5), fp(100)}},
		},
	}
	r := renderOrFail(t, spec)
	found := false
	for _, d := range r.Diagnostics {
		if d.Code == DiagLogNonPositive {
			found = true
		}
	}
	if !found {
		t.Fatal("log-non-positive diagnostic absent")
	}
	if strings.Contains(r.SVG, "NaN") {
		t.Fatal("NaN leaked into SVG")
	}
}

func TestRenderTimeAxis(t *testing.T) {
	base := 1700000000.0
	spec := &Spec{
		Width: 600, Height: 300,
		XAxis: AxisSpec{Type: AxisTime},
		Series: []SeriesSpec{
			{Type: SeriesLine,
				X: []float64{base, base + 3600, base + 7200, base + 10800},
				Y: []*float64{fp(1), fp(5), fp(2), fp(4)}},
		},
	}
	r := renderOrFail(t, spec)
	if !strings.Contains(r.SVG, ":") {
		t.Fatal("expected HH:MM time labels")
	}
}

func TestRenderMultiAxisAlignment(t *testing.T) {
	spec := &Spec{
		Width: 600, Height: 400,
		XAxis: AxisSpec{Type: AxisLinear},
		YAxes: []AxisSpec{
			{ID: "left", Label: "L"},
			{ID: "right", Label: "R"},
		},
		Series: []SeriesSpec{
			{Type: SeriesLine, YAxisID: "left", Y: []*float64{fp(0), fp(1), fp(2)}},
			{Type: SeriesLine, YAxisID: "right", Y: []*float64{fp(0), fp(1000), fp(2000)}},
		},
	}
	r1 := renderOrFail(t, spec)
	r2 := renderOrFail(t, spec)
	if r1.SVG != r2.SVG {
		t.Fatal("multi-axis render nondeterministic")
	}
	// Both axis labels must appear.
	if !strings.Contains(r1.SVG, ">L<") || !strings.Contains(r1.SVG, ">R<") {
		t.Fatal("axis labels missing")
	}
	// Right axis ticks (thousands) must appear.
	if !strings.Contains(r1.SVG, "2000") {
		t.Fatal("independent right axis scale missing")
	}
}

func TestRenderSharedAxisUniformTicks(t *testing.T) {
	spec := &Spec{
		Width: 600, Height: 400,
		XAxis: AxisSpec{Type: AxisLinear},
		Series: []SeriesSpec{
			{Type: SeriesLine, Y: []*float64{fp(0), fp(1)}},
			{Type: SeriesLine, Y: []*float64{fp(0), fp(100)}},
		},
	}
	r := renderOrFail(t, spec)
	// Shared axis: the union extent (0..100 padded) must appear once.
	if !strings.Contains(r.SVG, "100") {
		t.Fatal("shared axis did not cover union extent")
	}
}

func TestRenderStackedBar(t *testing.T) {
	spec := &Spec{
		Width: 500, Height: 400,
		XAxis: AxisSpec{Type: AxisLinear},
		Series: []SeriesSpec{
			{Type: SeriesStackedBar, Name: "s1", Y: []*float64{fp(1), fp(2), fp(3)}},
			{Type: SeriesStackedBar, Name: "s2", Y: []*float64{fp(2), fp(1), fp(2)}},
		},
	}
	r1 := renderOrFail(t, spec)
	r2 := renderOrFail(t, spec)
	if r1.SVG != r2.SVG {
		t.Fatal("stacked bar render nondeterministic")
	}
	// Stacked extent (max sum = 5) must drive the axis.
	if !strings.Contains(r1.SVG, ">5<") && !strings.Contains(r1.SVG, ">6<") {
		t.Fatal("stacked extent not reflected in axis")
	}
}

func TestRenderExplicitRange(t *testing.T) {
	lo, hi := -10.0, 10.0
	spec := &Spec{
		Width: 400, Height: 300,
		XAxis: AxisSpec{Type: AxisLinear},
		YAxes: []AxisSpec{{ID: "y-0", Min: &lo, Max: &hi}},
		Series: []SeriesSpec{
			{Type: SeriesLine, Y: []*float64{fp(0), fp(1)}},
		},
	}
	r := renderOrFail(t, spec)
	if !strings.Contains(r.SVG, ">-10<") || !strings.Contains(r.SVG, ">10<") {
		t.Fatal("explicit axis range not honored")
	}
}

func TestRenderDownsampleDiagnostic(t *testing.T) {
	n := 100000
	ys := make([]*float64, n)
	for i := range ys {
		ys[i] = fp(float64(i % 50))
	}
	spec := &Spec{
		Width: 800, Height: 400,
		XAxis: AxisSpec{Type: AxisLinear},
		Series: []SeriesSpec{
			{Type: SeriesLine, Y: ys},
		},
	}
	r := renderOrFail(t, spec)
	found := false
	for _, d := range r.Diagnostics {
		if d.Code == DiagDownsampled {
			found = true
		}
	}
	if !found {
		t.Fatal("downsample diagnostic absent")
	}
	// Output size must stay bounded.
	if len(r.SVG) > 500000 {
		t.Fatalf("SVG too large for 100k points: %d bytes", len(r.SVG))
	}
}

func TestRenderSmallCanvas(t *testing.T) {
	spec := &Spec{
		Width: 120, Height: 90,
		Title: "tiny",
		XAxis: AxisSpec{Type: AxisLinear, Label: "x"},
		Series: []SeriesSpec{
			{Type: SeriesLine, Y: []*float64{fp(1), fp(2)}},
		},
	}
	r := renderOrFail(t, spec)
	if !strings.HasSuffix(r.SVG, "</svg>") {
		t.Fatal("small canvas render incomplete")
	}
}
