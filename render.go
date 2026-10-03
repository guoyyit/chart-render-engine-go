package chart

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/guoyyit/chart-render-engine-go/internal/downsample"
	"github.com/guoyyit/chart-render-engine-go/internal/scale"
	"github.com/guoyyit/chart-render-engine-go/internal/svgb"
)

// default palette (Tableau 10)
var palette = []string{
	"#4E79A7", "#F28E2B", "#E15759", "#76B7B2", "#59A14F",
	"#EDC948", "#B07AA1", "#FF9DA7", "#9C755F", "#BAB0AC",
}

const (
	fontTitle  = 16.0
	fontAxis   = 12.0
	fontTick   = 11.0
	fontLegend = 12.0
	pad        = 12.0
)

type axisRT struct {
	spec scaleAxisSpec
	kind scale.Kind
	pos  AxisPosition
	lo   float64
	hi   float64
	sc   scale.Scale
}

// scaleAxisSpec is the axis data the scale layer needs.
type scaleAxisSpec struct {
	id    string
	pos   AxisPosition
	min   *float64
	max   *float64
	base  float64
	title string
}

type seriesRT struct {
	spec    Series
	pts     []downsample.Point
	color   string
	opacity float64
	xAxis   *axisRT
	yAxis   *axisRT
	valid   bool
}

