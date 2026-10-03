// Package downsample deterministically reduces series lengths while
// preserving extrema, first and last points per segment.
package downsample

import (
	"math"
	"sort"
)

// Point is a data point for downsampling.
type Point struct {
	X float64
	Y float64
}

// MinMax reduces points to at most maxPoints points. Each contiguous segment
// (split by NaN Y, which denotes a missing value) is bucketed; per bucket the
// minimum-Y and maximum-Y points are kept, plus the segment's first and last
// points. Ties resolve to the earlier point, making the result fully
// deterministic. NaN X points are dropped.
func MinMax(points []Point, maxPoints int) []Point {
	if maxPoints < 4 {
		maxPoints = 4
	}
	var out []Point
	n := len(points)
	i := 0
	for i < n {
		p := points[i]
		if math.IsNaN(p.Y) {
			// Missing value: keep as a gap marker.
			out = append(out, p)
			i++
			continue
		}
		if math.IsNaN(p.X) || math.IsInf(p.X, 0) {
			// Invalid x: cannot be positioned; drop.
			i++
			continue
		}
		j := i
		for j < n && !math.IsNaN(points[j].Y) && !math.IsNaN(points[j].X) && !math.IsInf(points[j].X, 0) {
			j++
		}
		out = append(out, minMaxSegment(points[i:j], maxPoints)...)
		i = j
	}
	return out
}

func minMaxSegment(seg []Point, maxPoints int) []Point {
	if len(seg) <= maxPoints {
		out := make([]Point, len(seg))
		copy(out, seg)
		return out
	}
	buckets := (maxPoints - 2) / 2
	if buckets < 2 {
		buckets = 2
	}
	out := make([]Point, 0, 2*buckets+2)
	out = append(out, seg[0])
	for b := 0; b < buckets; b++ {
		start := b * len(seg) / buckets
		end := (b + 1) * len(seg) / buckets
		if b == 0 {
			start = 1 // first point already kept
		}
		if b == buckets-1 {
			end = len(seg) // last point handled below
		}
		minIdx, maxIdx := -1, -1
		for k := start; k < end; k++ {
			if minIdx < 0 || seg[k].Y < seg[minIdx].Y {
				minIdx = k
			}
			if maxIdx < 0 || seg[k].Y > seg[maxIdx].Y {
				maxIdx = k
			}
		}
		// Emit in index order; skip points identical to the previous one.
		lo, hi := minIdx, maxIdx
		if lo > hi {
			lo, hi = hi, lo
		}
		for _, idx := range []int{lo, hi} {
			if idx < 0 {
				continue
			}
			p := seg[idx]
			if len(out) == 0 || out[len(out)-1] != p {
				out = append(out, p)
			}
		}
	}
	last := seg[len(seg)-1]
	if out[len(out)-1] != last {
		out = append(out, last)
	}
	return out
}

// BarBucket aggregates points of one bar bucket.
type BarBucket struct {
	Index int     // bucket index in [0, maxBuckets)
	X0    float64 // bucket x start
	X1    float64 // bucket x end
	Value float64 // aggregated y (sum of valid points)
	Count int     // number of valid points in the bucket
}

// Bars aggregates points into at most maxBuckets fixed-width buckets over
// the shared x range [x0, x1]. Values are summed so stacked bars stay
// correct. NaN Y and non-finite X points are skipped. Points are sorted by
// X first, so the result does not depend on input order.
func Bars(points []Point, x0, x1 float64, maxBuckets int) []BarBucket {
	if maxBuckets < 1 {
		maxBuckets = 1
	}
	if x0 == x1 {
		x1 = x0 + 1
	}
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	valid := make([]Point, 0, len(points))
	for _, p := range points {
		if math.IsNaN(p.Y) || math.IsNaN(p.X) || math.IsInf(p.X, 0) {
			continue
		}
		valid = append(valid, p)
	}
	sort.SliceStable(valid, func(i, j int) bool { return valid[i].X < valid[j].X })
	if len(valid) == 0 {
		return nil
	}
	width := (x1 - x0) / float64(maxBuckets)
	buckets := make([]BarBucket, maxBuckets)
	for i := range buckets {
		bx0 := x0 + float64(i)*width
		bx1 := x0 + float64(i+1)*width
		buckets[i] = BarBucket{Index: i, X0: bx0, X1: bx1}
	}
	for _, p := range valid {
		idx := int((p.X - x0) / width)
		if idx < 0 {
			idx = 0
		}
		if idx >= maxBuckets {
			idx = maxBuckets - 1
		}
		buckets[idx].Value += p.Y
		buckets[idx].Count++
	}
	out := make([]BarBucket, 0, len(buckets))
	for _, b := range buckets {
		if b.Count > 0 {
			out = append(out, b)
		}
	}
	return out
}
