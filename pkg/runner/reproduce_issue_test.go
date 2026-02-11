package runner_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/mariozechner/coding-agent/session/pkg/models"
	"github.com/mariozechner/coding-agent/session/pkg/runner"
	"github.com/mariozechner/coding-agent/session/pkg/store"
	"github.com/mariozechner/coding-agent/session/pkg/store/jsonl"
)

// MockModel implements models.ModelProvider
type MockModel struct {
	CapturedMessages []models.AgentMessage
}

func (m *MockModel) List(ctx context.Context) ([]string, error) {
	return []string{"mock-model"}, nil
}

func (m *MockModel) Stream(ctx context.Context, modelName string, instructions string, messages []models.AgentMessage) (models.ModelStream, error) {
	// This mock implementation is simplified for the test.
	// In a real scenario, if using `testify/mock`, MockModel would embed `mock.Mock`
	// and this method would call `m.Called(...)`.
	// For this test, we'll just capture messages and return a mock stream.
	m.CapturedMessages = messages
	return &MockStream{}, nil
}

type MockStream struct{}

func (s *MockStream) FullMessage() (models.AgentMessage, error) {
	return models.AgentMessage{
		Role: store.RoleAssistant,
		Content: []store.Content{
			{
				Type: store.ContentTypeText,
				Text: &store.TextContent{Content: "Mock Response"},
			},
		},
	}, nil
}

func (s *MockStream) Close() error { return nil }

func TestRunner_Integration(t *testing.T) {
	// 1. Setup Store
	tempDir, err := os.MkdirTemp("", "runner_repro")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	m := jsonl.NewManager(tempDir)

	// Create Agent
	if err := m.NewAgent(&store.Agent{ID: "default", Model: "mock-model"}); err != nil {
		t.Fatal(err)
	}

	// 2. Setup Runner with Mock Model
	mockModel := &MockModel{}
	r := runner.New(m, mockModel, "mock-model", nil) // sandbox nil

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start Runner
	go func() {
		r.Start(ctx)
	}()

	// 3. Create Session (this triggers NewSession event, but Runner ignores creation? No, it listens to all events)
	// Actually, NewSession does NOT trigger an event in Manager implementation we saw?
	// Let's check AppendMessage triggers it.

	sess, err := m.NewSession("default", "")
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	// 4. Append User Message
	// This should trigger the Runner to pick it up.
	msgID, err := sess.AppendMessage(store.RoleUser, []store.Content{
		{Type: store.ContentTypeText, Text: &store.TextContent{Content: "Hello"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Appended user message: %s", msgID)

	// 5. Wait for Agent Response
	// We poll the session for updates.
	deadline := time.Now().Add(5 * time.Second)
	foundResponse := false
	for time.Now().Before(deadline) {
		// Reload session or read context
		// We can reuse sess object as it reads from memory/file?
		// jsonl.Session reads from memory map, but Runner loads a NEW session instance.
		// So the in-memory map of *our* 'sess' variable won't be updated by Runner's write unless we reload or it re-reads file.
		// jsonl.Session does NOT auto-reload from file.
		// We must LoadSession again to see changes made by Runner (persisted to file).

		s2, err := m.LoadSession(sess.ID())
		if err != nil {
			t.Fatal(err)
		}

		ctx, err := s2.GetContext()
		s2.Close()
		if err != nil {
			t.Fatal(err)
		}

		if len(ctx) > 0 {
			last := ctx[len(ctx)-1]
			if last.Message != nil && last.Message.Role == store.RoleAssistant {
				if last.Message.Content[0].Text.Content == "Mock Response" {
					foundResponse = true
					break
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	if !foundResponse {
		t.Fatal("timed out waiting for agent response")
	}

	// Check if correct messages were sent to model
	if len(mockModel.CapturedMessages) == 0 {
		t.Error("Mock model verification failed: no messages captured, but we got a response? (Impossible if flow works)")
	}
}
