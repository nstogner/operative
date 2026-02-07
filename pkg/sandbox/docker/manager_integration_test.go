package docker_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mariozechner/coding-agent/session/pkg/sandbox/docker"
)

type mockDelegate struct {
	promptModelCalled bool
	promptSelfCalled  bool
}

func (m *mockDelegate) PromptModel(ctx context.Context, prompt string) (string, error) {
	m.promptModelCalled = true
	if prompt == "ping" {
		return "pong", nil
	}
	return "mock response", nil
}
func (m *mockDelegate) PromptSelf(ctx context.Context, message string) error {
	m.promptSelfCalled = true
	return nil
}

func TestIntegration_DockerManager_RunCell(t *testing.T) {
	// Skip if docker is not available or if running in CI without docker
	// For this environment, we assume docker (colima) is available as per user note

	// Check if DOCKER_HOST is set. If not, we skip.
	if os.Getenv("DOCKER_HOST") == "" {
		t.Skip("Skipping integration test: DOCKER_HOST not set")
	}

	mgr, err := docker.New()
	if err != nil {
		t.Skipf("Skipping test: Docker not available or failed to init: %v", err)
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

	delegate := &mockDelegate{}

	// First run (should trigger cold start)
	code := "print('Hello, World!')"
	res, err := mgr.RunCell(ctx, sessionID, code, delegate)
	if err != nil {
		t.Fatalf("RunCell failed: %v", err)
	}

	if res.Output == "" && res.Stdout == "" {
		t.Errorf("Expected output, got empty")
	}
	t.Logf("Result: %+v", res)

	// Second run (warm)
	code2 := "x = 10\nx * 2"
	res2, err := mgr.RunCell(ctx, sessionID, code2, delegate)
	if err != nil {
		t.Fatalf("RunCell 2 failed: %v", err)
	}
	t.Logf("Result 2: %+v", res2)

	// Third run: prompt_model
	code3 := "resp = prompt_model('ping')\nprint(resp)"
	res3, err := mgr.RunCell(ctx, sessionID, code3, delegate)
	if err != nil {
		t.Fatalf("RunCell 3 failed: %v", err)
	}
	t.Logf("Result 3: %+v", res3)
	if !delegate.promptModelCalled {
		t.Error("Expected promptModel to be called")
	}
	// The output might contain the print result "pong\n"
	if res3.Stdout != "pong\n" {
		// It might have other things if ipython is noisy, but usually it's just the print
		// Let's check contains if exact match fails, or just strict for now
		if res3.Stdout != "pong\n" {
			t.Errorf("Expected output 'pong\\n', got '%s'", res3.Stdout)
		}
	}

	// Fourth run: prompt_self
	code4 := "prompt_self('hello self')"
	_, err = mgr.RunCell(ctx, sessionID, code4, delegate)
	if err != nil {
		t.Fatalf("RunCell 4 failed: %v", err)
	}
	if !delegate.promptSelfCalled {
		t.Error("Expected promptSelf to be called")
	}
}
