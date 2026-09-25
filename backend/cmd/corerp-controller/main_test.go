package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/storage"
)

func TestControllerCLIRequiresExistingDBAndOneKnownJSONRequest(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "world.db")
	if err := run(ctx, path, "enroll", strings.NewReader(`{}`), &bytes.Buffer{}); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatal("missing DB accepted", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("local controller command created a missing DB", err)
	}
	store, err := storage.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{`{"unknown":true}`, `{} {}`} {
		if err := run(ctx, path, "assign", strings.NewReader(body), &bytes.Buffer{}); !core.HasCode(err, core.CodeInvalidArgument) {
			t.Fatalf("invalid local request %q accepted: %v", body, err)
		}
	}
	if err := run(ctx, path, "release", strings.NewReader(`{"unknown":true}`), &bytes.Buffer{}); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatal("release accepted unknown JSON fields", err)
	}
	if err := run(ctx, path, "replace", strings.NewReader(`{"unknown":true}`), &bytes.Buffer{}); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatal("replace accepted unknown JSON fields", err)
	}
}
