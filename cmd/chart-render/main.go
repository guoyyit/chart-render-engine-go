// Command chart-render reads a chart spec (JSON) from a file or stdin
// and writes the rendered SVG to stdout. Diagnostics go to stderr as
// JSON so the SVG stream stays clean.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/guoyyit/chart-render-engine-go/chart"
)

func main() {
	in := flag.String("in", "", "input spec file (default: stdin)")
	diag := flag.Bool("diag", false, "print diagnostics to stderr")
	flag.Parse()

	var data []byte
	var err error
	if *in == "" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(*in)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "read spec:", err)
		os.Exit(2)
	}
	spec, err := chart.ParseSpec(data)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	res, err := chart.NewEngine().Render(context.Background(), spec)
	if err != nil {
		fmt.Fprintln(os.Stderr, "render:", err)
		os.Exit(1)
	}
	if *diag && len(res.Diagnostics) > 0 {
		enc := json.NewEncoder(os.Stderr)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res.Diagnostics)
	}
	fmt.Print(res.SVG)
}
