package chart

// Layout constants (px). Fixed so output is deterministic.
const (
	fontSize     = 12.0
	charWidth    = 7.0 // deterministic estimate per character
	tickLen      = 5.0
	axisPad      = 4.0
	outerPad     = 8.0
	titleHeight  = 22.0
	axisLabelH   = 18.0
	tickLabelH   = 16.0
	legendRowH   = 18.0
	legendSwatch = 12.0
	legendGap    = 6.0
	minPlotSize  = 20.0
)

// textWidth estimates rendered text width deterministically.
func textWidth(s string) float64 { return float64(len(s)) * charWidth }

// axisSide describes one y axis's space requirements on one side.
type axisSide struct {
	label      string
	tickLabels []string
}

func (a axisSide) width() float64 {
	w := tickLen + axisPad
	maxTick := 0.0
	for _, t := range a.tickLabels {
		if tw := textWidth(t); tw > maxTick {
			maxTick = tw
		}
	}
	w += maxTick
	if a.label != "" {
		w += axisLabelH
	}
	return w
}

// Layout is the computed, non-overlapping placement of chart regions.
type Layout struct {
	PlotX, PlotY float64
	PlotW, PlotH float64

	TitleX, TitleY float64
	HasTitle       bool

	LegendX, LegendY float64
	LegendRowH       float64
	HasLegend        bool

	BottomLegendRows int
}

// computeLayout derives the layout from measured content. Every region
// is reserved before the plot area gets the remainder, so regions never
// overlap; the plot shrinks (down to minPlotSize) as content grows.
func computeLayout(spec *Spec, xTickLabels []string, leftAxes, rightAxes []axisSide, legendEntries []string) Layout {
	w := float64(spec.Width)
	h := float64(spec.Height)

	top := outerPad
	bottom := outerPad + tickLabelH
	if spec.XAxis.Label != "" {
		bottom += axisLabelH
	}
	left := outerPad
	for _, a := range leftAxes {
		left += a.width()
	}
	right := outerPad
	for _, a := range rightAxes {
		right += a.width()
	}

	var lo Layout
	if spec.Title != "" {
		lo.HasTitle = true
		lo.TitleX = w / 2
		lo.TitleY = top + fontSize
		top += titleHeight
	}

	legendRight := spec.Legend.Visible && spec.Legend.Position != "bottom"
	legendBottom := spec.Legend.Visible && spec.Legend.Position == "bottom"
	if legendRight && len(legendEntries) > 0 {
		maxW := 0.0
		for _, e := range legendEntries {
			if tw := textWidth(e); tw > maxW {
				maxW = tw
			}
		}
		right += legendSwatch + legendGap + maxW + outerPad
		lo.HasLegend = true
		lo.LegendRowH = legendRowH
	}
	if legendBottom && len(legendEntries) > 0 {
		// Deterministic row packing: entries flow left to right.
		rowW := 0.0
		rows := 1
		avail := w - 2*outerPad
		for _, e := range legendEntries {
			ew := legendSwatch + legendGap + textWidth(e) + legendGap*2
			if rowW+ew > avail && rowW > 0 {
				rows++
				rowW = 0
			}
			rowW += ew
		}
		lo.HasLegend = true
		lo.LegendRowH = legendRowH
		lo.BottomLegendRows = rows
		bottom += float64(rows) * legendRowH
	}

	lo.PlotX = left
	lo.PlotY = top
	lo.PlotW = w - left - right
	lo.PlotH = h - top - bottom
	if lo.PlotW < minPlotSize {
		lo.PlotW = minPlotSize
	}
	if lo.PlotH < minPlotSize {
		lo.PlotH = minPlotSize
	}
	if legendRight && lo.HasLegend {
		lo.LegendX = w - right + outerPad + leftAxesWidth(rightAxes)
		lo.LegendY = lo.PlotY
	}
	if legendBottom && lo.HasLegend {
		lo.LegendX = outerPad
		lo.LegendY = h - bottom + tickLabelH + axisLabelH
	}
	return lo
}

func leftAxesWidth(sides []axisSide) float64 {
	w := 0.0
	for _, s := range sides {
		w += s.width()
	}
	return w
}

// thinLabels deterministically keeps every k-th label so labels do not
// overlap: k is the smallest integer making the kept labels fit.
func thinLabels(labels []string, availablePx float64) []bool {
	n := len(labels)
	keep := make([]bool, n)
	if n == 0 {
		return keep
	}
	maxW := 0.0
	for _, l := range labels {
		if tw := textWidth(l); tw > maxW {
			maxW = tw
		}
	}
	k := 1
	for k < n && float64((n+k-1)/k)*(maxW+axisPad) > availablePx {
		k++
	}
	for i := range keep {
		keep[i] = i%k == 0
	}
	return keep
}
