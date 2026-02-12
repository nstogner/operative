package main

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/nstogner/operative/pkg/controller"
	"github.com/nstogner/operative/pkg/model/gemini"
	"github.com/nstogner/operative/pkg/sandbox/docker"
	"github.com/nstogner/operative/pkg/server"
	"github.com/nstogner/operative/pkg/store/sqlite"
	"github.com/nstogner/operative/web"
)

func main() {
	// Setup logger.
	opts := &slog.HandlerOptions{Level: slog.LevelDebug}
	logger := slog.New(slog.NewTextHandler(os.Stderr, opts))
	slog.SetDefault(logger)

	// Config.
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		slog.Error("GEMINI_API_KEY environment variable not set")
		os.Exit(1)
	}

	ctx := context.Background()

	// Initialize store.
	wd, _ := os.Getwd()
	dbPath := filepath.Join(wd, "data", "operative.db")
	os.MkdirAll(filepath.Dir(dbPath), 0755)

	store, err := sqlite.New(dbPath)
	if err != nil {
		slog.Error("Failed to initialize store", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	// Initialize model provider.
	provider, err := gemini.New(ctx, apiKey)
	if err != nil {
		slog.Error("Failed to initialize Gemini provider", "error", err)
		os.Exit(1)
	}

	// Initialize sandbox manager.
	sbMgr, err := docker.New()
	if err != nil {
		slog.Error("Failed to initialize sandbox manager", "error", err)
		os.Exit(1)
	}
	defer sbMgr.Close()

	// Start sandbox reconciliation loop in background.
	// This keeps containers in sync with known operatives.
	go func() {
		if err := sbMgr.Run(ctx, store); err != nil {
			slog.Error("Sandbox manager stopped", "error", err)
		}
	}()

	// Initialize controller.
	ctrl := controller.New(store, store, store, provider, sbMgr)

	// Start controller in background.
	go func() {
		if err := ctrl.Start(ctx); err != nil {
			slog.Error("Controller stopped unexpectedly", "error", err)
		}
	}()

	// Start server.
	srv := server.New(store, store, store, provider, sbMgr, web.DistFS)
	if err := srv.Start(":8080"); err != nil {
		slog.Error("Server failed", "error", err)
		os.Exit(1)
	}
}
