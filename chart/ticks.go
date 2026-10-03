package chart

import (
	"math"
	"strconv"
	"strings"
	"time"
)

// Tick is one axis tick: a value in data coordinates plus its label.
type Tick struct {
	Value float64
	Label string
}

// defaultTickTarget is used when the spec does not request a count.
const defaultTickTarget = 6

func tickTarget(n int) int {
	if n <= 0 {
		return defaultTickTarget
	}
	if n > 20 {
		return 20
	}
	return n
}

// niceStep picks a 1/2/5-scaled step no smaller than raw.
func niceStep(raw float64) float64 {
	if raw <= 0 || math.IsNaN(raw) || math.IsInf(raw, 0) {
		return 1
	}
	mag := math.Pow(10, math.Floor(math.Log10(raw)))
	norm := raw / mag
	var f float64
	switch {
	case norm <= 1:
		f = 1
	case norm <= 2:
		f = 2
	case norm <= 5:
		f = 5
	default:
		f = 10
	}
	return f * mag
}

// stepDecimals returns how many decimal places are needed to represent
// multiples of step exactly enough for labels.
func stepDecimals(step float64) int {
	if step <= 0 {
		return 0
	}
	e := math.Floor(math.Log10(step))
	d := int(-e)
	if d < 0 {
		d = 0
	}
	if d > 10 {
		d = 10
	}
	return d
}

// formatTickNumber renders a tick value with deterministic trimming.
func formatTickNumber(v float64, decimals int) string {
	s := strconv.FormatFloat(v, 'f', decimals, 64)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	}
	if s == "-0" {
		s = "0"
	}
	return s
}

// LinearTicks generates ticks at "nice" (1/2/5-scaled) positions covering
// [min, max]. Degenerate ranges are expanded deterministically.
func LinearTicks(min, max float64, target int) []Tick {
	target = tickTarget(target)
	min, max = normalizeRange(min, max)
	step := niceStep((max - min) / float64(target))
	dec := stepDecimals(step)
	start := math.Ceil(min/step) * step
	var ticks []Tick
	// Iterate by integer index to avoid accumulating float error.
	k0 := int64(math.Round(start / step))
	for i := k0; ; i++ {
		v := float64(i) * step
		if v > max+step*1e-9 {
			break
		}
		if v < min-step*1e-9 {
			continue
		}
		ticks = append(ticks, Tick{Value: v, Label: formatTickNumber(v, dec)})
		if len(ticks) > 1000 {
			break // safety bound; unreachable for sane ranges
		}
	}
	return ticks
}

// normalizeRange expands degenerate or invalid ranges deterministically.
func normalizeRange(min, max float64) (float64, float64) {
	if math.IsNaN(min) || math.IsInf(min, 0) {
		min = 0
	}
	if math.IsNaN(max) || math.IsInf(max, 0) {
		max = min + 1
	}
	if min > max {
		min, max = max, min
	}
	if min == max {
		if min == 0 {
			return 0, 1
		}
		pad := math.Abs(min) * 0.5
		return min - pad, max + pad
	}
	return min, max
}

// LogTicks generates ticks on powers of ten (and 2/5 multiples when the
// range spans less than one decade). min/max must be positive; callers
// clamp beforehand, and non-positive inputs fall back to [1, 10].
func LogTicks(min, max float64, target int) []Tick {
	if min <= 0 || math.IsNaN(min) || math.IsInf(min, 0) {
		min = 1
	}
	if max <= min || math.IsNaN(max) || math.IsInf(max, 0) {
		max = min * 10
	}
	lo := math.Floor(math.Log10(min))
	hi := math.Ceil(math.Log10(max))
	span := hi - lo
	// Choose the coarsest multiplier grid that still yields a usable
	// number of ticks inside the range. Fully deterministic.
	grids := [][]float64{{1}, {1, 2, 5}, {1, 2, 3, 4, 5, 6, 7, 8, 9}}
	if span < 2 {
		grids = grids[1:]
	}
	for gi, mults := range grids {
		var ticks []Tick
		for e := int(lo) - 1; e <= int(hi)+1; e++ {
			base := math.Pow(10, float64(e))
			for _, m := range mults {
				v := m * base
				if v < min*(1-1e-12) || v > max*(1+1e-12) {
					continue
				}
				ticks = append(ticks, Tick{Value: v, Label: formatLogLabel(v)})
			}
		}
		if len(ticks) >= 2 || gi == len(grids)-1 {
			return ticks
		}
	}
	return nil
}

func formatLogLabel(v float64) string {
	if v >= 1e-4 && v < 1e7 {
		return formatTickNumber(v, stepDecimals(v))
	}
	e := int(math.Round(math.Log10(v)))
	if math.Abs(v-math.Pow(10, float64(e))) < v*1e-9 {
		return "1e" + strconv.Itoa(e)
	}
	return strconv.FormatFloat(v, 'e', 0, 64)
}

// timeSteps is the deterministic ladder of time tick steps, in seconds.
var timeSteps = []float64{
	1, 5, 15, 30,
	60, 300, 900, 1800,
	3600, 3 * 3600, 6 * 3600, 12 * 3600,
	86400, 7 * 86400, 30 * 86400, 90 * 86400, 365 * 86400,
}

// TimeTicks generates ticks aligned to UTC boundaries. Values are Unix
// seconds. Output is timezone-independent (always UTC).
func TimeTicks(min, max float64, target int) []Tick {
	target = tickTarget(target)
	min, max = normalizeRange(min, max)
	span := max - min
	step := timeSteps[len(timeSteps)-1]
	for _, s := range timeSteps {
		if span/s <= float64(target) {
			step = s
			break
		}
	}
	// For very large spans, scale the yearly step.
	if span/step > float64(target)*2 {
		step = niceStep(span / float64(target))
	}
	start := math.Ceil(min/step) * step
	var ticks []Tick
	k0 := int64(math.Round(start / step))
	for i := k0; ; i++ {
		v := float64(i) * step
		if v > max+step*1e-9 {
			break
		}
		if v < min-step*1e-9 {
			continue
		}
		ticks = append(ticks, Tick{Value: v, Label: formatTimeLabel(v, step)})
		if len(ticks) > 1000 {
			break
		}
	}
	return ticks
}

func formatTimeLabel(unixSec, step float64) string {
	t := time.Unix(int64(math.Round(unixSec)), 0).UTC()
	switch {
	case step < 60:
		return t.Format("15:04:05")
	case step < 86400:
		return t.Format("15:04")
	case step < 90*86400:
		return t.Format("01-02")
	default:
		return t.Format("2006-01")
	}
}
