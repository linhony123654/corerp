package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"corerp.local/backend/internal/storage"
)

func TestLocalStudioAccessCLI(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "admin.db")
	args := []string{"-db", path, "-operator", "principal_operator", "-instance", storage.M2DemoInstanceID, "-branch", storage.M2DemoBranchID, "-target", "principal_creator", "-status", "active", "-expected-head", "1", "-key", "cli-grant"}
	var out bytes.Buffer
	if err := run(ctx, args, &out); err == nil {
		t.Fatal("missing database accepted")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("CLI created missing database")
	}
	s, err := storage.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.BootstrapDemo(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.BootstrapM2Demo(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BootstrapM2AgentDemo(ctx); err != nil {
		t.Fatal(err)
	}
	life, err := s.RunAgentLife(ctx, storage.M2AgentSetupTime, 1)
	if err != nil {
		t.Fatal(err)
	}
	for i, arg := range args {
		if arg == "-expected-head" {
			args[i+1] = strconv.FormatInt(life.HeadSequence, 10)
		}
	}
	s.Close()
	if err := run(ctx, args, &out); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Bytes(), []byte(`"event_id"`)) {
		t.Fatal("no recovery receipt")
	}
	out.Reset()
	if err := run(ctx, args, &out); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Bytes(), []byte(`"replayed":true`)) {
		t.Fatal("CLI retry duplicated event")
	}
	s, err = storage.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.ReadStudioEvent(ctx, storage.StudioEventRequest{PrincipalID: "principal_creator", InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, EventID: "event_m2_cohort_initialized"}); err != nil {
		t.Fatal("CLI grant unusable", err)
	}
	explanation := storage.StudioExplanationRequest{PrincipalID: "principal_creator", InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, EventID: "event_m2_cohort_initialized"}
	if _, err := s.ReadStudioExplanation(ctx, explanation); err == nil {
		t.Fatal("legacy CLI grant silently expanded to explanation access")
	}
	s.Close()
	expanded := append(append([]string{}, args...), "-explain")
	if err := run(ctx, expanded, &out); err == nil {
		t.Fatal("changed permission accepted under old idempotency key")
	}
	for i, arg := range expanded {
		if arg == "-expected-head" {
			expanded[i+1] = strconv.FormatInt(life.HeadSequence+1, 10)
		}
		if arg == "-key" {
			expanded[i+1] = "cli-explain-grant"
		}
	}
	out.Reset()
	if err := run(ctx, expanded, &out); err != nil {
		t.Fatal("explicit explanation grant failed", err)
	}
	s, err = storage.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.ReadStudioExplanation(ctx, explanation); err != nil {
		t.Fatal("explicit CLI explanation grant unusable after reopen", err)
	}
	s.Close()
	createArgs := append(append([]string{}, args...), "-purpose", "create_world")
	for i, arg := range createArgs {
		if arg == "-expected-head" {
			createArgs[i+1] = strconv.FormatInt(life.HeadSequence+2, 10)
		}
		if arg == "-key" {
			createArgs[i+1] = "cli-create-authority"
		}
	}
	out.Reset()
	if err := run(ctx, createArgs, &out); err != nil {
		t.Fatal("CLI creation authority", err)
	}
	if !bytes.Contains(out.Bytes(), []byte(`"capability_id":"world.create"`)) {
		t.Fatal("CLI did not create independent capability")
	}
	out.Reset()
	if err := run(ctx, createArgs, &out); err != nil || !bytes.Contains(out.Bytes(), []byte(`"replayed":true`)) {
		t.Fatal("CLI creation-authority retry", err)
	}
}
