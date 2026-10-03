package chart

import (
	"github.com/guoyyit/chart-render-engine-go/svg"
)

// Color and style constants: fixed for deterministic output.
const (
	colorAxis  = "#333333"
	colorGrid  = "#e0e0e0"
	colorText  = "#333333"
	colorTitle = "#111111"
	colorFrame = "#ffffff"
	fontFamily = "Helvetica, Arial, sans-serif"
)

// drawFrame paints the background, plot border and title.
func drawFrame(b *svg.Builder, spec *Spec, lo *Layout) {
	b.Empty("rect",
		svg.A("x", "0"), svg.A("y", "0"),
		svg.A("width", svg.Num(float64(spec.Width))),
		svg.A("height", svg.Num(float64(spec.Height))),
		svg.A("fill", colorFrame))
	b.Empty("rect",
		svg.A("x", svg.Num(lo.PlotX)), svg.A("y", svg.Num(lo.PlotY)),
		svg.A("width", svg.Num(lo.PlotW)), svg.A("height", svg.Num(lo.PlotH)),
		svg.A("fill", "none"), svg.A("stroke", colorAxis),
		svg.A("stroke-width", "1"))
	if lo.HasTitle {
		b.Text(spec.Title,
			svg.A("x", svg.Num(lo.TitleX)), svg.A("y", svg.Num(lo.TitleY)),
			svg.A("text-anchor", "middle"),
			svg.A("font-family", fontFamily),
			svg.A("font-size", "14"),
			svg.A("font-weight", "bold"),
			svg.A("fill", colorTitle))
	}
}

// drawXAxis draws gridlines, ticks and labels for the shared x axis.
func drawXAxis(b *svg.Builder, lo *Layout, m *axisModel) {
	base := lo.PlotY + lo.PlotH
	for i, t := range m.ticks {
		px := m.scale.Map(t.Value)
		b.Empty("line",
			svg.A("x1", svg.Num(px)), svg.A("y1", svg.Num(lo.PlotY)),
			svg.A("x2", svg.Num(px)), svg.A("y2", svg.Num(base)),
			svg.A("stroke", colorGrid), svg.A("stroke-width", "1"))
		b.Empty("line",
			svg.A("x1", svg.Num(px)), svg.A("y1", svg.Num(base)),
			svg.A("x2", svg.Num(px)), svg.A("y2", svg.Num(base+tickLen)),
			svg.A("stroke", colorAxis), svg.A("stroke-width", "1"))
		if m.keep[i] {
			b.Text(t.Label,
				svg.A("x", svg.Num(px)), svg.A("y", svg.Num(base+tickLen+fontSize)),
				svg.A("text-anchor", "middle"),
				svg.A("font-family", fontFamily),
				svg.A("font-size", svg.Num(fontSize)),
				svg.A("fill", colorText))
		}
	}
	if m.spec.Label != "" {
		b.Text(m.spec.Label,
			svg.A("x", svg.Num(lo.PlotX+lo.PlotW/2)),
			svg.A("y", svg.Num(base+tickLen+fontSize+axisLabelH)),
			svg.A("text-anchor", "middle"),
			svg.A("font-family", fontFamily),
			svg.A("font-size", svg.Num(fontSize)),
			svg.A("fill", colorText))
	}
}

