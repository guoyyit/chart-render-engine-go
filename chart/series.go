package chart

import (
	"context"
	"sort"
	"strings"

	"github.com/guoyyit/chart-render-engine-go/svg"
)

// seriesPainter draws series glyphs into the clipped plot group.
type seriesPainter struct {
	b  *svg.Builder
	lo *Layout
	x  *axisModel
}

// opacityAttrs returns deterministic opacity attributes.
func opacityAttrs(o float64) []svg.Attr {
	if o >= 1 {
		return nil
	}
	return []svg.Attr{svg.A("fill-opacity", svg.Num(o)), svg.A("stroke-opacity", svg.Num(o))}
}

// drawSeries renders one non-stacked series.
func (p *seriesPainter) drawSeries(cs *cleanSeries, ym *axisModel, pts []Point) {
	if len(pts) == 0 {
		return
	}
	switch cs.spec.Type {
	case SeriesLine:
		p.drawLine(cs, ym, pts, false)
	case SeriesArea:
		p.drawLine(cs, ym, pts, true)
	case SeriesBar:
		p.drawBars(cs, ym, pts)
	case SeriesScatter:
		p.drawScatter(cs, ym, pts)
	}
}

// drawLine renders a line or filled area path, breaking at gaps.
func (p *seriesPainter) drawLine(cs *cleanSeries, ym *axisModel, pts []Point, fill bool) {
	var d strings.Builder
	baseline := ym.scale.Map(0)
	segStart := -1
	flush := func(end int) {
		if !fill || segStart < 0 || end-segStart < 1 {
			return
		}
		// Close the area segment down to the baseline.
		d.WriteString("L")
		d.WriteString(svg.Num(p.x.scale.Map(pts[end-1].X)))
		d.WriteString(",")
		d.WriteString(svg.Num(baseline))
		d.WriteString("L")
		d.WriteString(svg.Num(p.x.scale.Map(pts[segStart].X)))
		d.WriteString(",")
		d.WriteString(svg.Num(baseline))
		d.WriteString("Z")
	}
	for i, pt := range pts {
		px := p.x.scale.Map(pt.X)
		py := ym.scale.Map(pt.Y)
		if pt.Gap || i == 0 {
			flush(i)
			d.WriteString("M")
			segStart = i
		} else {
			d.WriteString("L")
		}
		d.WriteString(svg.Num(px))
		d.WriteString(",")
		d.WriteString(svg.Num(py))
	}
	flush(len(pts))
	attrs := []svg.Attr{
		svg.A("d", d.String()),
		svg.A("stroke", cs.color),
		svg.A("stroke-width", "1.5"),
	}
	if fill {
		attrs = append(attrs, svg.A("fill", cs.color))
		op := cs.opacity * 0.3
		attrs = append(attrs, svg.A("fill-opacity", svg.Num(op)))
		if cs.opacity < 1 {
			attrs = append(attrs, svg.A("stroke-opacity", svg.Num(cs.opacity)))
		}
	} else {
		attrs = append(attrs, svg.A("fill", "none"))
		attrs = append(attrs, opacityAttrs(cs.opacity)...)
	}
	p.b.Empty("path", attrs...)
}

// drawBars renders grouped bars: series sharing an x position are
// placed side by side in deterministic series order.
func (p *seriesPainter) drawBars(cs *cleanSeries, ym *axisModel, pts []Point) {
	w := p.barWidth(pts)
	baseline := ym.scale.Map(0)
	for _, pt := range pts {
		cx := p.x.scale.Map(pt.X)
		py := ym.scale.Map(pt.Y)
		y0, y1 := baseline, py
		if y1 < y0 {
			y0, y1 = y1, y0
		}
		h := y1 - y0
		if h < 0.5 {
			h = 0.5 // keep zero-height bars faintly visible deterministically
		}
		attrs := []svg.Attr{
			svg.A("x", svg.Num(cx-w/2)),
			svg.A("y", svg.Num(y0)),
			svg.A("width", svg.Num(w)),
			svg.A("height", svg.Num(h)),
			svg.A("fill", cs.color),
		}
		attrs = append(attrs, opacityAttrs(cs.opacity)...)
		p.b.Empty("rect", attrs...)
	}
}