func fnum(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

func ctxErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

// renderChart is the internal entry point used by Render.
func renderChart(ctx context.Context, spec Spec) (*Result, error) {
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	diags := []Diagnostic{}
	addDiag := func(level DiagnosticLevel, code, msg string, series int) {
		diags = append(diags, Diagnostic{Level: level, Code: code, Message: msg, Series: series})
	}

	if spec.Width <= 0 || spec.Height <= 0 {
		addDiag(DiagError, "INVALID_DIMENSIONS", fmt.Sprintf("canvas dimensions must be positive, got %gx%g", spec.Width, spec.Height), -1)
		return &Result{Diagnostics: diags}, nil
	}

	// ---- axes ----
	axes := map[string]*axisRT{}
	axisOrder := []string{}
	ensureAxis := func(id string, kind AxisKind, pos AxisPosition) *axisRT {
		if a, ok := axes[id]; ok {
			return a
		}
		a := &axisRT{spec: scaleAxisSpec{id: id, pos: pos}, kind: toScaleKind(kind), pos: pos}
		axes[id] = a
		axisOrder = append(axisOrder, id)
		return a
	}
	for _, as := range spec.Axes {
		if as.ID == "" {
			addDiag(DiagError, "INVALID_AXIS", "axis with empty id ignored", -1)
			continue
		}
		if _, dup := axes[as.ID]; dup {
			addDiag(DiagError, "DUPLICATE_AXIS", fmt.Sprintf("duplicate axis id %q ignored", as.ID), -1)
			continue
		}
		base := as.Base
		if as.Kind == AxisLog && (base <= 0 || base == 1) {
			addDiag(DiagWarning, "LOG_BASE_INVALID", fmt.Sprintf("log axis %q has invalid base %g, using 10", as.ID, base), -1)
			base = 10
		}
		a := &axisRT{
			spec: scaleAxisSpec{id: as.ID, pos: as.Position, min: as.Min, max: as.Max, base: base, title: as.Title},
			kind: toScaleKind(as.Kind),
			pos:  as.Position,
		}
		axes[as.ID] = a
		axisOrder = append(axisOrder, as.ID)
	}
	// default axes
	ensureAxis("x", AxisLinear, AxisBottom)
	ensureAxis("y", AxisLinear, AxisLeft)

	// ---- series normalization & validation ----
	if len(spec.Series) == 0 {
		addDiag(DiagWarning, "NO_SERIES", "spec has no series", -1)
	}
	series := make([]seriesRT, 0, len(spec.Series))
	for si, s := range spec.Series {
		if err := ctxErr(ctx); err != nil {
			return nil, err
		}
		color := s.Color
		if color == "" {
			color = palette[si%len(palette)]
		}
		opacity := s.Opacity
		if opacity == 0 {
			opacity = 1
		}
		if opacity < 0 || opacity > 1 {
			addDiag(DiagWarning, "INVALID_OPACITY", fmt.Sprintf("series %d opacity %g out of [0,1], clamped", si, opacity), si)
			if opacity < 0 {
				opacity = 0
			} else {
				opacity = 1
			}
		}
		xID, yID := s.XAxis, s.YAxis
		if xID == "" {
			xID = "x"
		}
		if yID == "" {
			yID = "y"
		}
		xa, okx := axes[xID]
		ya, oky := axes[yID]
		if !okx || !oky {
			missing := xID
			if !okx {
				missing = xID
			} else {
				missing = yID
			}
			addDiag(DiagError, "AXIS_NOT_FOUND", fmt.Sprintf("series %d references unknown axis %q, skipped", si, missing), si)
			series = append(series, seriesRT{spec: s, color: color, opacity: opacity, valid: false})
			continue
		}
		if len(s.Y) == 0 {
			addDiag(DiagWarning, "EMPTY_SERIES", fmt.Sprintf("series %d %q has no data, skipped", si, s.Name), si)
			series = append(series, seriesRT{spec: s, color: color, opacity: opacity, xAxis: xa, yAxis: ya, valid: false})
			continue
		}
		if s.X != nil && len(s.X) != len(s.Y) {
			addDiag(DiagError, "LENGTH_MISMATCH", fmt.Sprintf("series %d has %d x values but %d y values, skipped", si, len(s.X), len(s.Y)), si)
			series = append(series, seriesRT{spec: s, color: color, opacity: opacity, xAxis: xa, yAxis: ya, valid: false})
			continue
		}
		pts := make([]downsample.Point, len(s.Y))
		missing, invalid := 0, 0
		for k := range s.Y {
			x := float64(k)
			if s.X != nil {
				x = s.X[k]
			}
			y := s.Y[k]
			if math.IsNaN(y) {
				missing++
			} else if math.IsInf(y, 0) {
				invalid++
			}
			if math.IsNaN(x) || math.IsInf(x, 0) {
				invalid++
			}
			pts[k] = downsample.Point{X: x, Y: y}
		}
		if missing > 0 {
			addDiag(DiagInfo, "MISSING_VALUES", fmt.Sprintf("series %d has %d missing (NaN) values; they are not plotted and excluded from statistics", si, missing), si)
		}
		if invalid > 0 {
			addDiag(DiagWarning, "INVALID_VALUES", fmt.Sprintf("series %d has %d non-finite values; they are skipped", si, invalid), si)
		}
		series = append(series, seriesRT{spec: s, pts: pts, color: color, opacity: opacity, xAxis: xa, yAxis: ya, valid: true})
	}

	// ---- domain derivation ----
	deriveDomain := func(a *axisRT, vals []float64, isX bool) {
		valid := make([]float64, 0, len(vals))
		logBad := 0
		for _, v := range vals {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				continue
			}
			if a.kind == scale.KindLog && v <= 0 {
				logBad++
				continue
			}
			valid = append(valid, v)
		}
		if logBad > 0 {
			ax := "y"
			if isX {
				ax = "x"
			}
			addDiag(DiagWarning, "LOG_NON_POSITIVE", fmt.Sprintf("axis %q: %d non-positive values on log %s axis were skipped", a.spec.id, logBad, ax), -1)
		}
		if len(valid) == 0 {
			switch a.kind {
			case scale.KindLog:
				a.lo, a.hi = 1, 10
			case scale.KindTime:
				a.lo, a.hi = 0, 86400
			default:
				a.lo, a.hi = 0, 1
			}
			addDiag(DiagWarning, "EMPTY_AXIS_DOMAIN", fmt.Sprintf("axis %q has no valid data; using default domain [%s, %s]", a.spec.id, fnum(a.lo), fnum(a.hi)), -1)
			return
		}
		lo, hi := valid[0], valid[0]
		for _, v := range valid[1:] {
			if v < lo {
				lo = v
			}
			if v > hi {
				hi = v
			}
		}
		switch a.kind {
		case scale.KindLinear:
			a.lo, a.hi = scale.NiceDomain(lo, hi)
		case scale.KindLog:
			a.lo, a.hi = lo, hi
		case scale.KindTime:
			a.lo, a.hi = lo, hi
		}
	}
	// gather values per axis
	xVals := map[string][]float64{}
	yVals := map[string][]float64{}
	for si, s := range series {
		if !s.valid {
			continue
		}
		for _, p := range s.pts {
			xVals[s.xAxis.spec.id] = append(xVals[s.xAxis.spec.id], p.X)
			yVals[s.yAxis.spec.id] = append(yVals[s.yAxis.spec.id], p.Y)
		}
		_ = si
	}
	for _, id := range axisOrder {
		a := axes[id]
		if a.pos == AxisBottom || a.pos == AxisTop {
			deriveDomain(a, xVals[id], true)
		} else {
			deriveDomain(a, yVals[id], false)
		}
	}
	// explicit bounds override
	for _, id := range axisOrder {
		a := axes[id]
		applyBound := func(v *float64, isMin bool) {
			if v == nil {
				return
			}
			ok := !math.IsNaN(*v) && !math.IsInf(*v, 0)
			if a.kind == scale.KindLog {
				ok = ok && *v > 0
			}
			if !ok {
				addDiag(DiagError, "INVALID_AXIS_BOUNDS", fmt.Sprintf("axis %q explicit bound %s=%s invalid; ignored", a.spec.id, map[bool]string{true: "min", false: "max"}[isMin], fnum(*v)), -1)
				return
			}
			if isMin {
				a.lo = *v
			} else {
				a.hi = *v
			}
		}
		applyBound(a.spec.min, true)
		applyBound(a.spec.max, false)
		if a.lo > a.hi {
			addDiag(DiagError, "INVALID_AXIS_BOUNDS", fmt.Sprintf("axis %q min %s > max %s; swapping", a.spec.id, fnum(a.lo), fnum(a.hi)), -1)
			a.lo, a.hi = a.hi, a.lo
		}
		if a.kind == scale.KindLinear {
			a.lo, a.hi = scale.NiceDomain(a.lo, a.hi)
		}
	}

	// ---- legend items ----
	type legendItem struct {
		si    int
		label string
		color string
		typ   SeriesType
	}
	var legend []legendItem
	for si, s := range series {
		if s.valid && s.spec.Name != "" {
			legend = append(legend, legendItem{si: si, label: s.spec.Name, color: s.color, typ: s.spec.Type})
		}
	}

	// ---- layout ----
	W, H := spec.Width, spec.Height
	titleH := 0.0
	if spec.Title != "" {
		titleH = fontTitle + 8
	}
	legendH := 0.0
	legendRows := 0
	legendPerRow := 1
	if !spec.Legend.Hidden && len(legend) > 0 {
		avail := W - 2*pad
		if avail < 40 {
			avail = 40
		}
		// item width: swatch 26 + gap 8 + text
		perRow := 0
		rowW := 0.0
		for _, it := range legend {
			iw := 26 + 8 + svgb.TextWidth(it.label, fontLegend)
			if perRow == 0 {
				perRow = 1
				rowW = iw
			} else if rowW+8+iw <= avail {
				perRow++
				rowW += 8 + iw
			} else {
				break
			}
		}
		if perRow < 1 {
			perRow = 1
		}
		legendPerRow = perRow
		legendRows = (len(legend) + perRow - 1) / perRow
		legendH = float64(legendRows)*20 + 6
	}
	// tick label widths (using dummy scales)
	maxYLabelW := map[string]float64{}
	for _, id := range axisOrder {
		a := axes[id]
		if a.pos != AxisLeft && a.pos != AxisRight {
			continue
		}
		sc := a.buildScale(0, 1)
		ticks := sc.Ticks(5)
		mw := 0.0
		for _, t := range ticks {
			if t.Major {
				if w := svgb.TextWidth(t.Label, fontTick); w > mw {
					mw = w
				}
			}
		}
		maxYLabelW[id] = mw
	}
	leftM, rightM, topM, bottomM := pad, pad, pad, pad
	if spec.Title != "" {
		topM = pad + titleH
	}
	if legendH > 0 {
		topM += legendH
	}
	for _, id := range axisOrder {
		a := axes[id]
		switch a.pos {
		case AxisLeft:
			w := maxYLabelW[id]
			if w < 30 {
				w = 30
			}
			m := pad + w + 8
			if m > leftM {
				leftM = m
			}
		case AxisRight:
			w := maxYLabelW[id]
			if w < 30 {
				w = 30
			}
			m := pad + w + 8
			if m > rightM {
				rightM = m
			}
		case AxisBottom:
			m := pad + fontTick + 6
			if a.spec.title != "" {
				m += fontAxis + 6
			}
			if m > bottomM {
				bottomM = m
			}
		case AxisTop:
			m := pad + fontTick + 6
			if a.spec.title != "" {
				m += fontAxis + 6
			}
			if m > topM {
				topM = m
			}
		}
	}
	plotX := leftM
	plotY := topM
	plotW := W - leftM - rightM
	plotH := H - topM - bottomM
	if plotW < 10 {
		plotW = 10
	}
	if plotH < 10 {
		plotH = 10
	}

	// ---- build scales with real pixel ranges ----
	for _, id := range axisOrder {
		a := axes[id]
		switch a.pos {
		case AxisBottom, AxisTop:
			a.sc = a.buildScale(plotX, plotX+plotW)
		default:
			a.sc = a.buildScale(plotY+plotH, plotY)
		}
	}

	// ---- tick generation with deterministic thinning ----
	xTickCount := int(plotW / 90)
	if xTickCount < 2 {
		xTickCount = 2
	}
	yTickCount := int(plotH / 40)
	if yTickCount < 2 {
		yTickCount = 2
	}
	tickStride := map[string]int{}
	for _, id := range axisOrder {
		a := axes[id]
		n := xTickCount
		if a.pos == AxisLeft || a.pos == AxisRight {
			n = yTickCount
		}
		ticks := a.sc.Ticks(n)
		// deterministic label thinning
		var labels []string
		totalW := 0.0
		for _, t := range ticks {
			if t.Major {
				labels = append(labels, t.Label)
				totalW += svgb.TextWidth(t.Label, fontTick)
			}
		}
		stride := 1
		if a.pos == AxisBottom || a.pos == AxisTop {
			if totalW+float64(len(labels)-1)*6 > plotW && len(labels) > 2 {
				ratio := (totalW + float64(len(labels)-1)*6) / plotW
				stride = int(math.Ceil(ratio))
				if stride < 1 {
					stride = 1
				}
			}
		} else {
			avail := plotW - 20
			if avail < 20 {
				avail = 20
			}
			if totalW > avail && len(labels) > 2 {
				ratio := totalW / avail
				stride = int(math.Ceil(ratio))
			}
		}
		tickStride[id] = stride
	}

	// ---- downsampling ----
	maxPts := int(plotW * 2)
	if maxPts < 200 {
		maxPts = 200
	}
	if maxPts > 6000 {
		maxPts = 6000
	}
	barBuckets := int(plotW / 4)
	if barBuckets < 20 {
		barBuckets = 20
	}
	if barBuckets > 500 {
		barBuckets = 500
	}
	barData := map[int]map[int]float64{} // series index -> bucket index -> value
	for si, s := range series {
		if !s.valid {
			continue
		}
		if err := ctxErr(ctx); err != nil {
			return nil, err
		}
		switch s.spec.Type {
		case SeriesLine, SeriesArea, SeriesScatter:
			reduced := downsample.MinMax(s.pts, maxPts)
			if len(reduced) < len(s.pts) {
				addDiag(DiagInfo, "DOWNSAMPLED", fmt.Sprintf("series %d %q downsampled from %d to %d points", si, s.spec.Name, len(s.pts), len(reduced)), si)
			}
			series[si].pts = reduced
		case SeriesBar, SeriesStackedBar:
			x0, x1 := s.xAxis.lo, s.xAxis.hi
			buckets := downsample.Bars(s.pts, x0, x1, barBuckets)
			if len(buckets) > 0 {
				m := map[int]float64{}
				for _, b := range buckets {
					m[b.Index] += b.Value
				}
				barData[si] = m
				if len(s.pts) > barBuckets {
					addDiag(DiagInfo, "AGGREGATED", fmt.Sprintf("series %d %q aggregated from %d points to %d bar buckets", si, s.spec.Name, len(s.pts), len(buckets)), si)
				}
			}
		}
	}

	// ---- render ----
	b := svgb.New()
	b.SVG(W, H)
	if bg := bgColor(spec.Background); bg != "" {
		b.Rect(0, 0, W, H, svgb.Attr{K: "fill", V: bg})
	}

	// title
	if spec.Title != "" {
		b.Text(W/2, pad+fontTitle, spec.Title, "middle",
			svgb.Attr{K: "font-family", V: "sans-serif"},
			svgb.Attr{K: "font-size", V: fnum(fontTitle)},
			svgb.Attr{K: "font-weight", V: "bold"},
			svgb.Attr{K: "fill", V: "#222"})
	}

	// legend
	if legendH > 0 {
		lx := pad
		ly := pad
		if spec.Title != "" {
			ly = pad + titleH
		}
		col := 0
		row := 0
		for _, it := range legend {
			iw := 26 + 8 + svgb.TextWidth(it.label, fontLegend)
			x := lx
			y := ly + float64(row)*20
			drawSwatch(b, it.typ, x, y, it.color)
			b.Text(x+26, y+11, it.label, "start",
				svgb.Attr{K: "font-family", V: "sans-serif"},
				svgb.Attr{K: "font-size", V: fnum(fontLegend)},
				svgb.Attr{K: "fill", V: "#333"})
			col++
			if col >= legendPerRow {
				col = 0
				row++
				lx = pad
			} else {
				lx += iw + 8
			}
		}
	}

	// gridlines
	drawGrid := func() {
		for _, id := range axisOrder {
			a := axes[id]
			n := xTickCount
			if a.pos == AxisLeft || a.pos == AxisRight {
				n = yTickCount
			}
			for _, t := range a.sc.Ticks(n) {
				if !t.Major {
					continue
				}
				switch a.pos {
				case AxisBottom, AxisTop:
					x := a.sc.Map(t.Value)
					if x < plotX-0.5 || x > plotX+plotW+0.5 {
						continue
					}
					b.Line(x, plotY, x, plotY+plotH, svgb.Attr{K: "stroke", V: "#ededed"}, svgb.Attr{K: "stroke-width", V: "1"})
				default:
					y := a.sc.Map(t.Value)
					if y < plotY-0.5 || y > plotY+plotH+0.5 {
						continue
					}
					b.Line(plotX, y, plotX+plotW, y, svgb.Attr{K: "stroke", V: "#ededed"}, svgb.Attr{K: "stroke-width", V: "1"})
				}
			}
		}
	}
	drawGrid()

	// zero line
	for _, id := range axisOrder {
		a := axes[id]
		if a.pos != AxisLeft && a.pos != AxisRight {
			continue
		}
		if a.kind == scale.KindLog {
			continue
		}
		lo, hi := a.sc.Domain()
		if lo <= 0 && hi >= 0 {
			y := a.sc.Map(0)
			b.Line(plotX, y, plotX+plotW, y, svgb.Attr{K: "stroke", V: "#bbbbbb"}, svgb.Attr{K: "stroke-width", V: "1"})
		}
	}

	// series
	stackGroups := map[string][]int{}
	var stackOrder []string
	for si, s := range series {
		if !s.valid {
			continue
		}
		switch s.spec.Type {
		case SeriesLine:
			drawLine(b, s, plotX, plotY, plotW, plotH, false)
		case SeriesArea:
			drawLine(b, s, plotX, plotY, plotW, plotH, true)
		case SeriesScatter:
			drawScatter(b, s)
		case SeriesBar:
			drawBars(b, s, barData[si], barBuckets)
		case SeriesStackedBar:
			g := s.spec.StackGroup
			if g == "" {
				g = "default"
			}
			if _, ok := stackGroups[g]; !ok {
				stackOrder = append(stackOrder, g)
			}
			stackGroups[g] = append(stackGroups[g], si)
		}
	}
	// stacked bars (in stack group order, series order within)
	for _, g := range stackOrder {
		group := stackGroups[g]
		// draw per bucket
		for idx := 0; idx < barBuckets; idx++ {
			// check any series has this bucket
			has := false
			for _, si := range group {
				if _, ok := barData[si][idx]; ok {
					has = true
					break
				}
			}
			if !has {
				continue
			}
			posBase, negBase := 0.0, 0.0
			for _, si := range group {
				s := series[si]
				v := barData[si][idx]
				if _, ok := barData[si][idx]; !ok {
					continue
				}
				var base, top float64
				if v >= 0 {
					base, top = posBase, posBase+v
					posBase = top
				} else {
					base, top = negBase, negBase+v
					negBase = top
				}
				drawBarRect(b, s, idx, barBuckets, base, top)
			}
		}
	}

	// axes
	for _, id := range axisOrder {
		a := axes[id]
		drawAxis(b, a, plotX, plotY, plotW, plotH, xTickCount, yTickCount, tickStride[id])
	}

	b.Close()
	return &Result{SVG: b.String(), Diagnostics: diags}, nil
}

