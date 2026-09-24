package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/decision"
	"corerp.local/backend/internal/narrative"
	"corerp.local/backend/internal/storage"
	"corerp.local/backend/internal/transport/httpapi"
)

const (
	tokenEnvironment  = "CORERP_AUTH_TOKENS_JSON"
	cursorEnvironment = "CORERP_CURSOR_SECRET"
)

func main() {
	databasePath := flag.String("db", "", "required SQLite database path")
	listenAddress := flag.String("listen", "127.0.0.1:8080", "HTTP listen address")
	browserOrigins := flag.String("browser-origins", "", "optional comma-separated exact browser origins allowed to access the authenticated API")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	provider, mode, err := decision.FromEnvironment(os.Getenv)
	if err != nil {
		logger.Error("invalid decision provider configuration", "error", err)
		os.Exit(1)
	}
	narrator, _, err := narrative.FromEnvironment(os.Getenv)
	if err != nil {
		logger.Error("invalid narrative provider configuration", "error", err)
		os.Exit(1)
	}
	var origins []string
	if *browserOrigins != "" {
		origins = strings.Split(*browserOrigins, ",")
	}
	if err := runWithProviders(ctx, *databasePath, *listenAddress, os.Getenv(tokenEnvironment), os.Getenv(cursorEnvironment), logger, provider, mode, narrator, origins...); err != nil {
		logger.Error("server stopped with error", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, databasePath, listenAddress, tokenJSON, cursorSecret string, logger *slog.Logger) error {
	return runWithProvider(ctx, databasePath, listenAddress, tokenJSON, cursorSecret, logger, core.DeterministicRPDecisionProvider{}, "deterministic")
}

func runWithProvider(ctx context.Context, databasePath, listenAddress, tokenJSON, cursorSecret string, logger *slog.Logger, provider core.RPDecisionProvider, mode string) error {
	return runWithProviders(ctx, databasePath, listenAddress, tokenJSON, cursorSecret, logger, provider, mode, core.DeterministicRPNarrativeProvider{})
}

func runWithProviders(ctx context.Context, databasePath, listenAddress, tokenJSON, cursorSecret string, logger *slog.Logger, provider core.RPDecisionProvider, mode string, narrator core.RPStreamingNarrativeProvider, browserOrigins ...string) error {
	if strings.TrimSpace(databasePath) == "" {
		return core.NewError(core.CodeInvalidArgument, "-db is required")
	}
	if strings.TrimSpace(listenAddress) == "" {
		return core.NewError(core.CodeInvalidArgument, "-listen is required")
	}
	if logger == nil {
		return core.NewError(core.CodeInvalidArgument, "logger is required")
	}
	originPolicy, err := httpapi.BrowserOriginPolicy(browserOrigins)
	if err != nil {
		return err
	}
	tokens, err := parseTokenConfiguration(tokenJSON)
	if err != nil {
		return err
	}
	authenticator, err := httpapi.NewStaticTokenAuthenticator(tokens)
	if err != nil {
		return err
	}
	cursors, err := httpapi.NewCursorCodec([]byte(cursorSecret))
	if err != nil {
		return core.WrapError(core.CodeInvalidArgument, cursorEnvironment+" is invalid", err)
	}
	store, err := storage.Open(ctx, databasePath)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.BootstrapDemo(ctx); err != nil {
		return err
	}
	service, err := storage.NewRPServiceWithNarrative(store, provider, mode, narrator)
	if err != nil {
		return err
	}
	api, err := httpapi.New(service, authenticator, cursors)
	if err != nil {
		return err
	}
	server := &http.Server{
		Addr:              listenAddress,
		Handler:           originPolicy(api.Handler()),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		// Streaming responses install bounded per-write deadlines. A global
		// WriteTimeout would terminate healthy SSE subscriptions.
		WriteTimeout:   0,
		IdleTimeout:    60 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	serveErrors := make(chan error, 1)
	go func() {
		logger.Info("CoreRP HTTP API listening", "address", listenAddress)
		serveErrors <- server.ListenAndServe()
	}()
	select {
	case err := <-serveErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return core.WrapError(core.CodeStorageFailure, "serve HTTP API", err)
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownContext); err != nil {
			return core.WrapError(core.CodeStorageFailure, "gracefully shut down HTTP API", err)
		}
		err := <-serveErrors
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return core.WrapError(core.CodeStorageFailure, "finish HTTP API shutdown", err)
		}
		logger.Info("CoreRP HTTP API stopped")
		return nil
	}
}

func parseTokenConfiguration(raw string) (map[string]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, core.NewError(core.CodeInvalidArgument, tokenEnvironment+" is required")
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	var tokens map[string]string
	if err := decoder.Decode(&tokens); err != nil {
		return nil, core.WrapError(core.CodeInvalidArgument, tokenEnvironment+" must be a JSON object mapping credentials to principal IDs", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, core.NewError(core.CodeInvalidArgument, tokenEnvironment+" must contain exactly one JSON object")
	}
	if len(tokens) == 0 {
		return nil, core.NewError(core.CodeInvalidArgument, tokenEnvironment+" must contain at least one credential")
	}
	for token, principalID := range tokens {
		if strings.TrimSpace(token) == "" || strings.TrimSpace(principalID) == "" {
			return nil, core.NewError(core.CodeInvalidArgument, fmt.Sprintf("%s contains an empty credential or principal", tokenEnvironment))
		}
	}
	return tokens, nil
}
