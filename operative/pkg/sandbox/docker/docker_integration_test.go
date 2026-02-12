package docker

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// stripOut strips IPython's "Out[N]: " prefix from output.
var outPrefix = regexp.MustCompile(`^Out\[\d+\]: `)

func stripOut(s string) string {
	return strings.TrimSpace(outPrefix.ReplaceAllString(strings.TrimSpace(s), ""))
}

// stubDelegate is a no-op implementation of sandbox.Delegate for tests.
type stubDelegate struct{}

func (d *stubDelegate) PromptModel(ctx context.Context, prompt string) (string, error) {
	return "stubbed response", nil
}
func (d *stubDelegate) PromptSelf(ctx context.Context, message string) error {
	return nil
}

const testOperativeID = "integration-test-operative"

// staticLister implements sandbox.OperativeLister with a fixed list of IDs.
type staticLister struct {
	ids []string
}

func (l *staticLister) ListIDs(ctx context.Context) ([]string, error) {
	return l.ids, nil
}

// setupManagerAndRun creates a Docker Manager, starts the Run loop with the
// test operative, waits for the sandbox to be fully ready (gRPC reachable),
// and returns the manager.
func setupManagerAndRun(t *testing.T) (*Manager, context.CancelFunc) {
	t.Helper()
	mgr, err := New()
	if err != nil {
		t.Skipf("Docker not available, skipping integration test: %v", err)
	}

	// Quick check that Docker daemon is responsive.
	pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer pingCancel()
	_, err = mgr.Status(pingCtx, "ping-check")
	if err != nil {
		mgr.Close()
		t.Skipf("Docker daemon not responsive: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Start the Run loop with our test operative.
	lister := &staticLister{ids: []string{testOperativeID}}
	go func() {
		if err := mgr.Run(ctx, lister); err != nil && ctx.Err() == nil {
			t.Logf("Run loop error: %v", err)
		}
	}()

	// Wait for sandbox to be FULLY ready:
	// 1. Docker container state == "running"
	// 2. gRPC port is reachable
	deadline := time.After(120 * time.Second)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			cancel()
			mgr.Close()
			t.Fatalf("Timed out waiting for sandbox to start")
		case <-ticker.C:
			// Check Docker state first.
			status, _ := mgr.Status(context.Background(), testOperativeID)
			if status != "running" {
				continue
			}
			// Docker says running — verify gRPC is actually reachable.
			port, err := mgr.getRunningPort(context.Background(), testOperativeID)
			if err != nil {
				continue
			}
			dialCtx, dialCancel := context.WithTimeout(context.Background(), 1*time.Second)
			conn, err := grpc.DialContext(dialCtx, fmt.Sprintf("127.0.0.1:%s", port),
				grpc.WithTransportCredentials(insecure.NewCredentials()),
				grpc.WithBlock(),
			)
			dialCancel()
			if err != nil {
				continue
			}
			conn.Close()
			return mgr, cancel
		}
	}
}

func cleanupManager(mgr *Manager, cancel context.CancelFunc, t *testing.T) {
	t.Helper()
	// Cancel the Run loop first — this stops reconciliation.
	cancel()
	// Stop the test container.
	ctx, c := context.WithTimeout(context.Background(), 30*time.Second)
	defer c()
	mgr.stopContainer(ctx, testOperativeID)
	mgr.Close()
}

// TestIntegrationRunCellExpression verifies that a bare expression (like
// IPython) produces output. This is the core bug fix: "9*9" must print "81".
func TestIntegrationRunCellExpression(t *testing.T) {
	mgr, cancel := setupManagerAndRun(t)
	defer cleanupManager(mgr, cancel, t)

	ctx, c := context.WithTimeout(context.Background(), 30*time.Second)
	defer c()

	result, err := mgr.RunCell(ctx, testOperativeID, "9*9", &stubDelegate{})
	if err != nil {
		t.Fatalf("RunCell: %v", err)
	}

	output := stripOut(result.Output)
	if output != "81" {
		t.Errorf("expected output %q, got %q", "81", output)
	}
}

// TestIntegrationRunCellPrint verifies explicit print() still works.
func TestIntegrationRunCellPrint(t *testing.T) {
	mgr, cancel := setupManagerAndRun(t)
	defer cleanupManager(mgr, cancel, t)

	ctx, c := context.WithTimeout(context.Background(), 30*time.Second)
	defer c()

	result, err := mgr.RunCell(ctx, testOperativeID, `print("hello world")`, &stubDelegate{})
	if err != nil {
		t.Fatalf("RunCell: %v", err)
	}

	output := strings.TrimSpace(result.Output)
	if output != "hello world" {
		t.Errorf("expected output %q, got %q", "hello world", output)
	}
}