func bgColor(bg string) string {
	switch bg {
	case "":
		return "#ffffff"
	case "transparent":
		return ""
	default:
		return bg
	}
}

func toScaleKind(k AxisKind) scale.Kind {
	switch k {
	case AxisLog:
		return scale.KindLog
	case AxisTime:
		return scale.KindTime
	default:
		return scale.KindLinear
	}
}

func (a *axisRT) buildScale(r0, r1 float64) scale.Scale {
	switch a.kind {
	case scale.KindLog:
		return scale.NewLog(a.spec.base, a.lo, a.hi, r0, r1)
	case scale.KindTime:
		return scale.NewTime(a.lo, a.hi, r0, r1)
	default:
		return scale.NewLinear(a.lo, a.hi, r0, r1)
	}
}

func drawSwatch(b *svgb.B, typ SeriesType, x, y float64, color string) {
	switch typ {
	case SeriesLine:
		b.Line(x, y+7, x+20, y+7, svgb.Attr{K: "stroke", V: color}, svgb.Attr{K: "stroke-width", V: "2"})
	case SeriesArea:
		b.Rect(x, y+2, 20, 10, svgb.Attr{K: "fill", V: color}, svgb.Attr{K: "fill-opacity", V: "0.35"})
		b.Line(x, y+2, x+20, y+2, svgb.Attr{K: "stroke", V: color}, svgb.Attr{K: "stroke-width", V: "2"})
	case SeriesScatter:
		b.Circle(x+10, y+7, 4, svgb.Attr{K: "fill", V: color})
	default:
		b.Rect(x, y+2, 20, 10, svgb.Attr{K: "fill", V: color})
	}
}

