package domain

import "time"

// Operative represents a long-running agent with a container sandbox,
// a configurable model, and a rolling message stream.
type Operative struct {
	ID                    string    `json:"id"`
	Name                  string    `json:"name"`
	AdminInstructions     string    `json:"admin_instructions"`
	OperativeInstructions string    `json:"operative_instructions"`
	Model                 string    `json:"model"`
	CompactionModel       string    `json:"compaction_model,omitempty"`
	CompactionThreshold   float64   `json:"compaction_threshold,omitempty"` // 0-1, fraction of max context window
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

// StreamEntry represents a single entry in an operative's message stream.
type StreamEntry struct {
	ID          string    `json:"id"`
	OperativeID string    `json:"operative_id"`
	Role        Role      `json:"role"`
	ContentType string    `json:"content_type"` // "text", "tool_call", "tool_result"
	Content     string    `json:"content"`      // Text content or JSON-encoded tool call/result
	Model       string    `json:"model,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
}

// Note is a persistent, searchable text entry attached to an operative.
type Note struct {
	ID          string    `json:"id"`
	OperativeID string    `json:"operative_id"`
	Title       string    `json:"title"`
	Content     string    `json:"content"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// NoteRef is a lightweight reference to a note, returned by search operations.
type NoteRef struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// Model represents an available LLM model.
type Model struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Provider  string `json:"provider"`
	MaxTokens int    `json:"max_tokens,omitempty"`
}

// ToolCall represents a tool invocation by the model.
type ToolCall struct {
	ID    string         `json:"id"`
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
}

// ToolResult represents the outcome of a tool call execution.
type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
	IsError    bool   `json:"is_error"`
}
