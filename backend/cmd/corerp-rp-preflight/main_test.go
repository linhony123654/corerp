package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func authoredSpec() core.StudioWorldSpec {
	return core.StudioWorldSpec{
		Version: core.StudioWorldSpecVersion, Name: "独立预检测试世界", StartWorldTime: core.StudioWorldStart,
		Population: 2,
		Places:     []core.StudioWorldPlace{{Key: "hall", Name: "会客室", Kind: "public"}, {Key: "lane", Name: "走廊", Kind: "public"}},
		Links:      []core.StudioWorldLink{{From: "hall", To: "lane", Minutes: 5}},
		People:     []core.StudioWorldPerson{{Key: "player", Name: "Lin", Place: "hall", Player: true}, {Key: "nora", Name: "Nora", Place: "hall", Persona: "private-persona-marker: gentle and patient"}},
	}
}

func executeSpec(t *testing.T, raw []byte) (int, []byte, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "world.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	var out, diagnostics bytes.Buffer
	code := run([]string{"-spec", path}, &out, &diagnostics)
	unchanged, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(unchanged, raw) {
		t.Fatal("preflight changed the declaration", err)
	}
	return code, out.Bytes(), diagnostics.String()
}

func TestPreflightUsesStudioReadinessWithoutInventingOrExposingCanon(t *testing.T) {
	for _, name := range []string{"unknown_relationship", "missing_persona", "missing_address", "ready_relationship"} {
		t.Run(name, func(t *testing.T) {
			spec := authoredSpec()
			wantCode := 0
			switch name {
			case "missing_persona":
				spec.People[1].Name = "贾母" // A famous name supplies no canon.
				spec.People[1].Persona = ""
				wantCode = 2
			case "missing_address", "ready_relationship":
				spec.Acquaintances = [][2]string{{"player", "nora"}}
				spec.Relationships = []core.StudioWorldRelationship{{From: "nora", To: "player", Role: "朋友"}}
				if name == "ready_relationship" {
					spec.Relationships[0].AddressTo = []string{"Lin"}
				} else {
					wantCode = 2
				}
			}
			raw, err := json.Marshal(spec)
			if err != nil {
				t.Fatal(err)
			}
			code, out, diagnostics := executeSpec(t, raw)
			var report struct {
				Kind        string                 `json:"kind"`
				SpecSHA256  string                 `json:"spec_sha256"`
				RPReadiness core.StudioRPReadiness `json:"rp_readiness"`
			}
			if code != wantCode || diagnostics != "" || json.Unmarshal(out, &report) != nil {
				t.Fatalf("exit=%d diagnostics=%q output=%s", code, diagnostics, out)
			}
			if report.Kind != "corerp.rp-preflight.v1" || !strings.HasPrefix(report.SpecSHA256, "sha256:") || !reflect.DeepEqual(report.RPReadiness, spec.RPConfigurationReadiness()) {
				t.Fatalf("preflight disagrees with the Studio contract: %s", out)
			}
			if bytes.Contains(out, []byte("private-persona-marker")) {
				t.Fatal("preflight exposed private persona")
			}
		})
	}
}

func TestPreflightRejectsInvalidInputsBeforeReportingReady(t *testing.T) {
	raw, err := json.Marshal(authoredSpec())
	if err != nil {
		t.Fatal(err)
	}
	invalidTopology := authoredSpec()
	invalidTopology.Links[0].To = "missing"
	invalid, err := json.Marshal(invalidTopology)
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range [][]byte{[]byte("{"), append(append([]byte{}, raw...), raw...), append(raw[:len(raw)-1:len(raw)-1], []byte(",\"extra\":true}")...), invalid, []byte(strings.Repeat(" ", maxSpecBytes+1))} {
		code, out, diagnostics := executeSpec(t, data)
		if code != 1 || len(out) != 0 || diagnostics == "" {
			t.Fatalf("invalid input produced a readiness claim: code=%d diagnostics=%q output=%s", code, diagnostics, out)
		}
	}
}
