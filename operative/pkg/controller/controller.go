package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/nstogner/operative/pkg/domain"
	"github.com/nstogner/operative/pkg/model"
	"github.com/nstogner/operative/pkg/sandbox"
	"github.com/nstogner/operative/pkg/store"
)

// Controller is the main control loop for operatives. It subscribes to stream
// events and orchestrates the agent loop: calling the model, executing tools,
// and managing compaction.
type Controller struct {
	operatives store.OperativeStore
	stream     store.StreamStore
	notes      store.NoteStore
	provider   model.Provider
	sandbox    sandbox.Manager
}

// New creates a new Controller.
func New(
	operatives store.OperativeStore,
	stream store.StreamStore,
	notes store.NoteStore,
	provider model.Provider,
	sandbox sandbox.Manager,
) *Controller {
	return &Controller{
		operatives: operatives,
		stream:     stream,
		notes:      notes,
		provider:   provider,
		sandbox:    sandbox,
	}
}

// staticInstructions describes the operative's environment and available tools.
// This is always prepended to the system instructions.
const staticInstructions = `You are an operative — an autonomous agent with access to a sandboxed Python environment and tools for managing your own knowledge.

## Environment

You have access to a persistent IPython kernel running in a sandboxed container. You can execute arbitrary Python code using the run_ipython_cell tool. State persists across cells within a single session.

The following functions are injected into the IPython namespace and can be called directly from any cell:

- prompt_model(prompt: str) -> str
  Sends a prompt to the LLM and returns the text response. Use this to delegate sub-tasks like summarization, analysis, or Q&A within your code.

- prompt_self(message: str) -> None
  Sends a system message back into the conversation stream. Use this for progress updates or to surface information to the user during long-running computations.

## Available Tools

- run_ipython_cell: Execute Python code in your IPython sandbox. The last expression in a cell is automatically displayed (like a Jupyter notebook). Use this for computation, data processing, or any task that benefits from code execution.
- update_instructions: Update your own self-set instructions. Use this to record important preferences, behavioral guidelines, or context you want remembered across conversations.
- store_note: Store a searchable note with a title and content. Use this to save important information for later retrieval.
- keyword_search_notes: Search your stored notes by keyword. Returns note IDs and titles.
- vector_search_notes: Search your stored notes by semantic similarity. Returns note IDs and titles.
- get_note: Retrieve the full content of a note by its ID.
- delete_note: Delete a note by its ID.

## Guidelines

- Use the IPython sandbox for almost all tasks. Examples include:
  * Browsing the web
  * Reading and writing files
  * Running math
  * Calling LLMs to summarize large files (using the ipython prompt_model func)
  * Importing libraries
- Store important findings and knowledge using notes for future reference.
- Update your self-set instructions when you learn important operational preferences.`

// buildInstructions concatenates the three instruction sources:
// 1. Static environment/tools description
// 2. Admin-set instructions
// 3. Operative self-set instructions
func buildInstructions(op *domain.Operative) string {
	parts := []string{staticInstructions}
	if op.AdminInstructions != "" {
		parts = append(parts, "## Admin Instructions\n\n"+op.AdminInstructions)
	}
	if op.OperativeInstructions != "" {
		parts = append(parts, "## Operative Self-Set Instructions\n\n"+op.OperativeInstructions)
	}
	return strings.Join(parts, "\n\n")
}

// Start listens for stream events and triggers the control loop.
func (c *Controller) Start(ctx context.Context) error {
	events := c.stream.Subscribe()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case operativeID := <-events:
			if err := c.step(ctx, operativeID); err != nil {
				slog.Error("Controller step error", "operativeID", operativeID, "error", err)
			}
		}
	}
}

// step executes one step of the control loop for the given operative.
func (c *Controller) step(ctx context.Context, operativeID string) error {
	// Load the operative configuration.
	op, err := c.operatives.Get(ctx, operativeID)
	if err != nil {
		return fmt.Errorf("loading operative: %w", err)
	}

	// Load recent stream entries.
	entries, err := c.stream.GetEntries(ctx, operativeID, 0)
	if err != nil {
		return fmt.Errorf("loading stream: %w", err)
	}

	if len(entries) == 0 {
		return nil
	}

	// Determine what to do based on the last entry.
	last := entries[len(entries)-1]

	switch {
	case last.Role == domain.RoleUser:
		// User sent a message → call the model.
		if err := c.callModel(ctx, op, entries); err != nil {
			return err
		}
		// Check if compaction is needed after the model response.
		updatedEntries, err := c.stream.GetEntries(ctx, operativeID, 0)
		if err != nil {
			return fmt.Errorf("reloading stream for compaction: %w", err)
		}
		return c.checkAndCompact(ctx, op, updatedEntries)

	case last.Role == domain.RoleAssistant && last.ContentType == domain.ContentTypeToolCall:
		// Model requested a tool call → execute it.
		return c.executeTool(ctx, op, last)

	case last.Role == domain.RoleTool:
		// Tool result → call model again with the result.
		if err := c.callModel(ctx, op, entries); err != nil {
			return err
		}
		updatedEntries, err := c.stream.GetEntries(ctx, operativeID, 0)
		if err != nil {
			return fmt.Errorf("reloading stream for compaction: %w", err)
		}
		return c.checkAndCompact(ctx, op, updatedEntries)

	default:
		// Nothing to do (e.g. assistant text response, compaction summary).
		return nil
	}
}