// barWidth derives a deterministic bar width from the minimum x gap.
func (p *seriesPainter) barWidth(pts []Point) float64 {
	if len(pts) < 2 {
		return p.lo.PlotW * 0.1
	}
	xs := make([]float64, len(pts))
	for i, pt := range pts {
		xs[i] = pt.X
	}
	sort.Float64s(xs)
	gap := 0.0
	for i := 1; i < len(xs); i++ {
		d := xs[i] - xs[i-1]
		if d > 0 && (gap == 0 || d < gap) {
			gap = d
		}
	}
	if gap == 0 {
		return p.lo.PlotW * 0.1
	}
	w := p.x.scale.Map(xs[0]+gap*0.7) - p.x.scale.Map(xs[0])
	if w < 1 {
		w = 1
	}
	return w
}

// drawScatter renders fixed-radius circles.
func (p *seriesPainter) drawScatter(cs *cleanSeries, ym *axisModel, pts []Point) {
	for _, pt := range pts {
		attrs := []svg.Attr{
			svg.A("cx", svg.Num(p.x.scale.Map(pt.X))),
			svg.A("cy", svg.Num(ym.scale.Map(pt.Y))),
			svg.A("r", "2.5"),
			svg.A("fill", cs.color),
		}
		attrs = append(attrs, opacityAttrs(cs.opacity)...)
		p.b.Empty("circle", attrs...)
	}
}

// drawStackedBars renders all stacked-bar series grouped by y axis.
// Series stack in spec order; x positions align by exact value.
func (p *seriesPainter) drawStackedBars(ctx context.Context, cleaned []cleanSeries, yAxisOf []int, yModels []axisModel) error {
	// Group stacked series per axis, preserving spec order.
	byAxis := map[int][]int{}
	var axisOrder []int
	for i := range cleaned {
		if cleaned[i].spec.Type != SeriesStackedBar {
			continue
		}
		ai := yAxisOf[i]
		if _, ok := byAxis[ai]; !ok {
			axisOrder = append(axisOrder, ai)
		}
		byAxis[ai] = append(byAxis[ai], i)
	}
	sort.Ints(axisOrder)
	for _, ai := range axisOrder {
		if err := ctx.Err(); err != nil {
			return ErrCanceled
		}
		ym := &yModels[ai]
		// Cumulative sums per x, keys sorted for determinism.
		cum := map[float64]float64{}
		var keys []float64
		seen := map[float64]bool{}
		for _, si := range byAxis[ai] {
			cs := &cleaned[si]
			pts := downsample(cs.xs, cs.ys, 0)
			for _, pt := range pts {
				if !seen[pt.X] {
					seen[pt.X] = true
					keys = append(keys, pt.X)
				}
			}
		}
		sort.Float64s(keys)
		// Bar width from global key gaps.
		width := p.stackedBarWidth(keys)
		for _, si := range byAxis[ai] {
			cs := &cleaned[si]
			pts := downsample(cs.xs, cs.ys, 0)
			vals := map[float64]float64{}
			for _, pt := range pts {
				vals[pt.X] = pt.Y
			}
			for _, k := range keys {
				v, ok := vals[k]
				if !ok {
					continue // missing: contributes nothing, not zero
				}
				y0 := ym.scale.Map(cum[k])
				cum[k] += v
				y1 := ym.scale.Map(cum[k])
				top, bot := y0, y1
				if bot < top {
					top, bot = bot, top
				}
				h := bot - top
				if h < 0.5 {
					h = 0.5
				}
				attrs := []svg.Attr{
					svg.A("x", svg.Num(p.x.scale.Map(k)-width/2)),
					svg.A("y", svg.Num(top)),
					svg.A("width", svg.Num(width)),
					svg.A("height", svg.Num(h)),
					svg.A("fill", cs.color),
				}
				attrs = append(attrs, opacityAttrs(cs.opacity)...)
				p.b.Empty("rect", attrs...)
			}
		}
	}
	return nil
}

func (p *seriesPainter) stackedBarWidth(keys []float64) float64 {
	if len(keys) < 2 {
		return p.lo.PlotW * 0.1
	}
	gap := 0.0
	for i := 1; i < len(keys); i++ {
		d := keys[i] - keys[i-1]
		if d > 0 && (gap == 0 || d < gap) {
			gap = d
		}
	}
	if gap == 0 {
		return p.lo.PlotW * 0.1
	}
	w := p.x.scale.Map(keys[0]+gap*0.7) - p.x.scale.Map(keys[0])
	if w < 1 {
		w = 1
	}
	return w
}
