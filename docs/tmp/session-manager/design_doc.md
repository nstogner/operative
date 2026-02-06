# Design Doc: Session Manager (Go)

## 1. Overview
This document outlines the design for a `SessionManager` library in Go, replicating the core functionality of the TypeScript `pi-mono` SessionManager. The goal is to provide a robust, concurrent-safe, append-only conversation history manager that supports branching (tree structure) and context compaction.

## 2. Goals & Non-Goals
### Goals
*   **Append-Only Storage**: Use JSONL for crash resilience and simple history tracking.
*   **Tree Structure**: Support infinite branching without duplicating file content using `id` and `parent_id` pointers.
*   **Context Compaction**: Efficiently handle long conversations by summarizing older context and maintaining a pointer to the first kept message.
*   **Checkpointing**: Capture context from abandoned paths when branching using summaries.
*   **Concurrency**: Thread-safe operations for high-performance concurrent agents.
*   **Strict Typing**: Leverage Go's type system for entry schemas.

### Non-Goals
*   **Backwards Compatibility**: No support for legacy `pi-mono` formats (v1/v2 transformations).
*   **UI Concerns**: TUI formatting logic is out of scope; this is a data layer.

## 3. Storage Format
We will adhere to the **JSONL (JSON Lines)** format. Each line represents an event in the session.

### File Structure
*   **Header (Line 1)**: Session metadata.
*   **Body (Line 2+)**: Ordered sequence of events.

### Entry Types
All entries (except Header) share a common base structure.

```json
{
  "type": "message|compaction|branch_summary|...",
  "id": "uuid-v4",
  "parent_id": "parent-uuid" | null,
  "timestamp": "2024-01-01T12:00:00Z"
}
```

## 4. Go Data Models

### 4.1. Core Types

```go
package session

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
	RoleTool              MessageRole = "tool"               // For tool results
	RoleBashExecution     MessageRole = "bashExecution"     // Command execution event
	RoleCustom            MessageRole = "custom"            // Extension-injected data
	RoleBranchSummary     MessageRole = "branchSummary"     // Checkpoint for abandoned branch
	RoleCompactionSummary MessageRole = "compactionSummary" // Summary of discarded history
)

// Header is the first line of the file (metadata)
type Header struct {
	Type          EntryType `json:"type"` // Always "session"
	ID            string    `json:"id"`
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
```

### 4.2. Payload Definitions

```go
type MessageEntry struct {
	Role    string    `json:"role"`
	Content []Content `json:"content"`
	Model   string    `json:"model,omitempty"`
}

type ModelChangeEntry struct {
	Provider string `json:"provider"`
	ModelID  string `json:"model_id"`
}

type ThinkingLevelEntry struct {
	ThinkingLevel string `json:"thinking_level"` // e.g. "high", "low", "off"
}

type LabelEntry struct {
	TargetID string `json:"target_id"`
	Label    string `json:"label,omitempty"` // empty/null to remove
}

type SessionInfoEntry struct {
	Name string `json:"name"`
}

type CompactionEntry struct {
	Summary          string `json:"summary"`
	FirstKeptEntryID string `json:"first_kept_entry_id"`
	TokensBefore     int    `json:"tokens_before"`
}

type BranchSummaryEntry struct {
	Summary string `json:"summary"`
	FromID  string `json:"from_id"`
}

type CustomEntry struct {
	CustomType string         `json:"custom_type"`
	Data       map[string]any `json:"data"`
}
```

### 4.3. Content Types (Message Components)

```go
type ContentType string

const (
	ContentTypeText       ContentType = "text"
	ContentTypeImage      ContentType = "image"
	ContentTypeToolUse    ContentType = "tool_use"
	ContentTypeToolResult ContentType = "tool_result"
)

type Content struct {
	Type ContentType `json:"type"`

	// Only one of these will be non-nil
	Text       *TextContent       `json:"text,omitempty"`
	Image      *ImageContent      `json:"image,omitempty"`
	ToolUse    *ToolUseContent    `json:"tool_use,omitempty"`
	ToolResult *ToolResultContent `json:"tool_result,omitempty"`
}

type TextContent struct {
	Content string `json:"content"`
}

type ImageContent struct {
	Source *ImageSource `json:"source"`
}

type ImageSource struct {
	Type      string `json:"type"` // "base64" or "url"
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type ToolUseContent struct {
	ID    string         `json:"id"`
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
}

type ToolResultContent struct {
	ToolUseID string `json:"tool_use_id"`
	IsError   bool   `json:"is_error"`
	Content   string `json:"content"`
}
```

