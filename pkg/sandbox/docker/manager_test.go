package docker_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mariozechner/coding-agent/session/pkg/sandbox/docker"
)

func TestDockerManager_RunCell(t *testing.T) {
	// Skip if docker is not available or if running in CI without docker
	// For this environment, we assume docker (colima) is available as per user note

	mgr, err := docker.New()
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer mgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	sessionID := uuid.New().String()
	// Cleanup after test
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		mgr.Stop(cleanupCtx, sessionID)
	}()

	t.Logf("Running cell in session %s...", sessionID)

	// First run (should trigger cold start)
	code := "print('Hello, World!')"
	res, err := mgr.RunCell(ctx, sessionID, code)
	if err != nil {
		t.Fatalf("RunCell failed: %v", err)
	}

	if res.Output == "" && res.Stdout == "" {
		t.Errorf("Expected output, got empty")
	}
	t.Logf("Result: %+v", res)

	// Second run (warm)
	code2 := "x = 10\nx * 2"
	res2, err := mgr.RunCell(ctx, sessionID, code2)
	if err != nil {
		t.Fatalf("RunCell 2 failed: %v", err)
	}
	t.Logf("Result 2: %+v", res2)
}
