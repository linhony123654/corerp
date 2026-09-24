package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestM2CLIRPLifePreparationIsExplicitAndRepeatable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rp-life-cli.db")
	var output bytes.Buffer
	for attempt := 0; attempt < 2; attempt++ {
		output.Reset()
		if err := run(context.Background(), path, "rp-life-prepare", &output); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(output.String(), `"npc_count": 3`) || !strings.Contains(output.String(), `"schedule_count": 116`) {
			t.Fatalf("missing life composition: %s", output.String())
		}
		if attempt == 1 && strings.Count(output.String(), `"replayed": true`) != 4 {
			t.Fatalf("life preparation not idempotent: %s", output.String())
		}
	}
}

func TestM2CLIFinalCohortPreparationIsRepeatable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "final-cohorts-cli.db")
	var output bytes.Buffer
	for attempt := 0; attempt < 2; attempt++ {
		output.Reset()
		if err := run(context.Background(), path, "rp-final-cohorts-prepare", &output); err != nil {
			t.Fatal(err)
		}
		for _, expected := range []string{`"cohort_id": "cohort_final_block_b"`, `"cohort_id": "cohort_final_block_c"`, `"population_count": 4`} {
			if !strings.Contains(output.String(), expected) {
				t.Fatalf("missing %s: %s", expected, output.String())
			}
		}
		if attempt == 1 && strings.Count(output.String(), `"replayed": true`) != 2 {
			t.Fatalf("not replayed: %s", output.String())
		}
	}
}

func TestM2CLIBootstrapAndRoundTripAreRepeatable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m2-cli.db")
	var output bytes.Buffer
	if err := run(context.Background(), path, "bootstrap", &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"status": "ready"`) {
		t.Fatalf("unexpected bootstrap output: %s", output.String())
	}
	output.Reset()
	if err := run(context.Background(), path, "roundtrip", &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"first_sequence": 2`) || !strings.Contains(output.String(), `"first_sequence": 3`) {
		t.Fatalf("unexpected roundtrip output: %s", output.String())
	}
	output.Reset()
	if err := run(context.Background(), path, "roundtrip", &output); err != nil {
		t.Fatal(err)
	}
	if strings.Count(output.String(), `"replayed": true`) != 2 {
		t.Fatalf("repeated roundtrip was not idempotent: %s", output.String())
	}
}

func TestM2CLIAgentLifeIsRepeatable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m2-agent-cli.db")
	var output bytes.Buffer
	if err := run(context.Background(), path, "agents", &output); err != nil {
		t.Fatal(err)
	}
	first := output.String()
	for _, expected := range []string{`"processed_items": 4`, `"place_id": "place_m2_cafe"`, `"subject_agent_id": "entity_m2_agent_bo"`} {
		if !strings.Contains(first, expected) {
			t.Fatalf("Agent CLI output lacks %s: %s", expected, first)
		}
	}
	output.Reset()
	if err := run(context.Background(), path, "agents", &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"replayed": true`) || !strings.Contains(output.String(), `"processed_items": 0`) {
		t.Fatalf("repeated Agent CLI run was not idempotent: %s", output.String())
	}
}

func TestM2CLIRPPrepareIsRepeatable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m2-rp-cli.db")
	var output bytes.Buffer
	if err := run(context.Background(), path, "rp-prepare", &output); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`"event_sequence": 7`, `"player_id": "entity_m2_rp_lin"`, `"npc_count": 3`, `"place_count": 5`} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("RP setup CLI result lacks %s: %s", expected, output.String())
		}
	}
	output.Reset()
	if err := run(context.Background(), path, "rp-prepare", &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"replayed": true`) || !strings.Contains(output.String(), `"event_sequence": 7`) {
		t.Fatalf("repeated RP setup changed world: %s", output.String())
	}
}

func TestM2CLIRPTravelPrepareIsRepeatable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m2-rp-travel-cli.db")
	var output bytes.Buffer
	if err := run(context.Background(), path, "rp-travel-prepare", &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"event_sequence": 8`) || !strings.Contains(output.String(), `"link_count": 8`) {
		t.Fatalf("RP travel setup output was not authoritative: %s", output.String())
	}
	output.Reset()
	if err := run(context.Background(), path, "rp-travel-prepare", &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"replayed": true`) || !strings.Contains(output.String(), `"event_sequence": 8`) {
		t.Fatalf("RP travel setup retry changed authority: %s", output.String())
	}
}

func TestM2CLIEconomyDayOneIsExplicitAndRestartable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m2-economy-cli.db")
	var output bytes.Buffer
	if err := run(context.Background(), path, "economy-day1", &output); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`"processed_items": 2`, `"head_sequence": 7`, `"current_world_time": "2026-09-23T07:01:00Z"`} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("economic CLI result lacks %s: %s", expected, output.String())
		}
	}
	output.Reset()
	if err := run(context.Background(), path, "economy-day1", &output); err != nil {
		t.Fatal(err)
	}
	if strings.Count(output.String(), `"replayed": true`) != 2 || !strings.Contains(output.String(), `"processed_items": 0`) {
		t.Fatalf("repeat economic CLI run changed facts: %s", output.String())
	}
}

func TestM2CLIEconomyThirtyDayIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m2-economy-30-cli.db")
	var output bytes.Buffer
	if err := run(context.Background(), path, "economy30", &output); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`"schedule_count": 116`, `"processed_items": 393`, `"head_sequence": 399`, `"current_world_time": "2026-10-22T12:00:00Z"`} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("30-day mixed CLI result lacks %s: %s", expected, output.String())
		}
	}
	output.Reset()
	if err := run(context.Background(), path, "economy30", &output); err != nil {
		t.Fatal(err)
	}
	if strings.Count(output.String(), `"replayed": true`) != 3 || !strings.Contains(output.String(), `"processed_items": 0`) {
		t.Fatalf("repeat 30-day mixed CLI changed facts: %s", output.String())
	}
}

func TestM2CLIThirtyDayAgentRoutineIsRestartable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m2-agent-30-cli.db")
	var output bytes.Buffer
	if err := run(context.Background(), path, "agents", &output); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := run(context.Background(), path, "agents30", &output); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`"schedule_count": 116`, `"processed_items": 116`, `"head_sequence": 125`, `"current_world_time": "2026-10-22T12:00:00Z"`} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("30-day CLI lacks %s: %s", expected, output.String())
		}
	}
	output.Reset()
	if err := run(context.Background(), path, "agents30", &output); err != nil {
		t.Fatal(err)
	}
	if strings.Count(output.String(), `"replayed": true`) != 2 || !strings.Contains(output.String(), `"processed_items": 0`) || !strings.Contains(output.String(), `"head_sequence": 125`) {
		t.Fatalf("repeated 30-day CLI changed authority: %s", output.String())
	}
}

func TestM2CLIOptInUnattendedDriver(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m2-agent-driver-cli.db")
	var output bytes.Buffer
	if err := run(context.Background(), path, "agents30-prepare", &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"schedule_count": 116`) {
		t.Fatalf("unprepared routine: %s", output.String())
	}
	output.Reset()
	if err := runAgentDriver(context.Background(), path, time.Millisecond, 10, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"status": "completed"`) || !strings.Contains(output.String(), `"processed_items": 120`) || !strings.Contains(output.String(), `"head_sequence": 125`) {
		t.Fatalf("unattended driver did not finish 30-day routine: %s", output.String())
	}
}