// drawYAxis draws one y axis. Axes with even index sit on the left,
// odd on the right; multiple axes per side stack outward in index
// order, which makes the alignment reproducible.
func drawYAxis(b *svg.Builder, lo *Layout, m *axisModel, idx, total int, grid bool) {
	side := idx % 2 // 0 = left, 1 = right
	depth := idx / 2
	var axisX float64
	if side == 0 {
		axisX = lo.PlotX - float64(depth)*axisSlotWidth(m)
	} else {
		axisX = lo.PlotX + lo.PlotW + float64(depth)*axisSlotWidth(m)
	}
	for i, t := range m.ticks {
		py := m.scale.Map(t.Value)
		if grid {
			b.Empty("line",
				svg.A("x1", svg.Num(lo.PlotX)), svg.A("y1", svg.Num(py)),
				svg.A("x2", svg.Num(lo.PlotX+lo.PlotW)), svg.A("y2", svg.Num(py)),
				svg.A("stroke", colorGrid), svg.A("stroke-width", "1"))
		}
		tickOut := tickLen
		if side == 1 {
			tickOut = -tickLen
		}
		b.Empty("line",
			svg.A("x1", svg.Num(axisX)), svg.A("y1", svg.Num(py)),
			svg.A("x2", svg.Num(axisX+tickOut)), svg.A("y2", svg.Num(py)),
			svg.A("stroke", colorAxis), svg.A("stroke-width", "1"))
		if m.keep[i] {
			anchor := "end"
			labelX := axisX - tickLen - axisPad
			if side == 1 {
				anchor = "start"
				labelX = axisX + tickLen + axisPad
			}
			b.Text(t.Label,
				svg.A("x", svg.Num(labelX)), svg.A("y", svg.Num(py+fontSize/3)),
				svg.A("text-anchor", anchor),
				svg.A("font-family", fontFamily),
				svg.A("font-size", svg.Num(fontSize)),
				svg.A("fill", colorText))
		}
	}
	b.Empty("line",
		svg.A("x1", svg.Num(axisX)), svg.A("y1", svg.Num(lo.PlotY)),
		svg.A("x2", svg.Num(axisX)), svg.A("y2", svg.Num(lo.PlotY+lo.PlotH)),
		svg.A("stroke", colorAxis), svg.A("stroke-width", "1"))
	if m.spec.Label != "" {
		cx := axisX
		if side == 0 {
			cx = axisX - axisSlotWidth(m) + axisLabelH/2
		} else {
			cx = axisX + axisSlotWidth(m) - axisLabelH/2
		}
		cy := lo.PlotY + lo.PlotH/2
		b.Open("text",
			svg.A("x", svg.Num(cx)), svg.A("y", svg.Num(cy)),
			svg.A("text-anchor", "middle"),
			svg.A("font-family", fontFamily),
			svg.A("font-size", svg.Num(fontSize)),
			svg.A("fill", colorText),
			svg.A("transform", "rotate(-90 "+svg.Num(cx)+" "+svg.Num(cy)+")"))
		b.Raw(svg.Esc(m.spec.Label))
		b.Close("text")
	}
}

// axisSlotWidth mirrors axisSide.width for a resolved axis model so
// stacked axes and layout reserve identical space.
func axisSlotWidth(m *axisModel) float64 {
	w := tickLen + axisPad + maxTickWidth(m)
	if m.spec.Label != "" {
		w += axisLabelH
	}
	return w
}

func maxTickWidth(m *axisModel) float64 {
	maxW := 0.0
	for _, t := range m.ticks {
		if tw := textWidth(t.Label); tw > maxW {
			maxW = tw
		}
	}
	return maxW
}

// drawLegend draws swatches and names at the layout-reserved position.
func drawLegend(b *svg.Builder, spec *Spec, lo *Layout, cleaned []cleanSeries) {
	if !lo.HasLegend || len(cleaned) == 0 {
		return
	}
	if spec.Legend.Position == "bottom" {
		x := lo.LegendX
		y := lo.LegendY
		rowW := 0.0
		avail := float64(spec.Width) - 2*outerPad
		for i := range cleaned {
			name := legendName(spec, i)
			ew := legendSwatch + legendGap + textWidth(name) + legendGap*2
			if rowW+ew > avail && rowW > 0 {
				y += legendRowH
				x = lo.LegendX
				rowW = 0
			}
			legendItem(b, x, y, cleaned[i].color, name)
			x += ew
			rowW += ew
		}
		return
	}
	y := lo.LegendY
	for i := range cleaned {
		legendItem(b, lo.LegendX, y, cleaned[i].color, legendName(spec, i))
		y += legendRowH
	}
}

func legendName(spec *Spec, i int) string {
	if spec.Series[i].Name != "" {
		return spec.Series[i].Name
	}
	return seriesID(spec.Series[i], i)
}

func legendItem(b *svg.Builder, x, y float64, color, name string) {
	b.Empty("rect",
		svg.A("x", svg.Num(x)), svg.A("y", svg.Num(y)),
		svg.A("width", svg.Num(legendSwatch)), svg.A("height", svg.Num(legendSwatch)),
		svg.A("fill", color))
	b.Text(name,
		svg.A("x", svg.Num(x+legendSwatch+legendGap)),
		svg.A("y", svg.Num(y+legendSwatch-2)),
		svg.A("font-family", fontFamily),
		svg.A("font-size", svg.Num(fontSize)),
		svg.A("fill", colorText))
}
