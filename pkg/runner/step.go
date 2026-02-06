package runner

import (
	"context"
	"fmt"

	"log/slog"

	"github.com/mariozechner/coding-agent/session/pkg/models"
	"github.com/mariozechner/coding-agent/session/pkg/sandbox"
	"github.com/mariozechner/coding-agent/session/pkg/session"
)

// RunStep performs a single step of the agent's logic based on the session state.
// It fetches context, decides whether to call the model or execute a tool, and appends the result to the session.
func RunStep(ctx context.Context, sess session.Session, modelName string, model models.ModelProvider, sbMgr sandbox.Manager) error {
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
	var lastMsg *session.MessageEntry
	if lastEntry.Message != nil {
		lastMsg = lastEntry.Message
	}

	if lastMsg == nil {
		return nil
	}

	// 3. Decide Action
	switch lastMsg.Role {
	case session.RoleUser, session.RoleTool:
		slog.Info("Calling model", "sessionID", sess.ID())
		err := stepCallModel(ctx, sess, modelName, model, entries)
		if err != nil {
			slog.Error("Model call failed", "error", err)
		}
		return err
	case session.RoleAssistant:
		toolCalls := extractToolCalls(lastMsg)
		if len(toolCalls) > 0 {
			return stepExecuteTools(ctx, sess, toolCalls, sbMgr)
		}
		return nil
	default:
		slog.Debug("Skipping step: last message role is neither User nor Tool nor Assistant with tools", "role", lastMsg.Role)
		return nil
	}
}

func stepCallModel(ctx context.Context, sess session.Session, modelName string, model models.ModelProvider, entries []session.Entry) error {
	var contextMessages []models.AgentMessage
	// 1. Prepend System Prompt (as User message or specific system role if supported)
	contextMessages = append(contextMessages, models.AgentMessage{
		Role: session.RoleUser,
		Content: []session.Content{{
			Type: session.ContentTypeText,
			Text: &session.TextContent{Content: "System: You are a helpful assistant. Please strictly use Markdown for all your responses."},
		}},
	})

	for _, entry := range entries {
		if entry.Message != nil {
			contextMessages = append(contextMessages, models.AgentMessage{
				Role:    entry.Message.Role,
				Content: entry.Message.Content,
			})
		}
	}

	stream, err := model.Stream(ctx, modelName, contextMessages)
	if err != nil {
		return fmt.Errorf("model stream error: %w", err)
	}
	defer stream.Close()

	assistantMsg, err := stream.FullMessage()
	if err != nil {
		return fmt.Errorf("model response error: %w", err)
	}

	if _, err := sess.AppendMessage(session.RoleAssistant, assistantMsg.Content); err != nil {
		return fmt.Errorf("failed to append assistant message: %w", err)
	}

	return nil
}

func stepExecuteTools(ctx context.Context, sess session.Session, toolCalls []session.Content, sbMgr sandbox.Manager) error {
	for _, toolCall := range toolCalls {
		toolName := toolCall.ToolUse.Name
		var resultMsg string

		if toolName == sandbox.ToolNameRunIPythonCell {
			if sbMgr == nil {
				resultMsg = "Error: Sandbox manager not available."
			} else {
				code, ok := toolCall.ToolUse.Input["code"].(string)
				if !ok {
					resultMsg = "Error: 'code' argument is required and must be a string."
				} else {
					slog.Info("Executing sandbox cell", "sessionID", sess.ID())
					res, err := sbMgr.RunCell(ctx, sess.ID(), code)
					if err != nil {
						resultMsg = fmt.Sprintf("Error executing cell: %v", err)
						slog.Error("Sandbox execution failed", "error", err)
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
		}

		// Append Tool Result
		content := []session.Content{
			{
				Type: session.ContentTypeToolResult,
				ToolResult: &session.ToolResultContent{
					ToolUseID: toolCall.ToolUse.ID,
					Content:   resultMsg,
				},
			},
		}

		if _, err := sess.AppendMessage(session.RoleTool, content); err != nil {
			return fmt.Errorf("failed to append tool result: %w", err)
		}
	}
	return nil
}

// Helper to extract tool calls from a message
func extractToolCalls(msg *session.MessageEntry) []session.Content {
	var calls []session.Content
	for _, c := range msg.Content {
		if c.Type == session.ContentTypeToolUse {
			calls = append(calls, c)
		}
	}
	return calls
}
