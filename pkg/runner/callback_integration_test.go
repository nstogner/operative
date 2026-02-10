package runner_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mariozechner/coding-agent/session/pkg/models/gemini"
	"github.com/mariozechner/coding-agent/session/pkg/runner"
	"github.com/mariozechner/coding-agent/session/pkg/sandbox/docker"
	"github.com/mariozechner/coding-agent/session/pkg/store"
	"github.com/mariozechner/coding-agent/session/pkg/store/jsonl"
)

func TestIntegration_Runner_SandboxCallbacks(t *testing.T) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		t.Skip("Skipping integration test: GEMINI_API_KEY not set")
	}

	// Longer timeout for multiple interactions
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

	go func() {
		if err := r.Start(ctx); err != nil && err != context.Canceled {
			t.Logf("Runner stopped with error: %v", err)
		}
	}()
	time.Sleep(100 * time.Millisecond)

	// Scenario 1: Prompt Self
	t.Run("PromptSelf", func(t *testing.T) {
		t.Log("Starting PromptSelf scenario...")
		sess, err := sessMgr.NewSession("", "")
		if err != nil {
			t.Fatalf("Failed to create session: %v", err)
		}
		defer sess.Close()

		// Helper to wait for a specific message content
		waitForMessage := func(target string, role store.MessageRole) bool {
			start := time.Now()
			for time.Since(start) < 60*time.Second {
				loadedSess, err := sessMgr.LoadSession(sess.ID())
				if err != nil {
					time.Sleep(1 * time.Second)
					continue
				}
				entries, _ := loadedSess.GetContext()
				loadedSess.Close()

				for _, e := range entries {
					if e.Message != nil && e.Message.Role == role {
						for _, c := range e.Message.Content {
							if c.Type == store.ContentTypeText && c.Text != nil {
								// Exact match to avoid matching the prompt instruction which contains the string
								if c.Text.Content == target {
									return true
								}
							}
						}
					}
				}
				time.Sleep(1 * time.Second)
			}
			return false
		}

		// Initial instruction
		query := "Run python code: prompt_self('Integration Test Self Prompt')"
		_, err = sess.AppendMessage(store.RoleUser, []store.Content{
			{Type: store.ContentTypeText, Text: &store.TextContent{Content: query}},
		})
		if err != nil {
			t.Fatalf("Failed to append user message: %v", err)
		}

		// We expect the model to call run_ipython_cell, then python to call prompt_self,
		// which appends a USER message "Integration Test Self Prompt"

		t.Log("Waiting for self-prompted message...")
		if !waitForMessage("Integration Test Self Prompt", store.RoleUser) {
			t.Fatal("Timed out waiting for prompt_self message")
		}
		t.Log("SUCCESS: Found prompt_self message")
	})

	// Scenario 2: Prompt Model
	t.Run("PromptModel", func(t *testing.T) {
		t.Log("Starting PromptModel scenario...")
		sess, err := sessMgr.NewSession("", "")
		if err != nil {
			t.Fatalf("Failed to create session: %v", err)
		}
		defer sess.Close()

		// Initial instruction
		// We ask it to print the result so we can see it in the tool output.
		// We strictly instruct it that prompt_model is available in the python environment.
		query := "Run python code to call the `prompt_model` function. Use this code: `print(prompt_model('Reply with only the word: SQUEAMISH'))`. The `prompt_model` function is already defined in the environment."

		_, err = sess.AppendMessage(store.RoleUser, []store.Content{
			{Type: store.ContentTypeText, Text: &store.TextContent{Content: query}},
		})
		if err != nil {
			t.Fatalf("Failed to append user message: %v", err)
		}

		// We expect:
		// 1. Model calls run_ipython_cell
		// 2. Python calls prompt_model("... SQUEAMISH ...")
		// 3. Go calls Model
		// 4. Model returns "SQUEAMISH"
		// 5. Python prints "SQUEAMISH"
		// 6. Tool Result contains "SQUEAMISH"

		t.Log("Waiting for tool output with SQUEAMISH...")
		found := false
		start := time.Now()
		for time.Since(start) < 60*time.Second {
			loadedSess, err := sessMgr.LoadSession(sess.ID())
			if err != nil {
				time.Sleep(1 * time.Second)
				continue
			}
			entries, _ := loadedSess.GetContext()
			loadedSess.Close()

			for _, e := range entries {
				// Look for ToolResult (RoleTool) containing SQUEAMISH
				if e.Message != nil && e.Message.Role == store.RoleTool {
					for _, c := range e.Message.Content {
						if c.Type == store.ContentTypeToolResult && c.ToolResult != nil {
							if strings.Contains(c.ToolResult.Content, "SQUEAMISH") {
								found = true
								break
							}
						}
					}
				}
			}
			if found {
				break
			}
			time.Sleep(1 * time.Second)
		}

		if !found {
			t.Fatal("Timed out waiting for prompt_model result 'SQUEAMISH'")
		}
		t.Log("SUCCESS: Found prompt_model response in tool output")
	})
}
