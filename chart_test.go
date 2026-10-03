package chart

import (
	"context"
	"math"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/guoyyit/chart-render-engine-go/internal/downsample"
	"github.com/guoyyit/chart-render-engine-go/internal/scale"
)

// ---------- scale / ticks ----------

func TestLinearTicksAreNice(t *testing.T) {
	s := scale.NewLinear(0, 100, 0, 100)
	ticks := s.Ticks(5)
	if len(ticks) < 3 {
		t.Fatalf("expected >=3 ticks, got %d", len(ticks))
	}
	for _, tk := range ticks {
		if !tk.Major {
			t.Fatalf("linear ticks should be major")
		}
		// labels must be nice numbers (no long floats)
		if strings.ContainsAny(tk.Label, "eE") && !strings.Contains(tk.Label, "e+") {
			t.Fatalf("tick label not nice: %q", tk.Label)
		}
	}
	if ticks[0].Value > 0 || ticks[len(ticks)-1].Value < 100 {
		t.Fatalf("ticks do not cover domain: %v", ticks)
	}
	// step must be a nice number (1,2,5 * 10^k)
	for i := 1; i < len(ticks); i++ {
		d := ticks[i].Value - ticks[i-1].Value
		if !isNiceStep(d) {
			t.Fatalf("step %g not nice", d)
		}
	}
}

func TestLinearTicksNegative(t *testing.T) {
	s := scale.NewLinear(-7, 3, 0, 100)
	ticks := s.Ticks(5)
	if len(ticks) < 3 {
		t.Fatalf("got %d ticks", len(ticks))
	}
	if ticks[0].Value > -7 || ticks[len(ticks)-1].Value < 3 {
		t.Fatalf("ticks do not cover domain: %v", ticks)
	}
}

func TestLogTicks(t *testing.T) {
	s := scale.NewLog(10, 1, 1000, 0, 100)
	ticks := s.Ticks(5)
	var majors []float64
	for _, tk := range ticks {
		if tk.Major {
			majors = append(majors, tk.Value)
		}
	}
	want := []float64{1, 10, 100, 1000}
	if len(majors) != len(want) {
		t.Fatalf("majors = %v, want %v", majors, want)
	}
	for i := range want {
		if majors[i] != want[i] {
			t.Fatalf("majors = %v, want %v", majors, want)
		}
	}
	// log scale must reject non-positive values
	if s.IsValid(0) || s.IsValid(-1) || !s.IsValid(1) {
		t.Fatal("log IsValid wrong")
	}
}

func TestLogNarrowDomainTicks(t *testing.T) {
	s := scale.NewLog(10, 2, 8, 0, 100)
	ticks := s.Ticks(5)
	has2, has5 := false, false
	for _, tk := range ticks {
		if tk.Label == "2" {
			has2 = true
		}
		if tk.Label == "5" {
			has5 = true
		}
	}
	if !has2 || !has5 {
		t.Fatalf("narrow log domain should label 2 and 5: %v", ticks)
	}
}

func TestTimeTicks(t *testing.T) {
	// one day, 5 ticks -> hour step
	s := scale.NewTime(0, 86400, 0, 100)
	ticks := s.Ticks(5)
	if len(ticks) < 3 {
		t.Fatalf("got %d ticks", len(ticks))
	}
	for _, tk := range ticks {
		if _, err := time.Parse("15:04", tk.Label); err != nil {
			t.Fatalf("tick label %q not HH:MM", tk.Label)
		}
	}
	// ~2 months -> day ticks
	s2 := scale.NewTime(0, 60*86400, 0, 100)
	t2 := s2.Ticks(6)
	for _, tk := range t2 {
		if _, err := time.Parse("2006-01-02", tk.Label); err != nil {
			t.Fatalf("tick label %q not YYYY-MM-DD", tk.Label)
		}
	}
	// ~2 years -> month ticks
	s3 := scale.NewTime(0, 700*86400, 0, 100)
	t3 := s3.Ticks(6)
	for _, tk := range t3 {
		if _, err := time.Parse("2006-01", tk.Label); err != nil {
			t.Fatalf("tick label %q not YYYY-MM", tk.Label)
		}
	}
}

