// corerp-admin is a local filesystem-privileged configuration tool, not a server.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/storage"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, out io.Writer) error {
	f := flag.NewFlagSet("corerp-admin", flag.ContinueOnError)
	db := f.String("db", "", "existing SQLite file (local administrator only)")
	actor := f.String("operator", "", "existing active operator principal ID")
	instance := f.String("instance", "", "exact world instance")
	branch := f.String("branch", "", "exact branch")
	target := f.String("target", "", "existing creator/operator principal ID")
	status := f.String("status", "", "active or revoked")
	explain := f.Bool("explain", false, "explicitly include bounded diagnostic/observation explanation reads")
	purpose := f.String("purpose", "", "empty for inspector access; create_world for independent creator-only genesis permission")
	head := f.Int64("expected-head", -1, "expected current branch head")
	key := f.String("key", "", "durable idempotency key; preserve exact flags on retry")
	if err := f.Parse(args); err != nil {
		return err
	}
	r := storage.StudioAccessRequest{Purpose: *purpose, Explain: *explain, Binding: core.CareerBinding{PrincipalID: *actor, InstanceID: *instance, BranchID: *branch, ExpectedHead: *head, IdempotencyKey: *key}, TargetPrincipalID: *target, Status: *status}
	if err := r.Binding.Validate(); err != nil {
		return err
	}
	if f.NArg() != 0 || *db == "" || *target == "" || (*status != "active" && *status != "revoked") {
		return fmt.Errorf("explicit database, target, status and binding flags required")
	}
	info, err := os.Stat(*db)
	if err != nil {
		return fmt.Errorf("existing database required: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("database must be a regular file")
	}
	s, err := storage.Open(ctx, *db)
	if err != nil {
		return err
	}
	defer s.Close()
	result, err := s.ConfigureStudioAccessLocal(ctx, r)
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(result)
}
