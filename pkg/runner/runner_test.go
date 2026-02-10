package runner

import (
	"context"
	"testing"
	"time"

	"github.com/mariozechner/coding-agent/session/pkg/models"
	"github.com/mariozechner/coding-agent/session/pkg/store"
	"github.com/mariozechner/coding-agent/session/pkg/store/jsonl"
)

// MockModel for testing
type MockModel struct {
	Response string
}

func (m *MockModel) List(ctx context.Context) ([]string, error) {
	return []string{"mock-model"}, nil
}

func (m *MockModel) Stream(ctx context.Context, modelName string, messages []models.AgentMessage) (models.ModelStream, error) {
	return &MockStream{
		Msg: models.AgentMessage{
			Role: store.RoleAssistant,
			Content: []store.Content{
				{Type: store.ContentTypeText, Text: &store.TextContent{Content: m.Response}},
			},
		},
	}, nil
}

type MockStream struct {
	Msg models.AgentMessage
}

func (s *MockStream) FullMessage() (models.AgentMessage, error) {
	return s.Msg, nil
}
func (s *MockStream) Close() error { return nil }

func TestRunnerIntegration(t *testing.T) {
	// 1. Setup Manager and Runner
	dir := t.TempDir()
	mgr := jsonl.NewManager(dir)

	// Create default agent for testing
	if err := mgr.NewAgent(&store.Agent{
		ID:    "default",
		Model: "mock-model",
	}); err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	mockModel := &MockModel{Response: "Response from Agent"}
	// Runner New signature: New(manager, model, modelName, sandboxManager)
	// Make sure imported runner uses 'store.Manager' now
	r := New(mgr, mockModel, "mock-model", nil)

	// 2. Start Runner in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := r.Start(ctx); err != nil && err != context.Canceled {
			t.Errorf("Runner failed: %v", err)
		}
	}()

	// Give runner time to subscribe
	time.Sleep(100 * time.Millisecond)

	// 3. Create Session and Append Message
	// Updated NewSession signature: NewSession(agentID, parentSessionID)
	sess, err := mgr.NewSession("", "")
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer sess.Close()

	_, err = sess.AppendMessage(store.RoleUser, []store.Content{
		{Type: store.ContentTypeText, Text: &store.TextContent{Content: "Hello"}},
	})
	if err != nil {
		t.Fatalf("Failed to append message: %v", err)
	}

	// 4. Wait for processing
	// We expect the Runner to see the event, call RunStep -> Model -> Append Assistant Message
	// This might happen quickly, or we might need to poll.

	timeout := time.After(2 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Fatal("Timeout waiting for agent response")
		case <-ticker.C:
			// Check if we have an assistant message
			// We need to reload the session or check the file?
			// Since we use the same manager instance which might cache or we just load strictly from disk...
			// jsonl.Session reads invalidates cache? No, it's pretty manual.
			// Let's rely on CreateBranchedSession or GetContext logic which reads from file?
			// Actually jsonl manager implementation loads entry map into memory on Load/Append.
			// To check updates made by another process (runner loading its own session struct),
			// we might need to "Reload" or just trust that Append to filesystem works.
			// But our current test holds `sess` which is an in-memory struct.
			// `Runner` Loads a *new* session struct instance.
			// So `sess` variable here won't see the updates automatically unless we refresh it or read the file.

			// Let's create a fresh handle to check
			checkSess, err := mgr.LoadSession(sess.ID())
			if err != nil {
				continue
			}

			entries, _ := checkSess.GetContext()
			checkSess.Close()

			// We expect: User Message, then Assistant Message
			if len(entries) >= 2 {
				last := entries[len(entries)-1]
				if last.Message != nil && last.Message.Role == store.RoleAssistant {
					// Success!
					return
				}
			}
		}
	}
}
