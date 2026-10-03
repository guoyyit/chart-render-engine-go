// Package chart renders declarative chart descriptions into deterministic,
// embeddable SVG vector text.
package chart

import "context"

// SeriesType identifies the visual mark used to draw a series.
type SeriesType int

const (
	// SeriesLine draws points connected by straight line segments.
	SeriesLine SeriesType = iota
	// SeriesArea draws a line series with a filled area down to the axis.
	SeriesArea
	// SeriesBar draws vertical bars.
	SeriesBar
	// SeriesStackedBar draws vertical bars stacked per stack group.
	SeriesStackedBar
	// SeriesScatter draws one circle per point.
	SeriesScatter
)

// AxisKind identifies the scale family of an axis.
type AxisKind int

const (
	// AxisLinear is a linear numeric scale.
	AxisLinear AxisKind = iota
	// AxisLog is a logarithmic scale (base 10 by default).
	AxisLog
	// AxisTime is a linear scale whose domain values are unix seconds and
	// whose ticks are aligned to calendar / clock boundaries.
	AxisTime
)

// AxisPosition places an axis on the plot edge.
type AxisPosition int

const (
	AxisBottom AxisPosition = iota
	AxisTop
	AxisLeft
	AxisRight
)

// Point is a single (x, y) data point. NaN values are treated as missing
// values and are never plotted or counted in statistics.
type Point struct {
	X float64
	Y float64
}

// Series describes one data series and its visual encoding.
type Series struct {
	// Name is used in the legend. Empty names omit the legend entry.
	Name string
	// Type selects the mark type.
	Type SeriesType
	// X are the x coordinates. When nil, x is the point index starting at 0.
	X []float64
	// Y are the y coordinates. NaN marks a missing value.
	Y []float64
	// Color is an SVG color string (e.g. "#4E79A7"). Empty selects a palette
	// color by series index.
	Color string
	// Opacity in [0,1]. Zero selects full opacity (1).
	Opacity float64
	// XAxis references an axis ID. Defaults to "x".
	XAxis string
	// YAxis references an axis ID. Defaults to "y".
	YAxis string
	// StackGroup groups stacked bar series. Defaults to "default".
	StackGroup string
	// BarWidth in x-axis data units. Zero selects an automatic width.
	BarWidth float64
}

// AxisSpec describes a coordinate axis.
type AxisSpec struct {
	// ID is referenced by series. Default axes are "x" and "y".
	ID string
	// Kind selects the scale family.
	Kind AxisKind
	// Position selects the plot edge.
	Position AxisPosition
	// Min explicitly fixes the domain lower bound. Nil derives it from data.
	Min *float64
	// Max explicitly fixes the domain upper bound. Nil derives it from data.
	Max *float64
	// Base is the logarithm base for AxisLog. Zero means 10.
	Base float64
	// Title is drawn next to the axis.
	Title string
}

// LegendSpec controls the legend.
type LegendSpec struct {
	// Hidden disables legend rendering.
	Hidden bool
}

// Spec is the declarative chart description.
type Spec struct {
	// Width and Height of the canvas in user units (pixels). Must be > 0.
	Width  float64
	Height float64
	// Title is drawn centered above the plot.
	Title string
	// Background is an SVG fill color. Empty means white; "transparent"
	// disables the background rect.
	Background string
	// Series are drawn in slice order, so later series overlay earlier ones.
	Series []Series
	// Axes declares coordinate axes. When empty, default x (bottom, linear)
	// and y (left, linear) axes are created.
	Axes []AxisSpec
	// Legend controls the legend.
	Legend LegendSpec
}

// DiagnosticLevel classifies a diagnostic.
type DiagnosticLevel int

const (
	DiagInfo DiagnosticLevel = iota
	DiagWarning
	DiagError
)

// Diagnostic describes a data or configuration issue encountered while
// rendering. Diagnostics never replace the returned error; they explain how
// the input was interpreted.
type Diagnostic struct {
	Level   DiagnosticLevel
	Code    string
	Message string
	// Series is the series index the diagnostic relates to, or -1.
	Series int
}

// Result is the render outcome.
type Result struct {
	// SVG is the vector chart text.
	SVG string
	// Diagnostics lists data/config issues in deterministic order.
	Diagnostics []Diagnostic
}

// Render renders spec into deterministic SVG text. The result is byte for
// byte identical for identical input and is safe to call concurrently.
// Render honors ctx cancellation: a canceled context returns ctx.Err() and
// produces no output.
func Render(ctx context.Context, spec Spec) (*Result, error) {
	return renderChart(ctx, spec)
}