## 5. Content & Handling Examples

### 5.1. File Content Handling
Files are typically handled in one of two ways:
1.  **Text Context**: The *contents* of the file are embedded in a `TextContent` with formatting (e.g., XML tags).
2.  **Tool Reference**: A tool (like `read_file`) returns the content, which becomes a `ToolResultContent`.

**Example: Text Content with File**
```json
{
  "type": "message",
  "id": "msg-123",
  "parent_id": "msg-122",
  "timestamp": "2024-01-01T12:00:00Z",
  "message": {
    "role": "user",
    "content": [
      {
        "type": "text",
        "text": {
          "content": "Analyze this file:\n<file name=\"main.go\">\npackage main\n...</file>"
        }
      }
    ]
  }
}
```

### 5.2. Provider Mappings
Normalizing upstream formats into our `Content` schema.

| Provider | Upstream Format | Internal `MessageEntry` Mapping |
| :--- | :--- | :--- |
| **OpenAI** | `content: "string"` | `Content: []Content{{Type: "text", Text: {Content: "..."}}}` |
| **OpenAI** | `content: [{type: "text", text: "..."}]` | `Content: []Content{{Type: "text", Text: {Content: "..."}}}` |
| **Anthropic** | `content: [{type: "text", text: "..."}]` | `Content: []Content{{Type: "text", Text: {Content: "..."}}}` |
| **Google** | `parts: [{text: "..."}]` | `Content: []Content{{Type: "text", Text: {Content: "..."}}}` |
| **Google** | `parts: [{inline_data: {...}}]` | `Content: []Content{{Type: "image", Image: {...}}}` |

## 6. Session Manager API

The `Session` struct manages the in-memory state and file I/O.

```go
type Session struct {
	mu          sync.RWMutex
	id          string
	filePath    string
	entries     map[string]Entry // ID -> Entry lookup
	leafID      string           // Current tip of the tree
	fileHandle  *os.File
}

type Config struct {
	Dir string // Directory to store sessions
}

type Manager interface {
	New(cfg Config) (*Session, error)
	Load(path string) (*Session, error)
}
```

### 6.1. Key Methods

