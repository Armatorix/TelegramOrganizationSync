// Command devserver is a local in-memory implementation of the
// organization server that the tos client talks to. It exposes the three
// API endpoints documented in DESIGN.md plus a small admin UI on the
// same port for poking at state from a browser.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Armatorix/TelegramOrganizationSync/internal/devserver"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "devserver:", err)
		os.Exit(1)
	}
}

func run() error {
	addr := flag.String("addr", ":8080", "listen address")
	apiKey := flag.String("api-key", envDefault("TOS_DEV_API_KEY", "dev-api-key"), "API key required by the spec endpoints")
	openapi := flag.String("openapi", "api/openapi.yaml", "path to OpenAPI spec to serve at /openapi.yaml; empty to disable")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	openapiPath := *openapi
	if openapiPath != "" {
		if _, err := os.Stat(openapiPath); err != nil {
			log.Warn("openapi spec not found; /openapi.yaml route disabled", "path", openapiPath, "err", err)
			openapiPath = ""
		}
	}

	store := devserver.NewStore()
	srv, err := devserver.New(store, *apiKey, openapiPath, log)
	if err != nil {
		return fmt.Errorf("build server: %w", err)
	}

	httpSrv := &http.Server{
		Addr:              *addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		log.Info("devserver listening", "addr", *addr, "api_key", *apiKey, "openapi", openapiPath)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		log.Info("shutting down")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		return httpSrv.Shutdown(shutdownCtx)
	}
}

func envDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
