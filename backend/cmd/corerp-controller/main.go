// corerp-controller is a local operator command for sourced controller
// enrollment, assignment, explicit release and replacement. It never provisions credentials.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/storage"
)

func main() {
	database := flag.String("db", "", "existing CoreRP SQLite database path")
	action := flag.String("action", "", "local operator action: enroll, assign, release or replace")
	flag.Parse()
	if *database == "" || (*action != "enroll" && *action != "assign" && *action != "release" && *action != "replace") || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: corerp-controller -db PATH -action enroll|assign|release|replace < request.json")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, *database, *action, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, database, action string, input io.Reader, output io.Writer) error {
	if database == "" || (action != "enroll" && action != "assign" && action != "release" && action != "replace") {
		return core.NewError(core.CodeInvalidArgument, "existing DB path and enroll/assign/release/replace action required")
	}
	info, err := os.Stat(database)
	if err != nil || !info.Mode().IsRegular() {
		return core.NewError(core.CodeInvalidArgument, "controller command requires an existing regular database file")
	}
	store, err := storage.Open(ctx, database)
	if err != nil {
		return err
	}
	defer store.Close()
	decoder := json.NewDecoder(io.LimitReader(input, 1<<20))
	decoder.DisallowUnknownFields()
	encoder := json.NewEncoder(output)
	switch action {
	case "enroll":
		var request storage.RPExternalControllerEnrollmentRequest
		if err := decodeOne(decoder, &request); err != nil {
			return err
		}
		result, err := store.EnrollRPExternalControllerLocal(ctx, request)
		if err != nil {
			return err
		}
		return encoder.Encode(result)
	case "assign":
		var request storage.RPExternalControllerAssignmentRequest
		if err := decodeOne(decoder, &request); err != nil {
			return err
		}
		result, err := store.AssignRPExternalControllerLocal(ctx, request)
		if err != nil {
			return err
		}
		return encoder.Encode(result)
	case "release":
		var request storage.RPExternalControllerReleaseRequest
		if err := decodeOne(decoder, &request); err != nil {
			return err
		}
		result, err := store.ReleaseRPExternalControllerLocal(ctx, request)
		if err != nil {
			return err
		}
		return encoder.Encode(result)
	case "replace":
		var request storage.RPExternalControllerReplacementRequest
		if err := decodeOne(decoder, &request); err != nil {
			return err
		}
		result, err := store.ReplaceRPExternalControllerLocal(ctx, request)
		if err != nil {
			return err
		}
		return encoder.Encode(result)
	default:
		return core.NewError(core.CodeInvalidArgument, "unknown local controller action")
	}
}

func decodeOne(decoder *json.Decoder, destination any) error {
	if err := decoder.Decode(destination); err != nil {
		return core.WrapError(core.CodeInvalidArgument, "controller request must be one JSON object", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return core.NewError(core.CodeInvalidArgument, "controller request contains a second JSON value")
		}
		return core.WrapError(core.CodeInvalidArgument, "controller request contains trailing data", err)
	}
	return nil
}