```go
// --- Lifecycle Management ---

// New initializes a new session.
// dir: Directory to store the session JSONL.
// parentSessionID: Optional ID of the session this one was branched from.
func New(dir string, parentSessionID string) (*Session, error)

// Load opens an existing session file.
// path: Absolute path to the .jsonl file.
func Load(path string) (*Session, error)

// ContinueRecent finds and loads the most recently modified session in a directory.
// dir: Directory to search for .jsonl files.
func ContinueRecent(dir string) (*Session, error)

// ForkFrom creates a new session based on an existing file's history.
// sourcePath: Path to the source session file.
// targetDir: Where the new session file will be created.
func ForkFrom(sourcePath string, targetDir string) (*Session, error)

// --- Writing & Appending ---

// Append adds a generic entry as a child of the current leaf and advances the leaf pointer.
// entry: The fully populated Entry struct to append.
func (s *Session) Append(entry Entry) error

// AppendMessage appends a standard conversation message.
// role: One of the Role constants (User, Assistant, Tool).
// content: Slice of Content items (text, images, tool calls).
func (s *Session) AppendMessage(role MessageRole, content []Content) (string, error)

// AppendThinkingLevelChange records a change in the agent's internal thinking depth.
// level: String identifier for the depth (e.g., "high", "low", "off").
func (s *Session) AppendThinkingLevelChange(level string) (string, error)

// AppendModelChange records a shift in the underlying LLM being used.
// provider: The AI provider name (e.g., "openai").
// modelID: The specific model version (e.g., "gpt-4o").
func (s *Session) AppendModelChange(provider, modelID string) (string, error)

// AppendCompaction records a summary of truncated history.
// summary: The LLM-generated summarization text.
// firstKeptID: The ID of the earliest message preserved after this compaction.
// tokens: The token count of the context *before* this compaction occurred.
func (s *Session) AppendCompaction(summary, firstKeptID string, tokens int) (string, error)

// AppendSessionInfo updates metadata like the session's display name.
// name: The human-readable name for the session.
func (s *Session) AppendSessionInfo(name string) (string, error)

// AppendCustomEntry persists arbitrary extension data.
// customType: Unique string key for the extension.
// data: Map of data to persist.
func (s *Session) AppendCustomEntry(customType string, data map[string]any) (string, error)

// SetLabel associates a bookmark string with an entry.
// targetID: ID of the entry to label.
// label: The text of the label (pass empty string to clear).
func (s *Session) SetLabel(targetID string, label string) (string, error)

// --- Navigation & Branching ---

// Branch moves the leaf pointer to an earlier entry.
// entryID: The ID of the entry to start branching from.
func (s *Session) Branch(entryID string) error

// BranchWithSummary moves the leaf pointer and appends a summary of the abandoned path.
// branchFromID: Entry ID to branch from.
// summary: LLM summary of the conversation path being abandoned.
func (s *Session) BranchWithSummary(branchFromID string, summary string) (string, error)

// CreateBranchedSession exports a linear message path to a new file.
// leafID: The end of the path to export.
func (s *Session) CreateBranchedSession(leafID string) (string, error)

// --- Reading & Context ---

// GetContext builds the linear history from the current leaf back to root, resolving summaries.
func (s *Session) GetContext() ([]Entry, error)

// GetTree returns the full session as a hierarchical tree structure.
func (s *Session) GetTree() ([]TreeNode, error)

// List return metadata for all session files in a directory.
// dir: Directory to scan for .jsonl files.
func List(dir string) ([]SessionInfo, error)
```

type TreeNode struct {
	Entry    Entry
	Children []TreeNode
	Label    string
}

type SessionInfo struct {
	ID           string
	Path         string
	Name         string
	Created      time.Time
	Modified     time.Time
	MessageCount int
}

## 7. Lifecycle & Branching Behavior

### 7.1. Appending
The session is a collection of entries forming a tree. The `leafID` tracks the "current" position. When a new message or event occurs:
1.  A new `Entry` is created with a unique `ID`.
2.  Its `ParentID` is set to the current `leafID`.
3.  The entry is appended to the JSONL file and added to the in-memory index.
4.  The `leafID` is updated to the new entry's `ID`.

### 7.2. Branching
Branching allows the user or agent to "go back in time."
*   **`Branch(id)`**: Simply updates the internal `leafID`. The next `Append` will create a child of the selected entry.
*   **`BranchWithSummary(id, summary)`**: Used when moving away from a long-running branch. It appends a `BranchSummaryEntry` as a child of the `id`, and then sets the `leafID` to this summary entry. This creates a "checkpoint" message in the new branch's history.

## 8. Compaction & Traversal Logic

Compaction reduces the number of messages sent to the LLM by summarizing old history.

### 8.1. Compaction Entry
A `CompactionEntry` contains:
*   `Summary`: The LLM-generated summarization text.
*   `FirstKeptEntryID`: The ID of the earliest message that remains in the context *after* the summary.
*   `TokensBefore`: Total token count of the context *before* this compaction occurred. This serves as a "snapshot" for observability (reporting how much history was summarized) and helps the system track context window usage over time.

### 8.2. Compaction Safety Rules
The implementation must ensure that compaction never breaks the semantic integrity of the conversation. Specifically:

*   **No Tool Splitting**: A `compaction` entry must never be placed between a tool call (Assistant message) and its corresponding `toolResult`. These must always exist as a single unit in the history.
*   **Valid Cut Points**: Compaction should only occur at "stable" boundaries:
    - Before a `user` message (start of a turn).
    - Before an `assistant` message that does *not* contain tool calls.
    - Before other non-message events (like `model_change` or `thinking_level_change`).
