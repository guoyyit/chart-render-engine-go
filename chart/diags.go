package chart

// DiagCode classifies a diagnostic entry.
type DiagCode string

const (
	DiagMissingValue   DiagCode = "missing-value"
	DiagInvalidValue   DiagCode = "invalid-value"
	DiagEmptySeries    DiagCode = "empty-series"
	DiagLengthMismatch DiagCode = "length-mismatch"
	DiagDownsampled    DiagCode = "downsampled"
	DiagAxisFallback   DiagCode = "axis-fallback"
	DiagLogNonPositive DiagCode = "log-non-positive"
)

// Diagnostic is a non-fatal issue found while rendering. Rendering
// continues; diagnostics let callers distinguish causes.
type Diagnostic struct {
	Code     DiagCode `json:"code"`
	SeriesID string   `json:"seriesId,omitempty"`
	Message  string   `json:"message"`
}

// Diagnostics is an ordered list of diagnostics (deterministic order:
// the order issues were discovered).
type Diagnostics []Diagnostic
