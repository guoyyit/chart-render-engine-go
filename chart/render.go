package chart

import (
	"context"
	"fmt"
	"sort"

	"github.com/guoyyit/chart-render-engine-go/svg"
)

// defaultPalette is the built-in deterministic color cycle.
var defaultPalette = []string{
	"#3366cc", "#dc3912", "#ff9900", "#109618", "#990099",
	"#0099c6", "#dd4477", "#66aa00", "#b82e2e", "#316395",
}

// axisModel is a fully resolved axis ready for drawing.
type axisModel struct {
	spec   AxisSpec
	scale  Scale
	ticks  []Tick
	keep   []bool // label thinning mask, parallel to ticks
	lo, hi float64
}

// render executes the deterministic rendering pipeline. Each phase
// checks ctx so cancellation never yields partial output.
func (e *Engine) render(ctx context.Context, spec *Spec) (*Result, error) {
	check := func() error {
		if ctx.Err() != nil {
			return ErrCanceled
		}
		return nil
	}
	if err := check(); err != nil {
		return nil, err
	}

	// Phase 1: clean data, collect diagnostics.
	dc := newDiagCounter()
	yAxisOf := resolveYAxes(spec)
	cleaned := make([]cleanSeries, len(spec.Series))
	for i, s := range spec.Series {
		at := AxisLinear
		if len(yAxisOf) > 0 {
			at = spec.YAxes[yAxisOf[i]].Type
			if at == "" {
				at = AxisLinear
			}
		}
		cleaned[i] = clean(s, i, at, dc)
		if i%16 == 0 {
			if err := check(); err != nil {
				return nil, err
			}
		}
	}
	diags := dc.list()
	for i := range cleaned {
		if cleaned[i].validCount() == 0 {
			diags = append(diags, Diagnostic{
				Code:     DiagEmptySeries,
				SeriesID: seriesID(spec.Series[i], i),
				Message:  "series has no usable points",
			})
		}
	}
	assignColors(spec, cleaned)

	// Phase 2: extents and ranges.
	if err := check(); err != nil {
		return nil, err
	}
	xMin, xMax, xHas := xExtent(cleaned)
	xSpec := spec.XAxis
	if xSpec.Type == "" {
		xSpec.Type = AxisLinear
	}
	xLo, xHi, xFb := deriveRange(xSpec, xMin, xMax, xHas)
	if xFb {
		diags = append(diags, Diagnostic{Code: DiagAxisFallback, Message: "x axis range fell back to default"})
	}
	yModels := make([]axisModel, len(yAxisOfAxis(spec)))
	for ai := range yModels {
		yspec := spec.YAxes[ai]
		if yspec.Type == "" {
			yspec.Type = AxisLinear
		}
		yspec.ID = axisID(yspec, ai)
		lo, hi, has := yExtent(cleaned, yAxisOf, ai, yspec.Type)
		rLo, rHi, fb := deriveRange(yspec, lo, hi, has)
		if fb {
			diags = append(diags, Diagnostic{Code: DiagAxisFallback, SeriesID: yspec.ID,
				Message: "y axis range fell back to default"})
		}
		yModels[ai] = axisModel{spec: yspec, lo: rLo, hi: rHi}
	}

	// Phase 3: ticks (labels needed for layout measurement).
	if err := check(); err != nil {
		return nil, err
	}
	xTicks := makeTicks(xSpec, xLo, xHi)
	xLabels := make([]string, len(xTicks))
	for i, t := range xTicks {
		xLabels[i] = t.Label
	}
	leftSides, rightSides := []axisSide{}, []axisSide{}
	yTicksPerAxis := make([][]Tick, len(yModels))
	for ai := range yModels {
		ticks := makeTicks(yModels[ai].spec, yModels[ai].lo, yModels[ai].hi)
		yTicksPerAxis[ai] = ticks
		labels := make([]string, len(ticks))
		for i, t := range ticks {
			labels[i] = t.Label
		}
		side := axisSide{label: yModels[ai].spec.Label, tickLabels: labels}
		if ai%2 == 0 {
			leftSides = append(leftSides, side)
		} else {
			rightSides = append(rightSides, side)
		}
	}
	legendEntries := legendNames(spec)

	// Phase 4: layout.
	lo := computeLayout(spec, xLabels, leftSides, rightSides, legendEntries)

	// Phase 5: scales and label thinning.
	xScale := newScale(xSpec.Type, xLo, xHi, lo.PlotX, lo.PlotX+lo.PlotW)
	xModel := axisModel{spec: xSpec, scale: xScale, ticks: xTicks, lo: xLo, hi: xHi}
	xModel.keep = thinLabels(xLabels, lo.PlotW)
	for ai := range yModels {
		m := &yModels[ai]
		m.scale = newScale(m.spec.Type, m.lo, m.hi, lo.PlotY+lo.PlotH, lo.PlotY)
		m.ticks = yTicksPerAxis[ai]
		m.keep = thinByCount(len(m.ticks), int(lo.PlotH/tickLabelH))
	}

	// Phase 6: draw.
	if err := check(); err != nil {
		return nil, err
	}
	b := svg.New(spec.Width, spec.Height)
	drawFrame(b, spec, &lo)
	drawXAxis(b, &lo, &xModel)
	for ai := range yModels {
		drawYAxis(b, &lo, &yModels[ai], ai, len(yModels), ai == 0)
	}
	// Clip plot area for series glyphs.
	b.Open("clipPath", svg.A("id", "plot-clip"))
	b.Empty("rect",
		svg.A("x", svg.Num(lo.PlotX)), svg.A("y", svg.Num(lo.PlotY)),
		svg.A("width", svg.Num(lo.PlotW)), svg.A("height", svg.Num(lo.PlotH)))
	b.Close("clipPath")
	b.Open("g", svg.A("clip-path", "url(#plot-clip)"))
	sp := &seriesPainter{b: b, lo: &lo, x: &xModel}
	if err := sp.drawStackedBars(ctx, cleaned, yAxisOf, yModels); err != nil {
		return nil, err
	}
	for i := range cleaned {
		if err := check(); err != nil {
			return nil, err
		}
		cs := &cleaned[i]
		if cs.spec.Type == SeriesStackedBar {
			continue // already drawn as a group
		}
		ym := &yModels[yAxisOf[i]]
		pts := downsample(cs.xs, cs.ys, spec.MaxPoints)
		if len(cs.ys) > len(pts) && cs.validCount() > len(pts) {
			diags = append(diags, Diagnostic{
				Code:     DiagDownsampled,
				SeriesID: seriesID(cs.spec, i),
				Message:  fmt.Sprintf("downsampled from %d to %d points", cs.validCount(), len(pts)),
			})
		}
		sp.drawSeries(cs, ym, pts)
	}
	b.Close("g")
	drawLegend(b, spec, &lo, cleaned)

	return &Result{SVG: b.String(), Diagnostics: diags}, nil
}