func drawLine(b *svgb.B, s seriesRT, px, py, pw, ph float64, area bool) {
	sx, sy := s.xAxis.sc, s.yAxis.sc
	base := sy.Map(0)
	if base > py+ph {
		base = py + ph
	}
	if base < py {
		base = py
	}
	var d strings.Builder
	n := len(s.pts)
	i := 0
	for i < n {
		if math.IsNaN(s.pts[i].Y) || !sx.IsValid(s.pts[i].X) || !sy.IsValid(s.pts[i].Y) {
			i++
			continue
		}
		j := i
		for j < n && !math.IsNaN(s.pts[j].Y) && sx.IsValid(s.pts[j].X) && sy.IsValid(s.pts[j].Y) {
			j++
		}
		seg := s.pts[i:j]
		if len(seg) == 0 {
			i = j
			continue
		}
		first := true
		for _, p := range seg {
			x := sx.Map(p.X)
			y := sy.Map(p.Y)
			if math.IsNaN(x) || math.IsNaN(y) {
				continue
			}
			if first {
				d.WriteString("M")
				first = false
			} else {
				d.WriteString("L")
			}
			d.WriteString(fnum(x))
			d.WriteByte(' ')
			d.WriteString(fnum(y))
		}
		if area {
			last := seg[len(seg)-1]
			fx := seg[0]
			d.WriteString("L")
			d.WriteString(fnum(sx.Map(last.X)))
			d.WriteByte(' ')
			d.WriteString(fnum(base))
			d.WriteString("L")
			d.WriteString(fnum(sx.Map(fx.X)))
			d.WriteByte(' ')
			d.WriteString(fnum(base))
			d.WriteString("Z")
		}
		i = j
	}
	if d.Len() == 0 {
		return
	}
	if area {
		b.Path(d.String(),
			svgb.Attr{K: "fill", V: s.color},
			svgb.Attr{K: "fill-opacity", V: fnum(s.opacity * 0.25)},
			svgb.Attr{K: "stroke", V: s.color},
			svgb.Attr{K: "stroke-width", V: "2"},
			svgb.Attr{K: "stroke-opacity", V: fnum(s.opacity)})
	} else {
		b.Path(d.String(),
			svgb.Attr{K: "fill", V: "none"},
			svgb.Attr{K: "stroke", V: s.color},
			svgb.Attr{K: "stroke-width", V: "2"},
			svgb.Attr{K: "stroke-opacity", V: fnum(s.opacity)})
	}
}

