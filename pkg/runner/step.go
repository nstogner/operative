package runner

import (
	"context"
	"fmt"
	"time"

	"log/slog"

	"github.com/mariozechner/coding-agent/session/pkg/models"
	"github.com/mariozechner/coding-agent/session/pkg/sandbox"
	"github.com/mariozechner/coding-agent/session/pkg/store"
)

func RunStep(ctx context.Context, sess store.Session, modelName string, model models.ModelProvider, sbMgr sandbox.Manager) error {
	// 0. Timeout
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second) // Increased timeout for sandbox startup
	defer cancel()

	// 1. Fetch Context
	entries, err := sess.GetContext()
	if err != nil {
		return fmt.Errorf("failed to get session context: %w", err)
	}

	if len(entries) == 0 {
		slog.Debug("No context entries found, skipping step")
		return nil
	}

	slog.Debug("Fetched session context", "count", len(entries))

	lastEntry := entries[len(entries)-1]

	// 2. Check Last State
	var lastMsg *store.MessageEntry
	if lastEntry.Message != nil {
		lastMsg = lastEntry.Message
	}

	if lastMsg == nil {
		return nil
	}

	// 3. Resolve Effective Model
	// Start with Agent's default model
	effectiveModel := sess.Header().Agent.Model
	// If CLI provided an override (modelName), we might respect it, but requirements say
	// "The session should use the model defined in the agent by default".
	// The CLI's modelName is passed in. If we implement /model, we should use the last TypeModelChange.

	for _, e := range entries {
		if e.Type == store.TypeModelChange && e.ModelChange != nil {
			effectiveModel = e.ModelChange.ModelID
		}
	}

	// If effectiveModel is still empty (no agent default, no changes), fallback to passed modelName or error
	if effectiveModel == "" {
		effectiveModel = modelName
	}

	// Log effective model
	slog.Info("Using effective model", "model", effectiveModel)

	// 4. Decide Action
	switch lastMsg.Role {
	case store.RoleUser, store.RoleTool:
		slog.Info("Calling model", "sessionID", sess.ID(), "model", effectiveModel)
		err := stepCallModel(ctx, sess, effectiveModel, model, entries)
		if err != nil {
			slog.Error("Model call failed", "error", err)
			// Report error to user
			sess.AppendMessage(store.RoleAssistant, []store.Content{{
				Type: store.ContentTypeText,
				Text: &store.TextContent{Content: fmt.Sprintf("**Error calling model:** %v", err)},
			}})
		}
		return err
	case store.RoleAssistant:
		toolCalls := extractToolCalls(lastMsg)
		if len(toolCalls) > 0 {
			return stepExecuteTools(ctx, sess, effectiveModel, model, toolCalls, sbMgr)
		}
		return nil
	default:
		slog.Debug("Skipping step: last message role is neither User nor Tool nor Assistant with tools", "role", lastMsg.Role)
		return nil
	}
}

func stepCallModel(ctx context.Context, sess store.Session, modelName string, model models.ModelProvider, entries []store.Entry) error {
	var contextMessages []models.AgentMessage

	// 1. Prepare System Prompt from Header
	agentInstructions := sess.Header().Agent.Instructions

	// Append IPython sandbox instructions
	agentInstructions += `

SYSTEM NOTE: You have access to a sandboxed IPython environment that persists throughout this session.
You can use it to accomplish tasks that would otherwise require specific tools.
For example, you can use IPython to:
- Browse the web
- Read and write files
- Run math
- Import libraries
- Call built-in functions:
  - "prompt_self(message: str)"
    - Example: Prompt yourself in the future using Python threading.
  - "prompt_model(prompt: str)"
    - Example: Prompt a model to inspect a large file and return a summary.

Whenever you have a task that will require processing a lot of text, use the IPython prompt_model function to do it,
and always return the result of that function instead of returning the raw text (to avoid token limits in your context window).
`

	// 2. Build Message History
	for _, entry := range entries {
		if entry.Message != nil {
			// Filter out old system messages if any exist in legacy sessions
			if entry.Message.Role == store.RoleSystem {
				continue
			}
			contextMessages = append(contextMessages, models.AgentMessage{
				Role:    entry.Message.Role,
				Content: entry.Message.Content,
			})
		}
	}

	// 3. Call Stream with instructions separated
	stream, err := model.Stream(ctx, modelName, agentInstructions, contextMessages)
	if err != nil {
		return fmt.Errorf("model stream error: %w", err)
	}
	defer stream.Close()

	assistantMsg, err := stream.FullMessage()
	if err != nil {
		return fmt.Errorf("model response error: %w", err)
	}

	if _, err := sess.AppendMessage(store.RoleAssistant, assistantMsg.Content); err != nil {
		return fmt.Errorf("failed to append assistant message: %w", err)
	}

	return nil
}