// seriesID returns the effective series id.
func seriesID(s SeriesSpec, i int) string {
	if s.ID != "" {
		return s.ID
	}
	return fmt.Sprintf("series-%d", i)
}

func axisID(a AxisSpec, i int) string {
	if a.ID != "" {
		return a.ID
	}
	return fmt.Sprintf("y-%d", i)
}

// resolveYAxes maps each series to a y axis index, creating implicit
// axes as needed. It also appends missing axis specs to spec.YAxes.
func resolveYAxes(spec *Spec) []int {
	if len(spec.YAxes) == 0 {
		spec.YAxes = []AxisSpec{{ID: "y-0", Type: AxisLinear}}
	}
	index := make(map[string]int, len(spec.YAxes))
	for i, a := range spec.YAxes {
		id := a.ID
		if id == "" {
			id = fmt.Sprintf("y-%d", i)
		}
		index[id] = i
	}
	out := make([]int, len(spec.Series))
	for i, s := range spec.Series {
		if s.YAxisID == "" {
			out[i] = 0
			continue
		}
		ai, ok := index[s.YAxisID]
		if !ok {
			// Unknown axis id: create it so the reference is explicit
			// and reproducible.
			spec.YAxes = append(spec.YAxes, AxisSpec{ID: s.YAxisID, Type: AxisLinear})
			ai = len(spec.YAxes) - 1
			index[s.YAxisID] = ai
		}
		out[i] = ai
	}
	return out
}

