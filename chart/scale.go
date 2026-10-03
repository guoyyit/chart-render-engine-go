package chart

import "math"

// Scale maps data coordinates to pixel coordinates deterministically.
// For log axes the domain is stored pre-transformed (log10).
type Scale struct {
	axisType AxisType
	d0, d1   float64 // domain (transformed for log axes)
	p0, p1   float64 // pixel range
}

// newScale builds a scale from a data-domain range to a pixel range.
func newScale(t AxisType, dMin, dMax, p0, p1 float64) Scale {
	if t == AxisLog {
		dMin, dMax = math.Log10(dMin), math.Log10(dMax)
	}
	if d1 := dMax - dMin; d1 == 0 {
		dMax = dMin + 1
	}
	return Scale{axisType: t, d0: dMin, d1: dMax, p0: p0, p1: p1}
}

// Map converts a data value to a pixel coordinate. Values outside the
// domain are clamped to the pixel range so a stray point cannot produce
// runaway coordinates.
func (s Scale) Map(v float64) float64 {
	if s.axisType == AxisLog {
		if v <= 0 {
			return s.p0 // caller filters non-positive values; clamp anyway
		}
		v = math.Log10(v)
	}
	t := (v - s.d0) / (s.d1 - s.d0)
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return s.p0 + t*(s.p1-s.p0)
}

// deriveRange computes the axis domain from explicit bounds and the
// auto-derived data extent. padFrac pads auto ranges so points do not
// sit on the plot edge. It reports whether a fallback was used.
func deriveRange(spec AxisSpec, dataMin, dataMax float64, hasData bool) (lo, hi float64, fallback bool) {
	lo, hi = dataMin, dataMax
	if !hasData {
		lo, hi = 0, 1
		fallback = true
	}
	if spec.Min != nil {
		lo = *spec.Min
	}
	if spec.Max != nil {
		hi = *spec.Max
	}
	if spec.Type == AxisLog {
		if lo <= 0 {
			lo = smallestPositive(hi)
			fallback = true
		}
		if hi <= lo {
			hi = lo * 10
			fallback = true
		}
		return lo, hi, fallback
	}
	if spec.Min == nil || spec.Max == nil {
		lo, hi = normalizeRange(lo, hi)
		// Pad auto-derived side(s) by 5% so glyphs do not touch edges.
		span := hi - lo
		if spec.Min == nil {
			lo -= span * 0.05
		}
		if spec.Max == nil {
			hi += span * 0.05
		}
	}
	if lo > hi {
		lo, hi = hi, lo
	}
	if lo == hi {
		lo, hi = normalizeRange(lo, hi)
	}
	return lo, hi, fallback
}

// smallestPositive returns a deterministic positive floor below hi.
func smallestPositive(hi float64) float64 {
	if hi <= 0 {
		return 1
	}
	return math.Pow(10, math.Floor(math.Log10(hi))-1)
}
