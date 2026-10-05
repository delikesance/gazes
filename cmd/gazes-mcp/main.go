// gazes-mcp serves the Gazes admin MCP server on stdin/stdout for local use. It opens the local
// databases directly, authenticates with the token in GAZES_MCP_TOKEN (same scopes as over HTTP)
// and never writes anything but the protocol to stdout; logs go to stderr.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/gazes/gazes/internal/admin"
	"github.com/gazes/gazes/internal/config"
	"github.com/gazes/gazes/internal/mcp"
	"github.com/go-chi/chi/v5"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gazes-mcp:", err)
		os.Exit(1)
	}
}

func run() error {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	plain := os.Getenv("GAZES_MCP_TOKEN")
	if plain == "" {
		return fmt.Errorf("GAZES_MCP_TOKEN is not set (create one with: gazes-admin token create)")
	}
	cfg := config.Load()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	store, err := admin.Open(cfg.AdminDBPath)
	if err != nil {
		return fmt.Errorf("open admin database: %w", err)
	}
	defer store.Close()
	svc, err := admin.NewService(store, filepath.Join(cfg.AccountsDir, "accounts.sqlite"))
	if err != nil {
		return fmt.Errorf("open accounts database: %w", err)
	}
	defer svc.Close()

	cred, err := mcp.VerifyStatic(ctx, store, plain)
	if err != nil {
		return fmt.Errorf("token refused (unknown, expired or revoked)")
	}
	router := chi.NewRouter()
	svc.Mount(router)
	srv := mcp.New(mcp.Config{Admin: router, Tokens: store, Audit: store, Logger: logger}, cred)
	return srv.RunStdio(ctx)
}
