package svg

import (
	"math"
	"strings"
	"testing"
)

func TestNumDeterministic(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "0"},
		{math.Copysign(0, -1), "0"},
		{1.5, "1.5"},
		{1.234567, "1.23"},
		{100.0, "100"},
		{-2.75, "-2.75"},
	}
	for _, c := range cases {
		if got := Num(c.in); got != c.want {
			t.Errorf("Num(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestEsc(t *testing.T) {
	if got := Esc(`a<b>&"c"`); got != `a&lt;b&gt;&amp;&quot;c&quot;` {
		t.Fatalf("bad escape: %q", got)
	}
}

func TestBuilderWellFormed(t *testing.T) {
	b := New(100, 50)
	b.Open("g", A("id", "x"))
	b.Empty("rect", A("width", Num(10)), A("height", Num(20)))
	b.Text("hi<b>", A("x", "1"))
	b.Close("g")
	out := b.String()
	if !strings.HasPrefix(out, `<svg xmlns="http://www.w3.org/2000/svg"`) {
		t.Fatal("missing svg root")
	}
	if !strings.HasSuffix(out, "</svg>") {
		t.Fatal("unclosed svg root")
	}
	if strings.Count(out, "<g") != strings.Count(out, "</g>") {
		t.Fatal("unbalanced g elements")
	}
	if !strings.Contains(out, "hi&lt;b&gt;") {
		t.Fatal("text not escaped")
	}
	// Same call sequence must produce identical bytes.
	b2 := New(100, 50)
	b2.Open("g", A("id", "x"))
	b2.Empty("rect", A("width", Num(10)), A("height", Num(20)))
	b2.Text("hi<b>", A("x", "1"))
	b2.Close("g")
	if b2.String() != out {
		t.Fatal("builder nondeterministic")
	}
}
