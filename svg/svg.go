// Package svg provides a deterministic SVG document builder.
// The same sequence of calls always produces byte-identical output.
package svg

import (
	"fmt"
	"strconv"
	"strings"
)

// Attr is a single XML attribute.
type Attr struct {
	Key   string
	Value string
}

// A creates an attribute.
func A(key, value string) Attr { return Attr{Key: key, Value: value} }

// Builder accumulates SVG elements into a document.
type Builder struct {
	buf     strings.Builder
	indent  int
	started bool
}

// New creates a Builder for a canvas of the given size.
func New(width, height int) *Builder {
	b := &Builder{}
	fmt.Fprintf(&b.buf, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`, width, height, width, height)
	b.started = true
	return b
}

// Num formats a float deterministically with at most 2 decimal places,
// trimming trailing zeros so output is byte-stable across runs.
func Num(v float64) string {
	s := strconv.FormatFloat(v, 'f', 2, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "-0" || s == "" {
		s = "0"
	}
	return s
}

// Esc escapes text for safe inclusion in XML.
func Esc(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
	)
	return r.Replace(s)
}

// Open starts an element with the given attributes. Call Close to end it.
func (b *Builder) Open(tag string, attrs ...Attr) {
	b.buf.WriteByte('<')
	b.buf.WriteString(tag)
	for _, a := range attrs {
		b.buf.WriteByte(' ')
		b.buf.WriteString(a.Key)
		b.buf.WriteString(`="`)
		b.buf.WriteString(Esc(a.Value))
		b.buf.WriteByte('"')
	}
	b.buf.WriteByte('>')
	b.indent++
}

// Close ends the most recently opened element.
func (b *Builder) Close(tag string) {
	b.indent--
	b.buf.WriteString("</")
	b.buf.WriteString(tag)
	b.buf.WriteByte('>')
}

// Empty writes a self-closing element.
func (b *Builder) Empty(tag string, attrs ...Attr) {
	b.buf.WriteByte('<')
	b.buf.WriteString(tag)
	for _, a := range attrs {
		b.buf.WriteByte(' ')
		b.buf.WriteString(a.Key)
		b.buf.WriteString(`="`)
		b.buf.WriteString(Esc(a.Value))
		b.buf.WriteByte('"')
	}
	b.buf.WriteString("/>")
}

// Text writes a text element with escaped content.
func (b *Builder) Text(content string, attrs ...Attr) {
	b.Open("text", attrs...)
	b.buf.WriteString(Esc(content))
	b.Close("text")
}

// Raw appends raw markup (used internally by composed helpers).
func (b *Builder) Raw(s string) { b.buf.WriteString(s) }

// String finalizes and returns the document.
func (b *Builder) String() string {
	if b.started {
		b.buf.WriteString("</svg>")
		b.started = false
	}
	return b.buf.String()
}