// callModel calls the model with the current stream context.
func (c *Controller) callModel(ctx context.Context, op *domain.Operative, entries []domain.StreamEntry) error {
	// Build system instructions from all three sources.
	instructions := buildInstructions(op)

	// Convert stream entries to model messages.
	messages := entriesToMessages(entries)

	// Call model.
	stream, err := c.provider.Stream(ctx, op.Model, instructions, messages)
	if err != nil {
		return fmt.Errorf("streaming model: %w", err)
	}
	defer stream.Close()

	msg, err := stream.FullMessage()
	if err != nil {
		return fmt.Errorf("getting model response: %w", err)
	}

	// Write the response to the stream.
	for _, content := range msg.Content {
		entry := &domain.StreamEntry{
			ID:          uuid.New().String(),
			OperativeID: op.ID,
			Role:        domain.RoleAssistant,
			Model:       op.Model,
		}

		switch content.Type {
		case domain.ContentTypeText:
			entry.ContentType = domain.ContentTypeText
			entry.Content = content.Text
		case domain.ContentTypeToolCall:
			entry.ContentType = domain.ContentTypeToolCall
			b, _ := json.Marshal(content.ToolCall)
			entry.Content = string(b)
		}

		if err := c.stream.Append(ctx, entry); err != nil {
			return fmt.Errorf("appending response: %w", err)
		}
	}

	return nil
}

// executeTool executes a tool call and appends the result.
func (c *Controller) executeTool(ctx context.Context, op *domain.Operative, entry domain.StreamEntry) error {
	var tc domain.ToolCall
	if err := json.Unmarshal([]byte(entry.Content), &tc); err != nil {
		return fmt.Errorf("parsing tool call: %w", err)
	}

	result, err := c.dispatchTool(ctx, op, &tc)
	if err != nil {
		// Record the error as a tool result.
		result = &domain.ToolResult{
			ToolCallID: tc.ID,
			Content:    fmt.Sprintf("Error: %v", err),
			IsError:    true,
		}
	}

	resultJSON, _ := json.Marshal(result)
	return c.stream.Append(ctx, &domain.StreamEntry{
		ID:          uuid.New().String(),
		OperativeID: op.ID,
		Role:        domain.RoleTool,
		ContentType: domain.ContentTypeToolResult,
		Content:     string(resultJSON),
	})
}

// dispatchTool routes a tool call to the appropriate handler.
func (c *Controller) dispatchTool(ctx context.Context, op *domain.Operative, tc *domain.ToolCall) (*domain.ToolResult, error) {
	switch tc.Name {
	case "run_ipython_cell":
		return c.toolRunIPythonCell(ctx, op, tc)
	case "update_instructions":
		return c.toolUpdateInstructions(ctx, op, tc)
	case "store_note":
		return c.toolStoreNote(ctx, op, tc)
	case "keyword_search_notes":
		return c.toolKeywordSearchNotes(ctx, op, tc)
	case "vector_search_notes":
		return c.toolVectorSearchNotes(ctx, op, tc)
	case "get_note":
		return c.toolGetNote(ctx, op, tc)
	case "delete_note":
		return c.toolDeleteNote(ctx, op, tc)
	default:
		return nil, fmt.Errorf("unknown tool: %s", tc.Name)
	}
}

// entriesToMessages converts stream entries to model messages.
func entriesToMessages(entries []domain.StreamEntry) []model.Message {
	var messages []model.Message
	for _, e := range entries {
		msg := model.Message{Role: e.Role}
		switch e.ContentType {
		case domain.ContentTypeText:
			msg.Content = []model.Content{{Type: domain.ContentTypeText, Text: e.Content}}
		case domain.ContentTypeToolCall:
			var tc domain.ToolCall
			json.Unmarshal([]byte(e.Content), &tc)
			msg.Content = []model.Content{{Type: domain.ContentTypeToolCall, ToolCall: &tc}}
		case domain.ContentTypeToolResult:
			var tr domain.ToolResult
			json.Unmarshal([]byte(e.Content), &tr)
			msg.Content = []model.Content{{Type: domain.ContentTypeToolResult, ToolResult: &tr}}
		}
		messages = append(messages, msg)
	}
	return messages
}
