package chart

import (
	"fmt"
	"math"
)

// cleanSeries is a series after validation and cleaning, ready for
// extent computation, downsampling and rendering.
type cleanSeries struct {
	spec    SeriesSpec
	xs      []float64
	ys      []*float64 // nil = missing or invalid (never treated as zero)
	color   string
	opacity float64
}

// diagCounter aggregates diagnostics per (series, code) so output size
// stays bounded even for pathological inputs.
type diagCounter struct {
	order []diagKey
	count map[diagKey]int
}

type diagKey struct {
	seriesID string
	code     DiagCode
}

func newDiagCounter() *diagCounter {
	return &diagCounter{count: make(map[diagKey]int)}
}

func (d *diagCounter) add(seriesID string, code DiagCode) {
	k := diagKey{seriesID, code}
	if _, ok := d.count[k]; !ok {
		d.order = append(d.order, k)
	}
	d.count[k]++
}

func (d *diagCounter) list() Diagnostics {
	var out Diagnostics
	for _, k := range d.order {
		msg := map[DiagCode]string{
			DiagMissingValue:   "missing y value(s) (null) skipped",
			DiagInvalidValue:   "non-finite value(s) (NaN/Inf) skipped",
			DiagLogNonPositive: "non-positive value(s) dropped on log axis",
		}[k.code]
		out = append(out, Diagnostic{
			Code:     k.code,
			SeriesID: k.seriesID,
			Message:  fmt.Sprintf("%s: %d", msg, d.count[k]),
		})
	}
	return out
}

// clean prepares one series: fills implicit x, drops non-finite values,
// and records diagnostics. Missing values stay nil and never enter
// statistics as zero.
func clean(s SeriesSpec, index int, yAxisType AxisType, dc *diagCounter) cleanSeries {
	id := s.ID
	if id == "" {
		id = fmt.Sprintf("series-%d", index)
	}
	n := len(s.Y)
	xs := make([]float64, n)
	ys := make([]*float64, n)
	for i := 0; i < n; i++ {
		var x float64
		if s.X != nil {
			x = s.X[i]
		} else {
			x = float64(i)
		}
		if math.IsNaN(x) || math.IsInf(x, 0) {
			dc.add(id, DiagInvalidValue)
			continue
		}
		xs[i] = x
		y := s.Y[i]
		if y == nil {
			dc.add(id, DiagMissingValue)
			continue
		}
		v := *y
		if math.IsNaN(v) || math.IsInf(v, 0) {
			dc.add(id, DiagInvalidValue)
			continue
		}
		if yAxisType == AxisLog && v <= 0 {
			dc.add(id, DiagLogNonPositive)
			continue
		}
		vv := v
		ys[i] = &vv
	}
	cs := cleanSeries{spec: s, xs: xs, ys: ys, opacity: 1}
	if s.Opacity != nil {
		o := *s.Opacity
		if o < 0 {
			o = 0
		}
		if o > 1 {
			o = 1
		}
		cs.opacity = o
	}
	return cs
}

// validCount returns the number of usable points.
func (c *cleanSeries) validCount() int {
	n := 0
	for _, y := range c.ys {
		if y != nil {
			n++
		}
	}
	return n
}
