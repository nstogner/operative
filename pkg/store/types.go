package store

import (
	"time"
)

// EntryType defines the kind of session entry
type EntryType string

const (
	TypeSession       EntryType = "session"
	TypeMessage       EntryType = "message"
	TypeModelChange   EntryType = "model_change"
	TypeThinkingLevel EntryType = "thinking_level"
	TypeLabel         EntryType = "label"
	TypeSessionInfo   EntryType = "session_info"
	TypeCompaction    EntryType = "compaction"
	TypeBranchSummary EntryType = "branch_summary"
	TypeCustom        EntryType = "custom"
)

// MessageRole defines the role of a message in the conversation.
type MessageRole string

const (
	RoleUser              MessageRole = "user"
	RoleAssistant         MessageRole = "assistant"
	RoleSystem            MessageRole = "system"            // System instructions (from Agent)
	RoleTool              MessageRole = "tool"              // For tool results
	RoleBashExecution     MessageRole = "bashExecution"     // Command execution event
	RoleCustom            MessageRole = "custom"            // Extension-injected data
	RoleBranchSummary     MessageRole = "branchSummary"     // Checkpoint for abandoned branch
	RoleCompactionSummary MessageRole = "compactionSummary" // Summary of discarded history
)

// Agent represents a configuration for an AI agent.
type Agent struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Instructions string   `json:"instructions"`
	Model        string   `json:"model,omitempty"` // Default model
	Tools        []string `json:"tools,omitempty"` // Allowed tools
}

// Header is the first line of the file (metadata)
type Header struct {
	Type          EntryType `json:"type"` // Always "session"
	ID            string    `json:"id"`
	Agent         Agent     `json:"agent"`
	Version       int       `json:"version"`
	ParentSession string    `json:"parent_session,omitempty"`
	CreatedAt     time.Time `json:"timestamp"`
}

// Entry is a "Tagged Union" that represents any record in the session log.
type Entry struct {
	Type      EntryType `json:"type"`
	ID        string    `json:"id"`
	ParentID  *string   `json:"parent_id"` // Pointer to allow null for root
	Timestamp time.Time `json:"timestamp"`

	// Payload pointers - only one will be non-nil
	Message       *MessageEntry       `json:"message,omitempty"`
	ModelChange   *ModelChangeEntry   `json:"model_change,omitempty"`
	ThinkingLevel *ThinkingLevelEntry `json:"thinking_level,omitempty"`
	Label         *LabelEntry         `json:"label,omitempty"`
	SessionInfo   *SessionInfoEntry   `json:"session_info,omitempty"`
	Compaction    *CompactionEntry    `json:"compaction,omitempty"`
	BranchSummary *BranchSummaryEntry `json:"branch_summary,omitempty"`
	Custom        *CustomEntry        `json:"custom,omitempty"`
}

// MessageEntry represents a conversation message.
type MessageEntry struct {
	Role    MessageRole `json:"role"`
	Content []Content   `json:"content"`
	Model   string      `json:"model,omitempty"`
}

// ModelChangeEntry records a shift in the underlying LLM.
type ModelChangeEntry struct {
	Provider string `json:"provider"`
	ModelID  string `json:"model_id"`
}

// ThinkingLevelEntry records a change in agent thinking depth.
type ThinkingLevelEntry struct {
	ThinkingLevel string `json:"thinking_level"` // e.g. "high", "low", "off"
}

// LabelEntry associates a bookmark with an entry.
type LabelEntry struct {
	TargetID string `json:"target_id"`
	Label    string `json:"label,omitempty"` // empty to remove
}

// SessionInfoEntry updates session metadata.
type SessionInfoEntry struct {
	Name string `json:"name"`
}

// CompactionEntry contains a summary of discarded history.
type CompactionEntry struct {
	Summary          string `json:"summary"`
	FirstKeptEntryID string `json:"first_kept_entry_id"`
	TokensBefore     int    `json:"tokens_before"`
}

// BranchSummaryEntry captures context from an abandoned path.
type BranchSummaryEntry struct {
	Summary string `json:"summary"`
	FromID  string `json:"from_id"`
}

// CustomEntry persists arbitrary extension data.
type CustomEntry struct {
	CustomType string         `json:"custom_type"`
	Data       map[string]any `json:"data"`
}

// ContentType defines the kind of message content.
type ContentType string

const (
	ContentTypeText       ContentType = "text"
	ContentTypeImage      ContentType = "image"
	ContentTypeToolUse    ContentType = "tool_use"
	ContentTypeToolResult ContentType = "tool_result"
)

// Content represents a single component of a message.
type Content struct {
	Type ContentType `json:"type"`

	// Only one of these will be non-nil
	Text       *TextContent       `json:"text,omitempty"`
	Image      *ImageContent      `json:"image,omitempty"`
	ToolUse    *ToolUseContent    `json:"tool_use,omitempty"`
	ToolResult *ToolResultContent `json:"tool_result,omitempty"`
}

// TextContent contains literal text.
type TextContent struct {
	Content          string `json:"content"`
	ThoughtSignature []byte `json:"thought_signature,omitempty"`
}

// ImageContent contains image data.
type ImageContent struct {
	Source *ImageSource `json:"source"`
}

// ImageSource defines the origin of image data.
type ImageSource struct {
	Type      string `json:"type"` // "base64" or "url"
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

// ToolUseContent represents a call to a tool.
type ToolUseContent struct {
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	Input            map[string]any `json:"input"`
	ThoughtSignature []byte         `json:"thought_signature,omitempty"`
}

// ToolResultContent represents the outcome of a tool call.
type ToolResultContent struct {
	ToolUseID string `json:"tool_use_id"`
	IsError   bool   `json:"is_error"`
	Content   string `json:"content"`
}

// SessionInfo provides metadata about a session file.
type SessionInfo struct {
	ID           string
	Path         string
	Name         string
	Status       string
	AgentID      string
	AgentName    string
	Created      time.Time
	Modified     time.Time
	MessageCount int
}

const (
	SessionStatusActive = "active"
	SessionStatusEnded  = "ended"
)

// TreeNode represents a hierarchical view of the session.
type TreeNode struct {
	Entry    Entry
	Children []TreeNode
	Label    string
}
