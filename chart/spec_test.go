package chart

import (
	"strings"
	"testing"
)

func TestParseSpecRoundTrip(t *testing.T) {
	doc := `{
	  "width": 640, "height": 400, "title": "demo",
	  "legend": {"visible": true, "position": "bottom"},
	  "xAxis": {"type": "time", "label": "t"},
	  "yAxes": [{"id": "a", "type": "log", "min": 1, "max": 1000}],
	  "palette": ["#111111", "#222222"],
	  "maxPoints": 500,
	  "series": [
	    {"type": "line", "name": "s1", "y": [1, null, 3], "opacity": 0.5},
	    {"type": "stacked-bar", "yAxisId": "a", "y": [2, 4, 6], "color": "#ff0000"}
	  ]
	}`
	spec, err := ParseSpec([]byte(doc))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if spec.Width != 640 || spec.Title != "demo" {
		t.Fatal("fields not decoded")
	}
	if spec.YAxes[0].Type != AxisLog || *spec.YAxes[0].Min != 1 {
		t.Fatal("axis not decoded")
	}
	if spec.Series[0].Y[1] != nil {
		t.Fatal("null must decode to missing value")
	}
	if *spec.Series[0].Opacity != 0.5 {
		t.Fatal("opacity not decoded")
	}
}

func TestParseSpecRejectsUnknownFields(t *testing.T) {
	_, err := ParseSpec([]byte(`{"width":100,"height":100,"bogus":1,"series":[]}`))
	if err == nil {
		t.Fatal("unknown field accepted")
	}
}

func TestParseSpecRejectsBadJSON(t *testing.T) {
	if _, err := ParseSpec([]byte(`{`)); err == nil {
		t.Fatal("bad JSON accepted")
	}
}

func TestValidateCanvasBounds(t *testing.T) {
	for _, wh := range [][2]int{{0, 100}, {100, -1}, {1 << 16, 100}} {
		s := &Spec{Width: wh[0], Height: wh[1]}
		if err := s.Validate(); err == nil {
			t.Fatalf("canvas %v accepted", wh)
		}
	}
}

func TestValidateUnknownSeriesType(t *testing.T) {
	s := &Spec{Width: 100, Height: 100, Series: []SeriesSpec{{Type: "pie"}}}
	if err := s.Validate(); err == nil || !strings.Contains(err.Error(), "unknown type") {
		t.Fatalf("bad series type not rejected: %v", err)
	}
}

func TestParseThenRenderDeterministic(t *testing.T) {
	doc := []byte(`{"width":400,"height":300,"xAxis":{"type":"linear"},
	  "series":[{"type":"line","y":[3,1,4,1,5,9,2,6]}]}`)
	s1, err := ParseSpec(doc)
	if err != nil {
		t.Fatal(err)
	}
	s2, err := ParseSpec(doc)
	if err != nil {
		t.Fatal(err)
	}
	r1 := renderOrFail(t, s1)
	r2 := renderOrFail(t, s2)
	if r1.SVG != r2.SVG {
		t.Fatal("parsed specs render differently")
	}
}
