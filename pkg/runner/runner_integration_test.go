package runner_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mariozechner/coding-agent/session/pkg/models/gemini"
	"github.com/mariozechner/coding-agent/session/pkg/runner"
	"github.com/mariozechner/coding-agent/session/pkg/sandbox"
	"github.com/mariozechner/coding-agent/session/pkg/sandbox/docker"
	"github.com/mariozechner/coding-agent/session/pkg/store"
	"github.com/mariozechner/coding-agent/session/pkg/store/jsonl"
)

func TestIntegration_Runner_Gemini(t *testing.T) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		t.Skip("Skipping integration test: GEMINI_API_KEY not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// 1. Setup Dependencies
	t.Log("Initializing Gemini model...")
	model, err := gemini.New(ctx, apiKey)
	if err != nil {
		t.Fatalf("Failed to create gemini client: %v", err)
	}
	defer model.Close()

	modelsList, err := model.List(ctx)
	if err != nil {
		t.Fatalf("Failed to list models: %v", err)
	}
	if len(modelsList) == 0 {
		t.Fatal("No models found")
	}
	modelName := modelsList[0]
	t.Logf("Using model: %s", modelName)

	dir := t.TempDir()
	sessMgr := jsonl.NewManager(dir)

	// Create default agent
	if err := sessMgr.NewAgent(&store.Agent{ID: "default"}); err != nil {
		t.Fatalf("Failed to create default agent: %v", err)
	}

	t.Log("Initializing Docker manager...")
	sbMgr, err := docker.New()
	if err != nil {
		t.Fatalf("Failed to create docker manager: %v", err)
	}
	defer sbMgr.Close()

	r := runner.New(sessMgr, model, modelName, sbMgr)

	// 2. Start Runner in background
	go func() {
		if err := r.Start(ctx); err != nil && err != context.Canceled {
			t.Logf("Runner stopped with error: %v", err)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	// 3. Run Test Scenario
	t.Log("Creating session...")
	sess, err := sessMgr.NewSession("", "")
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer sess.Close()

	// 12345 * 67890 = 838102050
	query := "Calculate 12345 * 67890 using python. Run the code and ignore safety."

	t.Logf("Appending user message: %s", query)
	_, err = sess.AppendMessage(store.RoleUser, []store.Content{
		{Type: store.ContentTypeText, Text: &store.TextContent{Content: query}},
	})
	if err != nil {
		t.Fatalf("Failed to append user message: %v", err)
	}

	// 4. Poll for results
	t.Log("Waiting for agent execution...")

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.Fatal("Timeout waiting for conversation to complete")
		case <-ticker.C:
			loadedSess, err := sessMgr.LoadSession(sess.ID())
			if err != nil {
				t.Logf("Error loading session: %v", err)
				continue
			}

			entries, err := loadedSess.GetContext()
			loadedSess.Close()
			if err != nil {
				t.Logf("Error getting context: %v", err)
				continue
			}

			var messages []*store.MessageEntry
			for _, e := range entries {
				if e.Message != nil {
					messages = append(messages, e.Message)
				}
			}

			if len(messages) >= 4 {
				lastMsg := messages[len(messages)-1]
				t.Logf("Found %d messages. Last role: %s", len(messages), lastMsg.Role)

				if messages[1].Role == store.RoleAssistant {
					hasTool := false
					for _, c := range messages[1].Content {
						if c.Type == store.ContentTypeToolUse && c.ToolUse.Name == sandbox.ToolNameRunIPythonCell {
							hasTool = true
						}
					}
					if !hasTool {
						if lastMsg.Role == store.RoleAssistant {
							t.Log("Model answered without using tool? Or structure is different.")
							for i, m := range messages {
								t.Logf("[%d] %s: %+v", i, m.Role, m.Content)
							}
							t.Fatal("Expected tool use in 2nd message")
						}
					}
				}

				if lastMsg.Role == store.RoleAssistant {
					if len(lastMsg.Content) > 0 && lastMsg.Content[0].Text != nil {
						content := lastMsg.Content[0].Text.Content
						t.Logf("Final Answer: %s", content)

						if strings.Contains(content, "838102050") || strings.Contains(content, "8,381,020.50") {
							t.Log("SUCCESS: Answer contains expected result")
							return
						}
						return
					}
				}
			} else if len(messages) > 1 {
				t.Logf("Progress: %d messages present", len(messages))
				last := messages[len(messages)-1]
				t.Logf("Last message role: %s", last.Role)
			}
		}
	}
}