func TestNiceDomainDegenerate(t *testing.T) {
	lo, hi := scale.NiceDomain(5, 5)
	if lo >= 5 || hi <= 5 {
		t.Fatalf("constant domain not padded: %g..%g", lo, hi)
	}
	lo, hi = scale.NiceDomain(0, 0)
	if lo >= 0 || hi <= 0 {
		t.Fatalf("zero domain not padded: %g..%g", lo, hi)
	}
}

func isNiceStep(d float64) bool {
	if d <= 0 {
		return false
	}
	e := math.Pow(10, math.Floor(math.Log10(d)))
	f := d / e
	return math.Abs(f-1) < 1e-9 || math.Abs(f-2) < 1e-9 || math.Abs(f-5) < 1e-9 || math.Abs(f-10) < 1e-9
}

// ---------- downsampling ----------

func TestMinMaxPreservesExtremaAndEnds(t *testing.T) {
	pts := make([]downsample.Point, 1000)
	for i := range pts {
		pts[i] = downsample.Point{X: float64(i), Y: float64(i % 100)}
	}
	got := downsample.MinMax(pts, 100)
	if len(got) > 100 {
		t.Fatalf("got %d points, want <= 100", len(got))
	}
	if got[0] != pts[0] || got[len(got)-1] != pts[len(pts)-1] {
		t.Fatal("first/last points not preserved")
	}
	// global min and max must be present
	var minY, maxY float64 = math.Inf(1), math.Inf(-1)
	for _, p := range pts {
		if p.Y < minY {
			minY = p.Y
		}
		if p.Y > maxY {
			maxY = p.Y
		}
	}
	hasMin, hasMax := false, false
	for _, p := range got {
		if p.Y == minY {
			hasMin = true
		}
		if p.Y == maxY {
			hasMax = true
		}
	}
	if !hasMin || !hasMax {
		t.Fatalf("extrema not preserved: min=%v max=%v", hasMin, hasMax)
	}
}

func TestMinMaxDeterministic(t *testing.T) {
	pts := make([]downsample.Point, 5000)
	for i := range pts {
		pts[i] = downsample.Point{X: float64(i), Y: math.Sin(float64(i) / 10)}
	}
	a := downsample.MinMax(pts, 200)
	b := downsample.MinMax(pts, 200)
	if len(a) != len(b) {
		t.Fatalf("lengths differ: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("result differs at %d", i)
		}
	}
}

func TestMinMaxKeepsMissingGaps(t *testing.T) {
	pts := []downsample.Point{
		{X: 0, Y: 1}, {X: 1, Y: 2}, {X: 2, Y: math.NaN()},
		{X: 3, Y: 4}, {X: 4, Y: 5},
	}
	got := downsample.MinMax(pts, 10)
	if len(got) != 5 {
		t.Fatalf("gap marker lost: %v", got)
	}
	if !math.IsNaN(got[2].Y) {
		t.Fatal("missing value marker not preserved")
	}
}

func TestBarsSumAndDeterminism(t *testing.T) {
	pts := make([]downsample.Point, 1000)
	for i := range pts {
		pts[i] = downsample.Point{X: float64(i), Y: 1}
	}
	b1 := downsample.Bars(pts, 0, 999, 10)
	b2 := downsample.Bars(pts, 0, 999, 10)
	if len(b1) != 10 {
		t.Fatalf("got %d buckets", len(b1))
	}
	for i := range b1 {
		if b1[i].Count != 100 {
			t.Fatalf("bucket %d count %d, want 100", i, b1[i].Count)
		}
		if b1[i].Value != 100 {
			t.Fatalf("bucket %d value %v, want 100", i, b1[i].Value)
		}
		if b1[i] != b2[i] {
			t.Fatal("bars not deterministic")
		}
	}
}

// ---------- render determinism ----------