// TestIntegrationRunCellAssignment verifies that an assignment produces no output.
func TestIntegrationRunCellAssignment(t *testing.T) {
	mgr, cancel := setupManagerAndRun(t)
	defer cleanupManager(mgr, cancel, t)

	ctx, c := context.WithTimeout(context.Background(), 30*time.Second)
	defer c()

	result, err := mgr.RunCell(ctx, testOperativeID, "x = 42", &stubDelegate{})
	if err != nil {
		t.Fatalf("RunCell: %v", err)
	}

	output := strings.TrimSpace(result.Output)
	if output != "" {
		t.Errorf("expected no output for assignment, got %q", output)
	}
}

// TestIntegrationRunCellMultiLine verifies multi-line code where the last
// line is an expression. IPython auto-displays the last expression result.
func TestIntegrationRunCellMultiLine(t *testing.T) {
	mgr, cancel := setupManagerAndRun(t)
	defer cleanupManager(mgr, cancel, t)

	ctx, c := context.WithTimeout(context.Background(), 30*time.Second)
	defer c()

	code := "x = 5\ny = 10\nx + y"
	result, err := mgr.RunCell(ctx, testOperativeID, code, &stubDelegate{})
	if err != nil {
		t.Fatalf("RunCell: %v", err)
	}

	output := stripOut(result.Output)
	if output != "15" {
		t.Errorf("expected output %q, got %q", "15", output)
	}
}

// TestIntegrationRunCellError verifies that runtime errors are reported.
func TestIntegrationRunCellError(t *testing.T) {
	mgr, cancel := setupManagerAndRun(t)
	defer cleanupManager(mgr, cancel, t)

	ctx, c := context.WithTimeout(context.Background(), 30*time.Second)
	defer c()

	result, err := mgr.RunCell(ctx, testOperativeID, "1/0", &stubDelegate{})
	if err != nil {
		t.Fatalf("RunCell: %v", err)
	}

	output := result.Output + result.Stderr
	if !strings.Contains(output, "ZeroDivisionError") {
		t.Errorf("expected ZeroDivisionError in output, got %q", output)
	}
}

// TestIntegrationRunCellNotRunning verifies that RunCell returns an error
// if the sandbox container is not running.
func TestIntegrationRunCellNotRunning(t *testing.T) {
	mgr, err := New()
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}
	defer mgr.Close()

	ctx, c := context.WithTimeout(context.Background(), 10*time.Second)
	defer c()

	_, err = mgr.RunCell(ctx, "nonexistent-operative", "1+1", &stubDelegate{})
	if err == nil {
		t.Fatal("expected error for non-running sandbox, got nil")
	}
	if !strings.Contains(err.Error(), "sandbox not running") {
		t.Errorf("expected 'sandbox not running' error, got: %v", err)
	}
}

// TestIntegrationSandboxStatus verifies Status returns correct states.
func TestIntegrationSandboxStatus(t *testing.T) {
	mgr, err := New()
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}
	defer mgr.Close()

	ctx, c := context.WithTimeout(context.Background(), 10*time.Second)
	defer c()

	// Non-existent operative should be "stopped".
	status, err := mgr.Status(ctx, "nonexistent-operative")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status != "stopped" {
		t.Errorf("expected status 'stopped', got %q", status)
	}
}

// TestIntegrationRunCellPromptModel verifies that sandbox code calling
// prompt_model() triggers the delegate and returns a response.
func TestIntegrationRunCellPromptModel(t *testing.T) {
	mgr, cancel := setupManagerAndRun(t)
	defer cleanupManager(mgr, cancel, t)

	ctx, c := context.WithTimeout(context.Background(), 30*time.Second)
	defer c()

	code := `result = prompt_model("What is 1+1?")
print(f"Model says: {result}")`

	var promptReceived bool
	delegate := &recordingDelegate{
		promptModelFn: func(ctx context.Context, prompt string) (string, error) {
			promptReceived = true
			return "2", nil
		},
	}

	result, err := mgr.RunCell(ctx, testOperativeID, code, delegate)
	if err != nil {
		t.Fatalf("RunCell: %v", err)
	}

	if !promptReceived {
		t.Error("expected delegate.PromptModel to be called")
	}

	output := strings.TrimSpace(result.Output)
	if !strings.Contains(output, "Model says: 2") {
		t.Errorf("expected 'Model says: 2' in output, got %q", output)
	}
}

// recordingDelegate records calls to delegate methods for assertions.
type recordingDelegate struct {
	promptModelFn func(ctx context.Context, prompt string) (string, error)
}

func (d *recordingDelegate) PromptModel(ctx context.Context, prompt string) (string, error) {
	if d.promptModelFn != nil {
		return d.promptModelFn(ctx, prompt)
	}
	return "stubbed", nil
}

func (d *recordingDelegate) PromptSelf(ctx context.Context, message string) error {
	return nil
}