func drawScatter(b *svgb.B, s seriesRT) {
	sx, sy := s.xAxis.sc, s.yAxis.sc
	for _, p := range s.pts {
		if math.IsNaN(p.Y) || !sx.IsValid(p.X) || !sy.IsValid(p.Y) {
			continue
		}
		x := sx.Map(p.X)
		y := sy.Map(p.Y)
		if math.IsNaN(x) || math.IsNaN(y) {
			continue
		}
		b.Circle(x, y, 3,
			svgb.Attr{K: "fill", V: s.color},
			svgb.Attr{K: "fill-opacity", V: fnum(s.opacity)})
	}
}

func drawBars(b *svgb.B, s seriesRT, vals map[int]float64, barBuckets int) {
	if vals == nil {
		return
	}
	idxs := make([]int, 0, len(vals))
	for idx := range vals {
		idxs = append(idxs, idx)
	}
	sort.Ints(idxs)
	for _, idx := range idxs {
		drawBarRect(b, s, idx, barBuckets, 0, vals[idx])
	}
}

func drawBarRect(b *svgb.B, s seriesRT, idx, barBuckets int, base, top float64) {
	sx, sy := s.xAxis.sc, s.yAxis.sc
	x0 := s.xAxis.lo + float64(idx)*(s.xAxis.hi-s.xAxis.lo)/float64(barBuckets)
	x1 := s.xAxis.lo + float64(idx+1)*(s.xAxis.hi-s.xAxis.lo)/float64(barBuckets)
	cx := sx.Map((x0 + x1) / 2)
	w := math.Abs(sx.Map(x1)-sx.Map(x0)) * 0.6
	if w < 0.5 {
		w = 0.5
	}
	yTop := sy.Map(top)
	yBot := sy.Map(base)
	if math.IsNaN(yTop) || math.IsNaN(yBot) {
		return
	}
	y := yTop
	h := yBot - yTop
	if h < 0 {
		y, h = yBot, -h
	}
	if h < 0.5 {
		h = 0.5
	}
	b.Rect(cx-w/2, y, w, h,
		svgb.Attr{K: "fill", V: s.color},
		svgb.Attr{K: "fill-opacity", V: fnum(s.opacity)})
}