func TestRenderDeterministic(t *testing.T) {
	spec := Spec{
		Width: 640, Height: 400, Title: "det",
		Series: []Series{
			{Name: "a", Type: SeriesLine, Y: []float64{1, 3, 2, 5, 4}},
			{Name: "b", Type: SeriesScatter, Y: []float64{2, 1, 4, 3, 6}},
		},
	}
	r1, err := Render(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := Render(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if r1.SVG != r2.SVG {
		t.Fatal("identical input produced different SVG")
	}
}

func TestConcurrentRenderIdentical(t *testing.T) {
	spec := Spec{
		Width: 800, Height: 500,
		Series: []Series{
			{Name: "line", Type: SeriesLine, Y: []float64{1, 3, 2, 5, 4, 7, 6}},
			{Name: "bar", Type: SeriesBar, Y: []float64{2, 4, 3, 6, 5, 8, 7}},
		},
	}
	const n = 20
	results := make([]string, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r, err := Render(context.Background(), spec)
			if err != nil {
				errs[i] = err
				return
			}
			results[i] = r.SVG
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("render %d failed: %v", i, err)
		}
	}
	for i := 1; i < n; i++ {
		if results[i] != results[0] {
			t.Fatalf("concurrent render %d differs", i)
		}
	}
}

func TestCancellation(t *testing.T) {
	spec := Spec{Width: 100, Height: 100, Series: []Series{{Type: SeriesLine, Y: []float64{1, 2, 3}}}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r, err := Render(ctx, spec)
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	if r != nil {
		t.Fatal("canceled render must return no result")
	}
}

// ---------- data handling ----------

func TestMissingValuesNotZeroed(t *testing.T) {
	spec := Spec{
		Width: 400, Height: 300,
		Series: []Series{{Name: "a", Type: SeriesLine, Y: []float64{10, math.NaN(), 10.5, math.NaN(), 11}}},
	}
	r, err := Render(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	var hasMissing bool
	for _, d := range r.Diagnostics {
		if d.Code == "MISSING_VALUES" {
			hasMissing = true
		}
	}
	if !hasMissing {
		t.Fatal("missing values not diagnosed")
	}
	// y domain must be ~[10,11], not include 0: check y-axis labels (anchor end)
	for _, m := range regexp.MustCompile(`text-anchor="end"[^>]*>([^<]+)</text>`).FindAllStringSubmatch(r.SVG, -1) {
		if m[1] == "0" {
			t.Fatal("missing values were treated as zero (0 y tick present)")
		}
	}
}

func TestInvalidValuesDiagnosed(t *testing.T) {
	spec := Spec{
		Width: 400, Height: 300,
		Series: []Series{{Type: SeriesScatter, Y: []float64{1, math.Inf(1), 3, math.Inf(-1), 5}}},
	}
	r, err := Render(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	codes := map[string]bool{}
	for _, d := range r.Diagnostics {
		codes[d.Code] = true
	}
	if !codes["INVALID_VALUES"] {
		t.Fatalf("INVALID_VALUES missing: %v", r.Diagnostics)
	}
}

func TestEmptySeries(t *testing.T) {
	spec := Spec{
		Width: 400, Height: 300,
		Series: []Series{
			{Name: "empty", Type: SeriesLine, Y: []float64{}},
			{Name: "ok", Type: SeriesLine, Y: []float64{1, 2, 3}},
		},
	}
	r, err := Render(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, d := range r.Diagnostics {
		if d.Code == "EMPTY_SERIES" {
			found = true
		}
	}
	if !found {
		t.Fatal("EMPTY_SERIES not diagnosed")
	}
	if !strings.Contains(r.SVG, "<svg") {
		t.Fatal("no svg produced")
	}
}

func TestLengthMismatch(t *testing.T) {
	spec := Spec{
		Width: 400, Height: 300,
		Series: []Series{{Type: SeriesLine, X: []float64{1, 2}, Y: []float64{1, 2, 3}}},
	}
	r, err := Render(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, d := range r.Diagnostics {
		if d.Code == "LENGTH_MISMATCH" {
			found = true
		}
	}
	if !found {
		t.Fatal("LENGTH_MISMATCH not diagnosed")
	}
}

func TestAllEqualData(t *testing.T) {
	spec := Spec{
		Width: 400, Height: 300,
		Series: []Series{{Type: SeriesLine, Y: []float64{42, 42, 42, 42}}},
	}
	r, err := Render(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if r.SVG == "" {
		t.Fatal("empty svg")
	}
	// repeated render identical
	r2, _ := Render(context.Background(), spec)
	if r.SVG != r2.SVG {
		t.Fatal("constant data render not stable")
	}
}

func TestLogAxis(t *testing.T) {
	spec := Spec{
		Width: 400, Height: 300,
		Axes: []AxisSpec{
			{ID: "x", Kind: AxisLinear, Position: AxisBottom},
			{ID: "y", Kind: AxisLog, Position: AxisLeft},
		},
		Series: []Series{{Type: SeriesLine, Y: []float64{1, 10, 100, 1000}}},
	}
	r, err := Render(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{"10", "100", "1000"} {
		if !strings.Contains(r.SVG, ">"+label+"<") {
			t.Fatalf("log tick %q missing", label)
		}
	}
}

func TestLogAxisNonPositive(t *testing.T) {
	spec := Spec{
		Width: 400, Height: 300,
		Axes:   []AxisSpec{{ID: "y", Kind: AxisLog, Position: AxisLeft}},
		Series: []Series{{Type: SeriesLine, Y: []float64{1, -2, 0, 10}}},
	}
	r, err := Render(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, d := range r.Diagnostics {
		if d.Code == "LOG_NON_POSITIVE" {
			found = true
		}
	}
	if !found {
		t.Fatal("LOG_NON_POSITIVE not diagnosed")
	}
}

func TestSharedAndIndependentAxes(t *testing.T) {
	spec := Spec{
		Width: 600, Height: 400,
		Axes: []AxisSpec{
			{ID: "x", Kind: AxisLinear, Position: AxisBottom},
			{ID: "y", Kind: AxisLinear, Position: AxisLeft},
			{ID: "y2", Kind: AxisLinear, Position: AxisRight},
		},
		Series: []Series{
			{Name: "a", Type: SeriesLine, Y: []float64{1, 2, 3}, YAxis: "y"},
			{Name: "b", Type: SeriesLine, Y: []float64{100, 200, 300}, YAxis: "y2"},
		},
	}
	r, err := Render(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	// both y axes labels present
	if !strings.Contains(r.SVG, ">300<") || !strings.Contains(r.SVG, ">3<") {
		t.Fatal("independent axis labels missing")
	}
}

func TestExplicitAxisBounds(t *testing.T) {
	lo, hi := 0.0, 10.0
	spec := Spec{
		Width: 400, Height: 300,
		Axes:   []AxisSpec{{ID: "y", Kind: AxisLinear, Position: AxisLeft, Min: &lo, Max: &hi}},
		Series: []Series{{Type: SeriesLine, Y: []float64{1, 2}}},
	}
	r, err := Render(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.SVG, ">10<") {
		t.Fatal("explicit max tick missing")
	}
}

func TestDownsamplingTriggered(t *testing.T) {
	big := make([]float64, 20000)
	for i := range big {
		big[i] = math.Sin(float64(i) / 100)
	}
	spec := Spec{
		Width: 800, Height: 400,
		Series: []Series{{Name: "big", Type: SeriesLine, Y: big}},
	}
	r, err := Render(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, d := range r.Diagnostics {
		if d.Code == "DOWNSAMPLED" {
			found = true
		}
	}
	if !found {
		t.Fatal("DOWNSAMPLED not triggered for large series")
	}
}

func TestBarAggregationTriggered(t *testing.T) {
	big := make([]float64, 20000)
	for i := range big {
		big[i] = 1
	}
	spec := Spec{
		Width: 800, Height: 400,
		Series: []Series{{Name: "big", Type: SeriesBar, Y: big}},
	}
	r, err := Render(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, d := range r.Diagnostics {
		if d.Code == "AGGREGATED" {
			found = true
		}
	}
	if !found {
		t.Fatal("AGGREGATED not triggered for large bar series")
	}
}

func TestAllChartTypes(t *testing.T) {
	types := []SeriesType{SeriesLine, SeriesArea, SeriesBar, SeriesStackedBar, SeriesScatter}
	for _, ty := range types {
		spec := Spec{
			Width: 400, Height: 300,
			Series: []Series{
				{Name: "a", Type: ty, StackGroup: "g", Y: []float64{1, 2, 3}},
				{Name: "b", Type: ty, StackGroup: "g", Y: []float64{3, 2, 1}},
			},
		}
		r, err := Render(context.Background(), spec)
		if err != nil {
			t.Fatalf("type %v: %v", ty, err)
		}
		if !strings.Contains(r.SVG, "<svg") || !strings.Contains(r.SVG, "</svg>") {
			t.Fatalf("type %v: malformed svg", ty)
		}
	}
}

func TestTimeAxisRender(t *testing.T) {
	// unix seconds for 2024-01-01 .. 2024-01-03
	x0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).Unix()
	xs := []float64{float64(x0), float64(x0 + 60*86400), float64(x0 + 120*86400)}
	spec := Spec{
		Width: 600, Height: 300,
		Axes:   []AxisSpec{{ID: "x", Kind: AxisTime, Position: AxisBottom}},
		Series: []Series{{Type: SeriesLine, X: xs, Y: []float64{1, 2, 3}}},
	}
	r, err := Render(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.SVG, "2024-01") {
		t.Fatal("time tick label missing")
	}
}

func TestInvalidDimensions(t *testing.T) {
	spec := Spec{Width: 0, Height: 100}
	r, err := Render(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if r == nil || len(r.Diagnostics) == 0 {
		t.Fatal("expected diagnostics for invalid dimensions")
	}
}

func TestUnknownAxis(t *testing.T) {
	spec := Spec{
		Width: 400, Height: 300,
		Series: []Series{{Type: SeriesLine, YAxis: "nope", Y: []float64{1, 2}}},
	}
	r, err := Render(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, d := range r.Diagnostics {
		if d.Code == "AXIS_NOT_FOUND" {
			found = true
		}
	}
	if !found {
		t.Fatal("AXIS_NOT_FOUND not diagnosed")
	}
}

func TestByteIdenticalAcrossRuns(t *testing.T) {
	spec := Spec{
		Width: 500, Height: 400,
		Title: "snapshot",
		Series: []Series{
			{Name: "a", Type: SeriesArea, Y: []float64{1, 4, 2, 5, 3}},
			{Name: "b", Type: SeriesLine, Y: []float64{2, 3, 1, 4, 2}},
		},
	}
	var first string
	for i := 0; i < 5; i++ {
		r, err := Render(context.Background(), spec)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = r.SVG
		} else if r.SVG != first {
			t.Fatalf("run %d produced different output", i)
		}
	}
}

// ---------- more edge cases ----------

func TestTickLabelsNoFloatDrift(t *testing.T) {
	s := scale.NewLinear(0, 1, 0, 100)
	ticks := s.Ticks(6)
	for _, tk := range ticks {
		if len(tk.Label) > 12 {
			t.Fatalf("tick label too long (float drift?): %q", tk.Label)
		}
	}
}

func TestEmptySpec(t *testing.T) {
	r, err := Render(context.Background(), Spec{Width: 400, Height: 300})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.SVG, "<svg") {
		t.Fatal("no svg for empty spec")
	}
	var found bool
	for _, d := range r.Diagnostics {
		if d.Code == "NO_SERIES" {
			found = true
		}
	}
	if !found {
		t.Fatal("NO_SERIES not diagnosed")
	}
}

func TestLegendHidden(t *testing.T) {
	spec := Spec{
		Width: 400, Height: 300,
		Legend: LegendSpec{Hidden: true},
		Series: []Series{{Name: "a", Type: SeriesLine, Y: []float64{1, 2, 3}}},
	}
	r, err := Render(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(r.SVG, ">a<") {
		t.Fatal("legend should be hidden")
	}
}

func TestSmallCanvas(t *testing.T) {
	spec := Spec{
		Width: 50, Height: 40,
		Title:  "t",
		Series: []Series{{Name: "a", Type: SeriesLine, Y: []float64{1, 2, 3}}},
	}
	r, err := Render(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.SVG, "<svg") {
		t.Fatal("no svg for small canvas")
	}
}

func TestTopAndRightAxes(t *testing.T) {
	spec := Spec{
		Width: 500, Height: 400,
		Axes: []AxisSpec{
			{ID: "x", Kind: AxisLinear, Position: AxisTop},
			{ID: "y", Kind: AxisLinear, Position: AxisRight},
		},
		Series: []Series{{Type: SeriesLine, Y: []float64{1, 2, 3}}},
	}
	r, err := Render(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.SVG, "<svg") {
		t.Fatal("no svg")
	}
}

func TestMissingXValues(t *testing.T) {
	spec := Spec{
		Width: 400, Height: 300,
		Series: []Series{{
			Type: SeriesLine,
			X:    []float64{0, 1, math.NaN(), 3, math.Inf(1)},
			Y:    []float64{1, 2, 3, 4, 5},
		}},
	}
	r, err := Render(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, d := range r.Diagnostics {
		if d.Code == "INVALID_VALUES" {
			found = true
		}
	}
	if !found {
		t.Fatal("invalid x values not diagnosed")
	}
}

func TestLogExplicitBounds(t *testing.T) {
	lo, hi := 1.0, 1000.0
	spec := Spec{
		Width: 400, Height: 300,
		Axes:   []AxisSpec{{ID: "y", Kind: AxisLog, Position: AxisLeft, Min: &lo, Max: &hi}},
		Series: []Series{{Type: SeriesLine, Y: []float64{2, 50}}},
	}
	r, err := Render(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{"1", "10", "100", "1000"} {
		if !strings.Contains(r.SVG, ">"+label+"<") {
			t.Fatalf("log tick %q missing", label)
		}
	}
}

func TestTimeAxisEmpty(t *testing.T) {
	spec := Spec{
		Width: 400, Height: 300,
		Axes:   []AxisSpec{{ID: "x", Kind: AxisTime, Position: AxisBottom}},
		Series: []Series{{Type: SeriesLine, Y: []float64{}}},
	}
	r, err := Render(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.SVG, "<svg") {
		t.Fatal("no svg")
	}
}

func TestSharedAxisUnifiesScale(t *testing.T) {
	// Two series sharing the same y axis must align: a given y value maps
	// to the same pixel in both paths.
	spec := Spec{
		Width: 500, Height: 400,
		Series: []Series{
			{Name: "a", Type: SeriesLine, Y: []float64{0, 5, 10}},
			{Name: "b", Type: SeriesLine, Y: []float64{10, 5, 0}},
		},
	}
	r, err := Render(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	// Both series share axis "y" -> only one y axis (labels 0..10 once)
	count := strings.Count(r.SVG, ">10<")
	if count != 1 {
		t.Fatalf("shared axis should render labels once, got %d occurrences of 10", count)
	}
}

func TestNegativeStackedBars(t *testing.T) {
	spec := Spec{
		Width: 400, Height: 300,
		Series: []Series{
			{Name: "a", Type: SeriesStackedBar, StackGroup: "g", Y: []float64{1, -2, 3}},
			{Name: "b", Type: SeriesStackedBar, StackGroup: "g", Y: []float64{1, 2, -3}},
		},
	}
	r, err := Render(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.SVG, "<svg") {
		t.Fatal("no svg")
	}
}

func TestTransparentBackground(t *testing.T) {
	spec := Spec{
		Width: 400, Height: 300,
		Background: "transparent",
		Series:     []Series{{Type: SeriesLine, Y: []float64{1, 2}}},
	}
	r, err := Render(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(r.SVG, "<rect") {
		t.Fatal("transparent background should not draw a rect")
	}
}

func TestDiagnosticsDistinguishCauses(t *testing.T) {
	spec := Spec{
		Width: 400, Height: 300,
		Series: []Series{{
			Type: SeriesLine,
			Y:    []float64{1, math.NaN(), math.Inf(1), 3},
		}},
	}
	r, err := Render(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	codes := map[string]bool{}
	for _, d := range r.Diagnostics {
		codes[d.Code] = true
	}
	if !codes["MISSING_VALUES"] {
		t.Fatal("missing (NaN) not distinguished from invalid")
	}
	if !codes["INVALID_VALUES"] {
		t.Fatal("invalid (Inf) not distinguished from missing")
	}
}
