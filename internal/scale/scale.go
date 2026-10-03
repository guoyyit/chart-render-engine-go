// Package scale maps data domains to pixel ranges and computes semantic
// tick values. All scales are immutable and safe for concurrent use.
package scale

import (
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Kind selects the scale family.
type Kind int

const (
	// KindLinear is a linear numeric scale.
	KindLinear Kind = iota
	// KindLog is a logarithmic scale.
	KindLog
	// KindTime is a linear scale with calendar-aligned ticks.
	KindTime
)

// Tick is one axis tick with its pre-formatted label.
type Tick struct {
	Value float64
	Label string
	// Major ticks are labeled and drawn longer; minor ticks are unlabeled.
	Major bool
}

// Scale maps domain values to a one-dimensional pixel range.
type Scale interface {
	// Map maps a domain value to pixel coordinates.
	Map(v float64) float64
	// Invert maps a pixel coordinate back to a domain value.
	Invert(p float64) float64
	// Ticks returns approximately n semantic ticks covering the domain.
	Ticks(n int) []Tick
	// Domain returns the (lo, hi) domain bounds.
	Domain() (lo, hi float64)
	// IsValid reports whether v can be mapped (e.g. log scales reject v<=0).
	IsValid(v float64) bool
}

// ---------- linear ----------

// Linear is a linear numeric scale.
type Linear struct {
	Lo, Hi float64
	R0, R1 float64
}

// NewLinear returns a linear scale.
func NewLinear(lo, hi, r0, r1 float64) *Linear {
	if lo == hi {
		lo, hi = padEqual(lo)
	}
	return &Linear{Lo: lo, Hi: hi, R0: r0, R1: r1}
}

func (s *Linear) Map(v float64) float64 {
	if s.Hi == s.Lo {
		return (s.R0 + s.R1) / 2
	}
	return s.R0 + (v-s.Lo)/(s.Hi-s.Lo)*(s.R1-s.R0)
}

func (s *Linear) Invert(p float64) float64 {
	if s.R1 == s.R0 {
		return s.Lo
	}
	return s.Lo + (p-s.R0)/(s.R1-s.R0)*(s.Hi-s.Lo)
}

func (s *Linear) Domain() (float64, float64) { return s.Lo, s.Hi }

func (s *Linear) IsValid(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

func (s *Linear) Ticks(n int) []Tick {
	if n < 2 {
		n = 2
	}
	step := NiceStep(s.Hi-s.Lo, n)
	start := math.Floor(s.Lo/step) * step
	end := math.Ceil(s.Hi/step) * step
	var ticks []Tick
	for i := 0; ; i++ {
		v := start + float64(i)*step
		if v > end+step*1e-6 {
			break
		}
		v = math.Round(v/step) * step // suppress float drift
		ticks = append(ticks, Tick{Value: v, Label: FormatTickNumber(v, step), Major: true})
		if i > 10000 { // defensive guard against degenerate steps
			break
		}
	}
	return ticks
}

// ---------- log ----------

// Log is a logarithmic scale.
type Log struct {
	Base   float64
	Lo, Hi float64
	R0, R1 float64
}

// NewLog returns a logarithmic scale. Non-positive domain bounds are
// defensively clamped to positive values.
func NewLog(base, lo, hi, r0, r1 float64) *Log {
	if base <= 0 || base == 1 {
		base = 10
	}
	if lo <= 0 || hi <= 0 {
		// Fall back to a stable positive domain.
		if hi <= 0 {
			hi = 1
		}
		if lo <= 0 {
			lo = hi / math.Pow(base, 3)
		}
	}
	if lo == hi {
		lo, hi = lo/base, hi*base
	}
	return &Log{Base: base, Lo: lo, Hi: hi, R0: r0, R1: r1}
}

func (s *Log) lb(v float64) float64 { return math.Log(v) / math.Log(s.Base) }

func (s *Log) Map(v float64) float64 {
	if !s.IsValid(v) {
		return math.NaN()
	}
	a, b := s.lb(s.Lo), s.lb(s.Hi)
	if a == b {
		return (s.R0 + s.R1) / 2
	}
	return s.R0 + (s.lb(v)-a)/(b-a)*(s.R1-s.R0)
}

func (s *Log) Invert(p float64) float64 {
	a, b := s.lb(s.Lo), s.lb(s.Hi)
	if s.R1 == s.R0 {
		return s.Lo
	}
	e := a + (p-s.R0)/(s.R1-s.R0)*(b-a)
	return math.Pow(s.Base, e)
}

func (s *Log) Domain() (float64, float64) { return s.Lo, s.Hi }

func (s *Log) IsValid(v float64) bool {
	return v > 0 && !math.IsNaN(v) && !math.IsInf(v, 0)
}

func (s *Log) Ticks(n int) []Tick {
	loExp := math.Floor(s.lb(s.Lo))
	hiExp := math.Ceil(s.lb(s.Hi))
	var majors, minors []Tick
	for e := loExp; e <= hiExp; e++ {
		v := math.Pow(s.Base, e)
		if v >= s.Lo && v <= s.Hi {
			if r := math.Round(v); math.Abs(v-r) <= math.Abs(v)*1e-9 {
				v = r
			}
			majors = append(majors, Tick{Value: v, Label: FormatNumber(v), Major: true})
		}
	}
	if len(majors) < 2 {
		// Narrow domain: add factor ticks, labeling 2 and 5.
		for e := loExp - 1; e <= hiExp+1; e++ {
			for m := 2; m <= 9; m++ {
				v := float64(m) * math.Pow(s.Base, e)
				if v >= s.Lo && v <= s.Hi {
					label := ""
					if m == 2 || m == 5 {
						label = FormatNumber(v)
					}
					minors = append(minors, Tick{Value: v, Label: label, Major: m == 2 || m == 5})
				}
			}
		}
	} else {
		// Wide domain: unlabeled minor ticks at factors 2..9.
		for e := loExp; e < hiExp; e++ {
			for m := 2; m <= 9; m++ {
				v := float64(m) * math.Pow(s.Base, e)
				if v > s.Lo && v < s.Hi {
					minors = append(minors, Tick{Value: v, Label: "", Major: false})
				}
			}
		}
	}
	out := append(majors, minors...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Value < out[j].Value })
	return out
}

// ---------- time ----------

// Time is a linear scale with calendar-aligned ticks. Domain values are
// unix seconds; all computation uses UTC for determinism.
type Time struct {
	Lo, Hi float64
	R0, R1 float64
}

// NewTime returns a time scale.
func NewTime(lo, hi, r0, r1 float64) *Time {
	if lo == hi {
		lo, hi = lo-3600, hi+3600
	}
	return &Time{Lo: lo, Hi: hi, R0: r0, R1: r1}
}

func (s *Time) Map(v float64) float64 {
	if s.Hi == s.Lo {
		return (s.R0 + s.R1) / 2
	}
	return s.R0 + (v-s.Lo)/(s.Hi-s.Lo)*(s.R1-s.R0)
}

func (s *Time) Invert(p float64) float64 {
	if s.R1 == s.R0 {
		return s.Lo
	}
	return s.Lo + (p-s.R0)/(s.R1-s.R0)*(s.Hi-s.Lo)
}

func (s *Time) Domain() (float64, float64) { return s.Lo, s.Hi }

func (s *Time) IsValid(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

// nice time step ladder in seconds.
var timeSteps = []float64{
	1, 2, 5, 10, 15, 30,
	60, 120, 300, 600, 900, 1800,
	3600, 7200, 10800, 21600, 43200, 86400,
	172800, 604800, 1209600,
	2629800, 7889400, 15778800, 31557600,
}

func pickTimeStep(target float64) float64 {
	for _, st := range timeSteps {
		if st >= target {
			return st
		}
	}
	return 31557600
}

func (s *Time) Ticks(n int) []Tick {
	if n < 2 {
		n = 2
	}
	span := s.Hi - s.Lo
	step := pickTimeStep(span / float64(n-1))
	lo := int64(math.Floor(s.Lo))
	hi := int64(math.Ceil(s.Hi))
	var ticks []Tick
	add := func(sec int64) {
		v := float64(sec)
		if v >= s.Lo-1e-6 && v <= s.Hi+1e-6 {
			t := time.Unix(sec, 0).UTC()
			ticks = append(ticks, Tick{Value: v, Label: FormatTime(t, step), Major: true})
		}
	}
	switch {
	case step >= 31557600: // years
		start := time.Unix(lo, 0).UTC()
		t := time.Date(start.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
		for t.Unix() <= hi {
			add(t.Unix())
			t = t.AddDate(1, 0, 0)
		}
	case step >= 2629800: // months (1, 2, 3, 6 months)
		months := int(math.Round(step / 2629800))
		if months < 1 {
			months = 1
		}
		start := time.Unix(lo, 0).UTC()
		t := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, time.UTC)
		for t.Unix() <= hi {
			add(t.Unix())
			t = t.AddDate(0, months, 0)
		}
	case step == 604800: // weeks, aligned to Monday
		t := time.Unix(lo, 0).UTC()
		t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
		for t.Weekday() != time.Monday {
			t = t.AddDate(0, 0, -1)
		}
		for t.Unix() <= hi {
			add(t.Unix())
			t = t.AddDate(0, 0, 7)
		}
	default:
		stepSec := int64(step)
		aligned := (lo / stepSec) * stepSec
		for sec := aligned; sec <= hi; sec += stepSec {
			add(sec)
		}
	}
	return ticks
}

// FormatTime formats a tick label for the given step size.
func FormatTime(t time.Time, step float64) string {
	switch {
	case step < 60:
		return t.Format("15:04:05")
	case step < 86400:
		return t.Format("15:04")
	case step < 2629800:
		return t.Format("2006-01-02")
	case step < 31557600:
		return t.Format("2006-01")
	default:
		return t.Format("2006")
	}
}

// ---------- helpers ----------

// padEqual expands a degenerate (zero-span) domain deterministically.
func padEqual(v float64) (float64, float64) {
	if v == 0 {
		return -1, 1
	}
	d := math.Abs(v) * 0.1
	if d < 1 {
		d = 1
	}
	return v - d, v + d
}

// NiceStep returns a "nice" tick step for a span with about n intervals.
func NiceStep(span float64, n int) float64 {
	if span <= 0 || n < 2 {
		return 1
	}
	raw := span / float64(n-1)
	exp := math.Floor(math.Log10(raw))
	f := raw / math.Pow(10, exp)
	var nf float64
	switch {
	case f <= 1:
		nf = 1
	case f <= 2:
		nf = 2
	case f <= 5:
		nf = 5
	default:
		nf = 10
	}
	return nf * math.Pow(10, exp)
}

// NiceDomain expands [lo, hi] to nice boundaries for a linear scale.
func NiceDomain(lo, hi float64) (float64, float64) {
	if lo > hi {
		lo, hi = hi, lo
	}
	if lo == hi {
		lo, hi = padEqual(lo)
	}
	step := NiceStep(hi-lo, 5)
	lo = math.Floor(lo/step) * step
	hi = math.Ceil(hi/step) * step
	return lo, hi
}

// FormatNumber formats a tick number deterministically, using scientific
// notation only for extreme magnitudes.
func FormatNumber(v float64) string {
	return FormatTickNumber(v, 0)
}

// FormatTickNumber formats a tick value given the tick step. The number of
// decimal places is derived from step so that binary float drift (e.g.
// 0.6000000000000001) never reaches the label.
func FormatTickNumber(v, step float64) string {
	if v == 0 {
		return "0"
	}
	av := math.Abs(v)
	if av >= 1e15 || (av > 0 && av < 1e-4) {
		return strconv.FormatFloat(v, 'e', -1, 64)
	}
	decimals := 0
	if step > 0 && step < 1 {
		decimals = int(math.Ceil(-math.Log10(step))) + 1
	}
	s := strconv.FormatFloat(v, 'f', decimals, 64)
	if decimals > 0 && strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	}
	return s
}