func drawAxis(b *svgb.B, a *axisRT, px, py, pw, ph float64, xTickCount, yTickCount, stride int) {
	n := xTickCount
	if a.pos == AxisLeft || a.pos == AxisRight {
		n = yTickCount
	}
	ticks := a.sc.Ticks(n)
	// axis line
	switch a.pos {
	case AxisBottom:
		b.Line(px, py+ph, px+pw, py+ph, svgb.Attr{K: "stroke", V: "#333"}, svgb.Attr{K: "stroke-width", V: "1"})
	case AxisTop:
		b.Line(px, py, px+pw, py, svgb.Attr{K: "stroke", V: "#333"}, svgb.Attr{K: "stroke-width", V: "1"})
	case AxisLeft:
		b.Line(px, py, px, py+ph, svgb.Attr{K: "stroke", V: "#333"}, svgb.Attr{K: "stroke-width", V: "1"})
	case AxisRight:
		b.Line(px+pw, py, px+pw, py+ph, svgb.Attr{K: "stroke", V: "#333"}, svgb.Attr{K: "stroke-width", V: "1"})
	}
	major := 0
	for _, t := range ticks {
		if !t.Major {
			continue
		}
		major++
		showLabel := stride == 1 || major == 1 || (major-1)%stride == 0
		switch a.pos {
		case AxisBottom:
			x := a.sc.Map(t.Value)
			if x < px-0.5 || x > px+pw+0.5 {
				continue
			}
			b.Line(x, py+ph, x, py+ph+5, svgb.Attr{K: "stroke", V: "#333"}, svgb.Attr{K: "stroke-width", V: "1"})
			if showLabel {
				b.Text(x, py+ph+5+fontTick+2, t.Label, "middle",
					svgb.Attr{K: "font-family", V: "sans-serif"},
					svgb.Attr{K: "font-size", V: fnum(fontTick)},
					svgb.Attr{K: "fill", V: "#333"})
			}
		case AxisTop:
			x := a.sc.Map(t.Value)
			if x < px-0.5 || x > px+pw+0.5 {
				continue
			}
			b.Line(x, py, x, py-5, svgb.Attr{K: "stroke", V: "#333"}, svgb.Attr{K: "stroke-width", V: "1"})
			if showLabel {
				b.Text(x, py-6, t.Label, "middle",
					svgb.Attr{K: "font-family", V: "sans-serif"},
					svgb.Attr{K: "font-size", V: fnum(fontTick)},
					svgb.Attr{K: "fill", V: "#333"})
			}
		case AxisLeft:
			y := a.sc.Map(t.Value)
			if y < py-0.5 || y > py+ph+0.5 {
				continue
			}
			b.Line(px, y, px-5, y, svgb.Attr{K: "stroke", V: "#333"}, svgb.Attr{K: "stroke-width", V: "1"})
			if showLabel {
				b.Text(px-8, y+4, t.Label, "end",
					svgb.Attr{K: "font-family", V: "sans-serif"},
					svgb.Attr{K: "font-size", V: fnum(fontTick)},
					svgb.Attr{K: "fill", V: "#333"})
			}
		case AxisRight:
			y := a.sc.Map(t.Value)
			if y < py-0.5 || y > py+ph+0.5 {
				continue
			}
			b.Line(px+pw, y, px+pw+5, y, svgb.Attr{K: "stroke", V: "#333"}, svgb.Attr{K: "stroke-width", V: "1"})
			if showLabel {
				b.Text(px+pw+8, y+4, t.Label, "start",
					svgb.Attr{K: "font-family", V: "sans-serif"},
					svgb.Attr{K: "font-size", V: fnum(fontTick)},
					svgb.Attr{K: "fill", V: "#333"})
			}
		}
	}
	// minor ticks for log axes
	if a.kind == scale.KindLog {
		for _, t := range ticks {
			if t.Major {
				continue
			}
			switch a.pos {
			case AxisBottom, AxisTop:
				x := a.sc.Map(t.Value)
				if x < px || x > px+pw {
					continue
				}
				y1, y2 := py+ph, py+ph+3
				if a.pos == AxisTop {
					y1, y2 = py, py-3
				}
				b.Line(x, y1, x, y2, svgb.Attr{K: "stroke", V: "#333"}, svgb.Attr{K: "stroke-width", V: "1"})
			default:
				y := a.sc.Map(t.Value)
				if y < py || y > py+ph {
					continue
				}
				x1, x2 := px, px-3
				if a.pos == AxisRight {
					x1, x2 = px+pw, px+pw+3
				}
				b.Line(x1, y, x2, y, svgb.Attr{K: "stroke", V: "#333"}, svgb.Attr{K: "stroke-width", V: "1"})
			}
		}
	}
	// axis titles
	if a.spec.title != "" {
		switch a.pos {
		case AxisBottom:
			b.Text(px+pw/2, py+ph+5+fontTick+2+fontAxis+4, a.spec.title, "middle",
				svgb.Attr{K: "font-family", V: "sans-serif"},
				svgb.Attr{K: "font-size", V: fnum(fontAxis)},
				svgb.Attr{K: "fill", V: "#222"})
		case AxisTop:
			b.Text(px+pw/2, py-6-fontTick-4, a.spec.title, "middle",
				svgb.Attr{K: "font-family", V: "sans-serif"},
				svgb.Attr{K: "font-size", V: fnum(fontAxis)},
				svgb.Attr{K: "fill", V: "#222"})
		case AxisLeft:
			b.Text(px+4, py+fontAxis+2, a.spec.title, "start",
				svgb.Attr{K: "font-family", V: "sans-serif"},
				svgb.Attr{K: "font-size", V: fnum(fontAxis)},
				svgb.Attr{K: "fill", V: "#222"})
		case AxisRight:
			b.Text(px+pw-4, py+fontAxis+2, a.spec.title, "end",
				svgb.Attr{K: "font-family", V: "sans-serif"},
				svgb.Attr{K: "font-size", V: fnum(fontAxis)},
				svgb.Attr{K: "fill", V: "#222"})
		}
	}
}