*   **Responsibility**: While the `SessionManager` provides the storage mechanism (`AppendCompaction`), the **caller** (or a specialized compaction utility) is responsible for calculating the safe `first_kept_entry_id`.

### 8.2. Traversal Algorithm (`GetContext`)
To build the message list for an LLM request:
1.  **Identify Leaf**: Start at the current `leafID`.
2.  **Find Path**: Follow `parent_id` pointers up to the root (null `parent_id`) to identify the sequence of entries in the current branch.
3.  **Find Compaction**: Identify the most recent `CompactionEntry` in the path (if any).
4.  **Assemble Messages**:
    - If no compaction: Include all messages in the path.
    - If compaction exists:
        - Include the `CompactionEntry`'s `Summary` first.
        - Start including standard messages *only after* reaching the entry matching `FirstKeptEntryID`.
        - Include all messages from `FirstKeptEntryID` up to the current leaf.
5.  **Inject System Info**: `ThinkingLevel` and `Model` changes are tracked during the walk to set the current session state.

## 9. Special Entry Handling

*   **LabelEntry**: These are "meta" entries that don't appear in the LLM context. They associate a string (label) with a target entry ID via the `SetLabel` method.
    - **Use Case: Bookmarking**: Users or agents can "bookmark" significant points in a conversation (e.g., "Requirement defined", "Bug reproduced") for easy retrieval.
    - **Use Case: Navigation**: UI components use labels to provide a "Table of Contents" for long sessions, allowing users to jump back to specific nodes in the tree.
    - **Use Case: Automation**: Extensions can label entries that need special periodic processing (e.g., "needs_review").
    - **State Management**: The `SessionManager` maintains a map of the *latest* label for any given entry. Setting a label to an empty string effectively removes the bookmark.
*   **SessionInfoEntry**: Used for metadata like the session's display name.
*   **CustomEntry**: Allows extensions to store arbitrary JSON data that persists in the session log but is ignored by the LLM context builder.

## 11. Orchestration Layer (AgentSession)
The `SessionManager` is a "passive" data layer focused on persistence and tree reconstruction. An "active" orchestration layer (e.g., `AgentSession`) is required to drive the agent's lifecycle.

### 11.1. Responsibilities
*   **Event Handling**: Subscribes to agent events (e.g., `message_end`) and calls `SessionManager.Append` to persist history in real-time.
*   **Context Management**: Calls `GetContext` before each LLM request to build the linear history.
*   **Automatic Compaction**:
    - Tracks token usage after each assistant response.
    - Decides when to trigger compaction based on the model's context window and a "reserve" threshold.
    - Uses a compaction utility to find safe cut points and generates the summary.
*   **Branching Decisions**:
    - Manages the `leafID` state during manual switches or automatic retries.
    - Determines when to use `BranchWithSummary` to preserve context from abandoned paths.
*   **Persistence Configuration**: Manages session file paths and ensures they are initialized correctly.

### 11.2. Relationship
| Component | Role | Logic |
| :--- | :--- | :--- |
| **LLM Agent** | Intelligence | Reasoning & Tool Calls |
| **AgentSession** | Orchestration | Decision making (When to compact? When to branch?) |
| **SessionManager** | Data Layer | Persistence, Tree Traversal, Path Reconstruction |
| **Compaction Utils** | Domain Logic | Token estimation, safety rule enforcement (valid cut points) |

## 12. Example Scenarios (JSONL)

### 12.1. Simple Session
```json
{"type":"session","id":"sess-123","version":1,"timestamp":"2024-01-01T10:00:00Z"}
{"type":"message","id":"msg-1","parent_id":null,"timestamp":"2024-01-01T10:00:01Z","message":{"role":"user","content":[{"type":"text","text":{"content":"Hello"}}]}}
{"type":"message","id":"msg-2","parent_id":"msg-1","timestamp":"2024-01-01T10:00:02Z","message":{"role":"assistant","content":[{"type":"text","text":{"content":"Hi there!"}}]}}
```