func yAxisOfAxis(spec *Spec) []AxisSpec { return spec.YAxes }

// assignColors resolves each series color deterministically.
func assignColors(spec *Spec, cleaned []cleanSeries) {
	palette := spec.Palette
	if len(palette) == 0 {
		palette = defaultPalette
	}
	for i := range cleaned {
		if cleaned[i].spec.Color != "" {
			cleaned[i].color = cleaned[i].spec.Color
		} else {
			cleaned[i].color = palette[i%len(palette)]
		}
	}
}

// xExtent computes the shared x extent over all valid points.
func xExtent(cleaned []cleanSeries) (min, max float64, has bool) {
	for i := range cleaned {
		c := &cleaned[i]
		for j, y := range c.ys {
			if y == nil {
				continue
			}
			x := c.xs[j]
			if !has {
				min, max, has = x, x, true
			} else {
				if x < min {
					min = x
				}
				if x > max {
					max = x
				}
			}
		}
	}
	return min, max, has
}

// yExtent computes the extent of one y axis over its series. Stacked
// bars contribute cumulative sums; everything else raw values.
func yExtent(cleaned []cleanSeries, yAxisOf []int, axis int, t AxisType) (min, max float64, has bool) {
	acc := map[float64]float64{}
	for i := range cleaned {
		c := &cleaned[i]
		if yAxisOf[i] != axis {
			continue
		}
		if c.spec.Type == SeriesStackedBar {
			for j, y := range c.ys {
				if y == nil {
					continue
				}
				acc[c.xs[j]] += *y
			}
			continue
		}
		for _, y := range c.ys {
			if y == nil {
				continue
			}
			v := *y
			if !has {
				min, max, has = v, v, true
			} else {
				if v < min {
					min = v
				}
				if v > max {
					max = v
				}
			}
		}
	}
	if len(acc) > 0 {
		keys := make([]float64, 0, len(acc))
		for k := range acc {
			keys = append(keys, k)
		}
		sort.Float64s(keys)
		for _, k := range keys {
			v := acc[k]
			if !has {
				min, max, has = v, v, true
			} else {
				if v < min {
					min = v
				}
				if v > max {
					max = v
				}
			}
		}
		// Stacked bars always include the zero baseline.
		if min > 0 {
			min = 0
		}
	}
	return min, max, has
}

// makeTicks dispatches on axis type.
func makeTicks(spec AxisSpec, lo, hi float64) []Tick {
	switch spec.Type {
	case AxisLog:
		return LogTicks(lo, hi, spec.Ticks)
	case AxisTime:
		return TimeTicks(lo, hi, spec.Ticks)
	default:
		return LinearTicks(lo, hi, spec.Ticks)
	}
}

// legendNames lists entry names in series order (deterministic).
func legendNames(spec *Spec) []string {
	if !spec.Legend.Visible {
		return nil
	}
	out := make([]string, 0, len(spec.Series))
	for i, s := range spec.Series {
		name := s.Name
		if name == "" {
			name = seriesID(s, i)
		}
		out = append(out, name)
	}
	return out
}

// thinByCount keeps every k-th of n items so at most maxKeep remain.
func thinByCount(n, maxKeep int) []bool {
	keep := make([]bool, n)
	if maxKeep < 1 {
		maxKeep = 1
	}
	k := 1
	for k < n && (n+k-1)/k > maxKeep {
		k++
	}
	for i := range keep {
		keep[i] = i%k == 0
	}
	return keep
}
