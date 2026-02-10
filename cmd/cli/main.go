package main

import (
	"context"
	"log/slog"
	"os"
	"path"

	"github.com/mariozechner/coding-agent/session/pkg/models"
	"github.com/mariozechner/coding-agent/session/pkg/models/gemini"
	"github.com/mariozechner/coding-agent/session/pkg/runner"
	"github.com/mariozechner/coding-agent/session/pkg/sandbox/docker"
	"github.com/mariozechner/coding-agent/session/pkg/server"
	"github.com/mariozechner/coding-agent/session/pkg/store/jsonl"
	"github.com/mariozechner/coding-agent/session/web"
)

func main() {
	// 1. Setup Logger
	opts := &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, opts))
	slog.SetDefault(logger)

	// 2. Config
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		slog.Error("GEMINI_API_KEY environment variable not set")
		os.Exit(1)
	}

	ctx := context.Background()

	// 3. Initialize Components
	// Manager
	wd, _ := os.Getwd()
	manager := jsonl.NewManager(path.Join(wd, "store"))

	// Model Provider
	var provider models.ModelProvider
	provider, err := gemini.New(ctx, apiKey)
	if err != nil {
		slog.Error("Failed to initialize Gemini provider", "error", err)
		os.Exit(1)
	}

	// Sandbox
	sbMgr, err := docker.New()
	if err != nil {
		slog.Error("Failed to initialize sandbox manager", "error", err)
		os.Exit(1)
	}

	// Runner
	defaultModel := "gemini-1.5-pro-latest"
	agentRunner := runner.New(manager, provider, defaultModel, sbMgr)

	// Start Runner
	go func() {
		if err := agentRunner.Start(ctx); err != nil {
			slog.Error("Runner stopped unexpectedy", "error", err)
		}
	}()

	// 4. Start Server
	// Create FS for dist
	// web.DistFS contains "dist/index.html", etc.
	// We can pass web.DistFS directly.

	srv := server.New(manager, provider, web.DistFS)

	if err := srv.Start(":8080"); err != nil {
		slog.Error("Server failed", "error", err)
		os.Exit(1)
	}
}