func stepExecuteTools(ctx context.Context, sess store.Session, modelName string, model models.ModelProvider, toolCalls []store.Content, sbMgr sandbox.Manager) error {
	for _, toolCall := range toolCalls {
		toolName := toolCall.ToolUse.Name
		var resultMsg string
		var isError bool

		if toolName == sandbox.ToolNameRunIPythonCell {
			if sbMgr == nil {
				resultMsg = "Error: Sandbox manager not available."
				isError = true
			} else {
				code, ok := toolCall.ToolUse.Input["code"].(string)
				if !ok {
					resultMsg = "Error: 'code' argument is required and must be a string."
					isError = true
				} else {
					slog.Info("Executing sandbox cell", "sessionID", sess.ID())

					delegate := &runnerDelegate{
						ctx:       ctx,
						sess:      sess,
						model:     model,
						modelName: modelName,
					}
					// Pass delegate
					res, err := sbMgr.RunCell(ctx, sess.ID(), code, delegate)
					if err != nil {
						resultMsg = fmt.Sprintf("Error executing cell: %v", err)
						slog.Error("Sandbox execution failed", "error", err)
						isError = true
					} else {
						// Format result
						if res.Output != "" {
							resultMsg = res.Output
						} else {
							// If split output or empty
							if res.Stdout != "" {
								resultMsg += "Stdout:\n" + res.Stdout + "\n"
							}
							if res.Stderr != "" {
								resultMsg += "Stderr:\n" + res.Stderr + "\n"
							}
							if resultMsg == "" {
								resultMsg = "(No output)"
							}
						}
						slog.Info("Sandbox execution successful")
					}
				}
			}
		} else {
			resultMsg = fmt.Sprintf("Error: Tool '%s' not found.", toolName)
			slog.Warn("Unknown tool called", "tool", toolName)
			isError = true
		}

		// Append Tool Result
		content := []store.Content{
			{
				Type: store.ContentTypeToolResult,
				ToolResult: &store.ToolResultContent{
					ToolUseID: toolCall.ToolUse.ID,
					IsError:   isError,
					Content:   resultMsg,
				},
			},
		}

		if _, err := sess.AppendMessage(store.RoleTool, content); err != nil {
			return fmt.Errorf("failed to append tool result: %w", err)
		}
	}
	return nil
}

// Helper to extract tool calls from a message
func extractToolCalls(msg *store.MessageEntry) []store.Content {
	var calls []store.Content
	for _, c := range msg.Content {
		if c.Type == store.ContentTypeToolUse {
			calls = append(calls, c)
		}
	}
	return calls
}

type runnerDelegate struct {
	ctx       context.Context
	sess      store.Session
	model     models.ModelProvider
	modelName string
}

func (d *runnerDelegate) PromptModel(ctx context.Context, prompt string) (string, error) {
	// Call model with the prompt
	// logic similar to stepCallModel but for a specific prompt
	// construct a minimal context with just the prompt
	messages := []models.AgentMessage{
		{
			Role: store.RoleUser,
			Content: []store.Content{
				{Type: store.ContentTypeText, Text: &store.TextContent{Content: prompt}},
			},
		},
	}
	stream, err := d.model.Stream(ctx, d.modelName, "", messages)
	if err != nil {
		return "", err
	}
	defer stream.Close()

	msg, err := stream.FullMessage()
	if err != nil {
		return "", err
	}

	// Extract text content
	if msg.Content != nil {
		for _, c := range msg.Content {
			if c.Type == store.ContentTypeText && c.Text != nil {
				return c.Text.Content, nil
			}
		}
	}
	return "", fmt.Errorf("no text response from model")
}

func (d *runnerDelegate) PromptSelf(ctx context.Context, message string) error {
	// Append a user message to the session
	// This "schedules" a future run since the runner loop will pick up the new event.
	// But wait, the runner loop triggers on events. If we append here, does it trigger immediately?
	// Yes, `sess.AppendMessage` should trigger the manager to publish an event.
	// The runner handles one event at a time.
	// If we are currently processing a step, we are in the middle of handling an event.
	// Appending a new message will create a NEW event.
	// That new event will remain in the queue (or be picked up next iteration).
	// This is exactly what we want.

	_, err := d.sess.AppendMessage(store.RoleUser, []store.Content{
		{
			Type: store.ContentTypeText,
			Text: &store.TextContent{Content: message},
		},
	})
	return err
}
