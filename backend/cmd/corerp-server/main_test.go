package main

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"
)

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
	// Leave enough startup time for SQLite migrations under the race detector,
	// then prove the process still exits through its bounded shutdown path.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	err := run(ctx, filepath.Join(t.TempDir(), "shutdown.db"), "127.0.0.1:0", `{"buyer-token":"principal_buyer"}`, "test-cursor-secret-must-be-at-least-32-bytes", logger)
	if err != nil {
		t.Fatalf("graceful shutdown: %v", err)
	}
}
