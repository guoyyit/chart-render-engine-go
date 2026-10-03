// Package chart implements a deterministic server-side chart rendering
// engine: declarative spec in, SVG text out.
package chart

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// AxisType selects the coordinate semantics of an axis.
type AxisType string

const (
	AxisLinear AxisType = "linear"
	AxisLog    AxisType = "log"
	AxisTime   AxisType = "time"
)

// SeriesType selects the glyph used to draw a series.
type SeriesType string

const (
	SeriesLine       SeriesType = "line"
	SeriesArea       SeriesType = "area"
	SeriesBar        SeriesType = "bar"
	SeriesStackedBar SeriesType = "stacked-bar"
	SeriesScatter    SeriesType = "scatter"
)

// Spec is the declarative chart description. It is JSON-decodable and
// fully determines the rendered output.
type Spec struct {
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Title  string `json:"title,omitempty"`

	Legend LegendSpec `json:"legend,omitempty"`

	XAxis AxisSpec   `json:"xAxis"`
	YAxes []AxisSpec `json:"yAxes,omitempty"`

	Series []SeriesSpec `json:"series"`

	// Palette overrides the default color cycle.
	Palette []string `json:"palette,omitempty"`

	// MaxPoints caps points per series after downsampling (0 = default).
	MaxPoints int `json:"maxPoints,omitempty"`
}

// LegendSpec configures the legend.
type LegendSpec struct {
	Visible  bool   `json:"visible,omitempty"`
	Position string `json:"position,omitempty"` // "right" (default) or "bottom"
}

// AxisSpec describes one axis. Min/Max nil means auto-derive.
type AxisSpec struct {
	ID    string   `json:"id,omitempty"`
	Type  AxisType `json:"type,omitempty"`
	Label string   `json:"label,omitempty"`
	Min   *float64 `json:"min,omitempty"`
	Max   *float64 `json:"max,omitempty"`
	// Ticks is the desired approximate tick count (0 = default).
	Ticks int `json:"ticks,omitempty"`
}

// SeriesSpec describes one data series. Y uses pointers so that null
// encodes a missing value. X may be nil, meaning "use point index".
type SeriesSpec struct {
	ID      string     `json:"id,omitempty"`
	Name    string     `json:"name,omitempty"`
	Type    SeriesType `json:"type"`
	X       []float64  `json:"x,omitempty"`
	Y       []*float64 `json:"y"`
	YAxisID string     `json:"yAxisId,omitempty"`
	Color   string     `json:"color,omitempty"`
	Opacity *float64   `json:"opacity,omitempty"`
}

// ParseSpec decodes and validates a chart spec.
func ParseSpec(data []byte) (*Spec, error) {
	var s Spec
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("invalid chart spec JSON: %w", err)
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return &s, nil
}

// Validate checks structural constraints of the spec.
func (s *Spec) Validate() error {
	if s.Width <= 0 || s.Height <= 0 {
		return fmt.Errorf("spec: width and height must be positive, got %dx%d", s.Width, s.Height)
	}
	if s.Width > 1<<15 || s.Height > 1<<15 {
		return fmt.Errorf("spec: canvas too large: %dx%d", s.Width, s.Height)
	}
	for i := range s.Series {
		sr := &s.Series[i]
		switch sr.Type {
		case SeriesLine, SeriesArea, SeriesBar, SeriesStackedBar, SeriesScatter:
		default:
			return fmt.Errorf("spec: series %d: unknown type %q", i, sr.Type)
		}
		if sr.X != nil && len(sr.X) != len(sr.Y) {
			return fmt.Errorf("spec: series %d: x/y length mismatch (%d vs %d)", i, len(sr.X), len(sr.Y))
		}
	}
	return nil
}
