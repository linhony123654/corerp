package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
)

func TestBrowserOriginConfigurationFailsBeforeOpeningDatabase(t *testing.T) {
	t.Setenv(backgroundIntervalEnvironment, "")
	path := filepath.Join(t.TempDir(), "must-not-create.db")
	err := runWithProviders(context.Background(), path, "127.0.0.1:0", `{"player-token":"principal_player"}`, "cursor-test-secret-at-least-32-bytes", slog.New(slog.NewTextHandler(io.Discard, nil)), core.DeterministicRPDecisionProvider{}, "deterministic", core.DeterministicRPNarrativeProvider{}, "*")
	if !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("invalid browser origin accepted: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("invalid configuration touched database: %v", err)
	}
}

func TestParseTokenConfiguration(t *testing.T) {
	tokens, err := parseTokenConfiguration(`{"buyer-token":"principal_buyer","creator-token":"principal_creator"}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(tokens) != 2 || tokens["buyer-token"] != "principal_buyer" {
		t.Fatalf("token configuration mismatch: %v", tokens)
	}
	for _, invalid := range []string{"", `{}`, `[]`, `{"":"principal_buyer"}`, `{"token":""}`, `{"token":"principal_buyer"} {}`} {
		if _, err := parseTokenConfiguration(invalid); err == nil {
			t.Fatalf("expected invalid token configuration to fail: %q", invalid)
		}
	}
}

func TestRunPerformsBoundedGracefulShutdown(t *testing.T) {
	t.Setenv(backgroundIntervalEnvironment, "")
	// A fixed cancellation delay can interrupt migrations instead of exercising
	// server shutdown. Wait for completed initialization / HTTP serve entry, then
	// independently bound shutdown and require its completion log.
	ctx, cancel := context.WithCancel(context.Background())
	started, stopped := make(chan struct{}, 1), make(chan struct{}, 1)
	logger := slog.New(shutdownTestHandler{slog.NewTextHandler(io.Discard, nil), started, stopped})
	finished := make(chan struct{})
	var runErr error
	dbPath := filepath.Join(t.TempDir(), "shutdown.db")
	t.Cleanup(func() {
		cancel()
		select {
		case <-finished:
		case <-time.After(15 * time.Second):
			t.Error("server did not exit during cleanup")
		}
	})
	go func() {
		runErr = run(ctx, dbPath, "127.0.0.1:0", `{"buyer-token":"principal_buyer"}`, "test-cursor-secret-must-be-at-least-32-bytes", logger)
		close(finished)
	}()
	select {
	case <-started:
	case <-finished:
		t.Fatalf("server exited before HTTP serve entry: %v", runErr)
	case <-time.After(30 * time.Second):
		t.Fatal("server initialization did not finish")
	}
	cancel()
	select {
	case <-finished:
		if runErr != nil {
			t.Fatalf("graceful shutdown: %v", runErr)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("graceful shutdown exceeded its bound")
	}
	select {
	case <-stopped:
	default:
		t.Fatal("server did not complete its graceful shutdown path")
	}
}

func TestParseBackgroundInterval(t *testing.T) {
	for raw, expected := range map[string]time.Duration{"": 0, "30s": 30 * time.Second, "5m": 5 * time.Minute, "1h": time.Hour} {
		actual, err := parseBackgroundInterval(raw)
		if err != nil || actual != expected {
			t.Fatalf("parse %q: %v %v", raw, actual, err)
		}
	}
	for _, raw := range []string{"29s", "61m", "always", "-1m"} {
		if _, err := parseBackgroundInterval(raw); !core.HasCode(err, core.CodeInvalidArgument) {
			t.Fatalf("invalid interval %q: %v", raw, err)
		}
	}
}

type shutdownTestHandler struct {
	slog.Handler
	started chan struct{}
	stopped chan struct{}
}

func (h shutdownTestHandler) Handle(ctx context.Context, record slog.Record) error {
	var signal chan struct{}
	switch record.Message {
	case "CoreRP HTTP API listening":
		signal = h.started
	case "CoreRP HTTP API stopped":
		signal = h.stopped
	}
	if signal != nil {
		select {
		case signal <- struct{}{}:
		default:
		}
	}
	return h.Handler.Handle(ctx, record)
}
