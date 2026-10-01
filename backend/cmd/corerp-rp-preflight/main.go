// corerp-rp-preflight checks a creator's declaration before any RP replay.
// It uses the Studio contract directly and never opens a world or a provider.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"corerp.local/backend/internal/core"
)

const maxSpecBytes = 1 << 20

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, output, diagnostics io.Writer) int {
	flags := flag.NewFlagSet("corerp-rp-preflight", flag.ContinueOnError)
	flags.SetOutput(diagnostics)
	path := flags.String("spec", "", "creator-authored Studio world declaration JSON")
	if err := flags.Parse(args); err != nil {
		return 1
	}
	if *path == "" || flags.NArg() != 0 {
		fmt.Fprintln(diagnostics, "usage: corerp-rp-preflight -spec /path/to/world.json")
		return 1
	}
	file, err := os.Open(*path)
	if err != nil {
		fmt.Fprintln(diagnostics, "cannot read world declaration")
		return 1
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxSpecBytes+1))
	if err != nil || len(raw) > maxSpecBytes {
		fmt.Fprintln(diagnostics, "world declaration unreadable or exceeds 1 MiB")
		return 1
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var spec core.StudioWorldSpec
	if err := decoder.Decode(&spec); err != nil {
		fmt.Fprintln(diagnostics, "invalid Studio world declaration JSON")
		return 1
	}
	if decoder.Decode(new(any)) != io.EOF {
		fmt.Fprintln(diagnostics, "world declaration must contain exactly one JSON document")
		return 1
	}
	if err := spec.Validate(); err != nil {
		fmt.Fprintln(diagnostics, "world declaration violates the Studio contract")
		return 1
	}
	readiness := spec.RPConfigurationReadiness()
	report := struct {
		Kind        string                 `json:"kind"`
		SpecSHA256  string                 `json:"spec_sha256"`
		RPReadiness core.StudioRPReadiness `json:"rp_readiness"`
	}{"corerp.rp-preflight.v1", fmt.Sprintf("sha256:%x", sha256.Sum256(raw)), readiness}
	encoder := json.NewEncoder(output)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		fmt.Fprintln(diagnostics, "cannot write RP readiness report")
		return 1
	}
	if readiness.Status != "READY" {
		return 2
	}
	return 0
}
