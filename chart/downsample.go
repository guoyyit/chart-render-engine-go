package chart

// Point is a cleaned, possibly downsampled data point in data coords.
type Point struct {
	X, Y float64
	// Gap is true when a missing/invalid value existed between this
	// point and the previous emitted point: renderers break the path.
	Gap bool
}

// defaultMaxPoints bounds per-series points after downsampling so that
// render time and SVG size stay controlled as input grows.
const defaultMaxPoints = 1000

// downsample reduces (xs, ys) to at most ~maxPoints points using
// min-max bucketing: each bucket contributes its first, last, minimum
// and maximum valid points (in index order, deduplicated). Buckets that
// contain no valid point produce a gap marker on the next emitted point.
//
// The result is a pure function of the input: fully deterministic.
// ys entries may be nil (missing); xs must be non-nil and same length.
func downsample(xs []float64, ys []*float64, maxPoints int) []Point {
	n := len(ys)
	if maxPoints <= 0 {
		maxPoints = defaultMaxPoints
	}
	if n == 0 {
		return nil
	}
	if n <= maxPoints || maxPoints < 8 {
		return emitAll(xs, ys)
	}
	buckets := maxPoints / 4
	if buckets < 1 {
		buckets = 1
	}
	bucketSize := (n + buckets - 1) / buckets
	out := make([]Point, 0, buckets*4)
	pendingGap := false
	for b := 0; b < n; b += bucketSize {
		end := b + bucketSize
		if end > n {
			end = n
		}
		first, last, minI, maxI := -1, -1, -1, -1
		var minV, maxV float64
		for i := b; i < end; i++ {
			y := ys[i]
			if y == nil {
				continue
			}
			if first < 0 {
				first = i
				minI, maxI = i, i
				minV, maxV = *y, *y
			} else {
				if *y < minV {
					minV, minI = *y, i
				}
				if *y > maxV {
					maxV, maxI = *y, i
				}
			}
			last = i
		}
		if first < 0 {
			pendingGap = true // whole bucket missing
			continue
		}
		// Collect bucket representatives in index order, deduplicated.
		sel := dedupSorted(first, minI, maxI, last)
		for j, i := range sel {
			gap := false
			if j == 0 && pendingGap {
				gap = true
			}
			out = append(out, Point{X: xs[i], Y: *ys[i], Gap: gap})
		}
		pendingGap = false
	}
	return out
}

// emitAll emits every valid point, marking gaps after missing entries.
func emitAll(xs []float64, ys []*float64) []Point {
	out := make([]Point, 0, len(ys))
	pendingGap := false
	for i, y := range ys {
		if y == nil {
			pendingGap = true
			continue
		}
		out = append(out, Point{X: xs[i], Y: *y, Gap: pendingGap})
		pendingGap = false
	}
	return out
}

// dedupSorted returns the distinct values of the four indices, which
// are already in non-decreasing order (first <= min/max <= last is not
// guaranteed for min/max, so sort a 4-element slice by hand).
func dedupSorted(a, b, c, d int) []int {
	arr := [4]int{a, b, c, d}
	// insertion sort over 4 elements
	for i := 1; i < 4; i++ {
		for j := i; j > 0 && arr[j] < arr[j-1]; j-- {
			arr[j], arr[j-1] = arr[j-1], arr[j]
		}
	}
	out := make([]int, 0, 4)
	prev := -1
	for _, v := range arr {
		if v != prev {
			out = append(out, v)
			prev = v
		}
	}
	return out
}
