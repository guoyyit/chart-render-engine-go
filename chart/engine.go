package chart

import (
	"context"
	"errors"
)

var _ = errors.New // errors kept for future typed errors

// ErrCanceled is returned when rendering is canceled via context.
var ErrCanceled = errors.New("chart: rendering canceled")

// Result holds the render output.
type Result struct {
	// SVG is the complete SVG document, byte-identical across runs
	// for the same input.
	SVG string `json:"svg"`
	// Diagnostics lists non-fatal issues encountered.
	Diagnostics Diagnostics `json:"diagnostics,omitempty"`
}

// Engine renders chart specs to SVG. It is stateless and safe for
// concurrent use.
type Engine struct{}

// NewEngine creates an Engine.
func NewEngine() *Engine { return &Engine{} }

// Render renders the spec. The same spec always yields a byte-identical
// Result.SVG. If ctx is canceled, Render returns ErrCanceled and no
// partial output.
func (e *Engine) Render(ctx context.Context, spec *Spec) (*Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, ErrCanceled
	}
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	return e.render(ctx, spec)
}
