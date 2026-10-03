// Package svgb builds SVG documents with deterministic attribute ordering.
package svgb

import (
	"math"
	"strconv"
	"strings"
)

// B is an SVG document builder.
type B struct {
	b strings.Builder
}

// New returns an empty builder.
func New() *B { return &B{} }

// Attr is an SVG attribute.
type Attr struct {
	K string
	V string
}

func num(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return "0"
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// SVG writes the document opening tag. w and h are the canvas size.
func (b *B) SVG(w, h float64) *B {
	b.b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" width="`)
	b.b.WriteString(num(w))
	b.b.WriteString(`" height="`)
	b.b.WriteString(num(h))
	b.b.WriteString(`" viewBox="0 0 `)
	b.b.WriteString(num(w))
	b.b.WriteByte(' ')
	b.b.WriteString(num(h))
	b.b.WriteString(`">`)
	return b
}

// Close writes the document closing tag.
func (b *B) Close() *B {
	b.b.WriteString("</svg>")
	return b
}

func (b *B) attrs(attrs []Attr) {
	for _, a := range attrs {
		b.b.WriteByte(' ')
		b.b.WriteString(a.K)
		b.b.WriteString(`="`)
		b.b.WriteString(a.V)
		b.b.WriteByte('"')
	}
}

// Rect writes a <rect/> element.
func (b *B) Rect(x, y, w, h float64, attrs ...Attr) *B {
	b.b.WriteString("<rect x=\"")
	b.b.WriteString(num(x))
	b.b.WriteString("\" y=\"")
	b.b.WriteString(num(y))
	b.b.WriteString("\" width=\"")
	b.b.WriteString(num(w))
	b.b.WriteString("\" height=\"")
	b.b.WriteString(num(h))
	b.b.WriteByte('"')
	b.attrs(attrs)
	b.b.WriteString("/>")
	return b
}

// Path writes a <path/> element with the given d attribute.
func (b *B) Path(d string, attrs ...Attr) *B {
	b.b.WriteString("<path d=\"")
	b.b.WriteString(d)
	b.b.WriteByte('"')
	b.attrs(attrs)
	b.b.WriteString("/>")
	return b
}

// Line writes a <line/> element.
func (b *B) Line(x1, y1, x2, y2 float64, attrs ...Attr) *B {
	b.b.WriteString("<line x1=\"")
	b.b.WriteString(num(x1))
	b.b.WriteString("\" y1=\"")
	b.b.WriteString(num(y1))
	b.b.WriteString("\" x2=\"")
	b.b.WriteString(num(x2))
	b.b.WriteString("\" y2=\"")
	b.b.WriteString(num(y2))
	b.b.WriteByte('"')
	b.attrs(attrs)
	b.b.WriteString("/>")
	return b
}

// Circle writes a <circle/> element.
func (b *B) Circle(cx, cy, r float64, attrs ...Attr) *B {
	b.b.WriteString("<circle cx=\"")
	b.b.WriteString(num(cx))
	b.b.WriteString("\" cy=\"")
	b.b.WriteString(num(cy))
	b.b.WriteString("\" r=\"")
	b.b.WriteString(num(r))
	b.b.WriteByte('"')
	b.attrs(attrs)
	b.b.WriteString("/>")
	return b
}

func escapeText(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// Text writes a <text/> element. anchor is start, middle or end.
func (b *B) Text(x, y float64, s, anchor string, attrs ...Attr) *B {
	b.b.WriteString("<text x=\"")
	b.b.WriteString(num(x))
	b.b.WriteString("\" y=\"")
	b.b.WriteString(num(y))
	b.b.WriteString("\" text-anchor=\"")
	b.b.WriteString(anchor)
	b.b.WriteByte('"')
	b.attrs(attrs)
	b.b.WriteByte('>')
	b.b.WriteString(escapeText(s))
	b.b.WriteString("</text>")
	return b
}

// String returns the accumulated SVG text.
func (b *B) String() string { return b.b.String() }

// TextWidth estimates the width of s at the given font size. ASCII glyphs
// are estimated at 0.6 em, spaces at 0.3 em and other (e.g. CJK) glyphs at
// 1.0 em. The estimate is deterministic and independent of platform fonts.
func TextWidth(s string, fontSize float64) float64 {
	var w float64
	for _, r := range s {
		switch {
		case r == ' ':
			w += 0.3
		case r < 128:
			w += 0.6
		default:
			w += 1.0
		}
	}
	return w * fontSize
}