### 12.2. Complex Session (Tools)
```json
{"type":"session","id":"sess-456","version":1,"timestamp":"2024-02-01T12:00:00Z"}
{"type":"message","id":"m-1","parent_id":null,"timestamp":"2024-02-01T12:00:01Z","message":{"role":"user","content":[{"type":"text","text":{"content":"Read main.go"}}]}}
{"type":"message","id":"m-2","parent_id":"m-1","timestamp":"2024-02-01T12:00:02Z","message":{"role":"assistant","content":[{"type":"tool_use","tool_use":{"id":"call_abc","name":"read_file","input":{"path":"main.go"}}}]}}
{"type":"message","id":"m-3","parent_id":"m-2","timestamp":"2024-02-01T12:00:03Z","message":{"role":"tool","content":[{"type":"tool_result","tool_result":{"tool_use_id":"call_abc","content":"package main..."}}]}}
```

## 13. Go Usage Example

```go
package main

import (
	"fmt"
	"log"
	"session" // hypothetical package
)

func main() {
	// 1. Initialize or continue a session
	// JSONL: {"type":"session","id":"sess-123","version":1,"timestamp":"2024-01-01T10:00:00Z","cwd":"/path/to/project"}
	mgr := session.NewManager("./sessions")
	s, err := mgr.ContinueRecent()
	if err != nil {
		s, _ = mgr.New("", "")
	}
	defer s.Close()

	// 2. Append a user message using Role constant
	// JSONL: {"type":"message","id":"msg-1","parent_id":null,"timestamp":"...","message":{"role":"user","content":[{"type":"text","text":{"content":"Hello, Agent!"}}]}}
	userEntryID, _ := s.AppendMessage(session.RoleUser, []session.Content{
		{Type: session.ContentTypeText, Text: &session.TextContent{Content: "Hello, Agent!"}},
	})

	// 3. Get context for the LLM
	// (Returns msg-1 based on current leaf)
	history, _ := s.GetContext()
	fmt.Printf("Context has %d messages\n", len(history))

	// 4. Simulate an assistant response using Role constant
	// JSONL: {"type":"message","id":"msg-2","parent_id":"msg-1","timestamp":"...","message":{"role":"assistant","content":[{"type":"text","text":{"content":"Hello! How can I help?"}}]}}
	s.AppendMessage(session.RoleAssistant, []session.Content{
		{Type: session.ContentTypeText, Text: &session.TextContent{Content: "Hello! How can I help?"}},
	})

	// 5. Branching example: go back to user message
	// Updates internal leaf pointer to msg-1
	err = s.Branch(userEntryID)
	if err == nil {
		// JSONL: {"type":"message","id":"msg-3","parent_id":"msg-1","timestamp":"...","message":{"role":"user","content":[{"type":"text","text":{"content":"Actually, tell me a joke."}}]}}
		s.AppendMessage(session.RoleUser, []session.Content{
			{Type: session.ContentTypeText, Text: &session.TextContent{Content: "Actually, tell me a joke."}},
		})
	}

	// 6. Labeling an entry
	// JSONL: {"type":"label","id":"lbl-1","parent_id":"msg-3","target_id":"msg-1","label":"first-greeting","timestamp":"..."}
	s.SetLabel(userEntryID, "first-greeting")

	// 7. Compaction example
	// In this scenario, we decide to summarize msg-1 and msg-2, keeping only msg-3 and forward.
	// We call AppendCompaction with the summary and the ID of the first message to keep.
	// JSONL: {"type":"compaction","id":"comp-1","parent_id":"lbl-1","summary":"User greeted and then asked for a joke.","first_kept_entry_id":"msg-3","tokens_before":1500,"timestamp":"..."}
	s.AppendCompaction("User greeted and then asked for a joke.", "msg-3", 1500)

	// 8. Resulting Context
	// GetContext() will now return:
	// 1. The Compaction Summary (from comp-1)
	// 2. The message with id "msg-3" (the first_kept_entry_id)
	history, _ = s.GetContext()
	for _, entry := range history {
		if entry.Type == session.TypeCompaction {
			fmt.Println("Compaction summary included")
		} else if entry.ID == "msg-3" {
			fmt.Println("Recent message included")
		}
	}
}
```
