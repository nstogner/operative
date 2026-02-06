package session

// Manager defines the interface for managing sessions in a specific directory.
type Manager interface {
	// New initializes a new session.
	// parentSessionID: Optional ID of the session this one was branched from.
	New(parentSessionID string) (Session, error)

	// Load opens an existing session file by its ID.
	Load(id string) (Session, error)

	// ContinueRecent finds and loads the most recently modified session in the directory.
	ContinueRecent() (Session, error)

	// ForkFrom creates a new session based on an existing session's history.
	// id: ID of the source session.
	ForkFrom(id string) (Session, error)

	// List returns metadata for all session files in the managed directory.
	List() ([]SessionInfo, error)

	// Subscribe returns a channel that emits session IDs when an event occurs in any managed session.
	Subscribe() <-chan string
}

// Session defines the interface for interacting with a single conversation session.
// It manages the in-memory state and persistence for a conversation tree.
type Session interface {
	// ID returns the session's unique identifier.
	ID() string

	// Path returns the absolute path to the session's storage file.
	Path() string

	// LeafID returns the ID of the current tip of the conversation tree.
	LeafID() string

	// Append adds a generic entry as a child of the current leaf and advances the leaf pointer.
	Append(entry Entry) error

	// AppendMessage appends a standard conversation message.
	// role: One of the Role constants (User, Assistant, Tool).
	// content: Slice of Content items (text, images, tool calls).
	AppendMessage(role MessageRole, content []Content) (string, error)

	// AppendThinkingLevelChange records a change in the agent's internal thinking depth.
	AppendThinkingLevelChange(level string) (string, error)

	// AppendModelChange records a shift in the underlying LLM being used.
	// provider: The AI provider name (e.g., "openai").
	// modelID: The specific model version (e.g., "gpt-4o").
	AppendModelChange(provider, modelID string) (string, error)

	// AppendCompaction records a summary of truncated history.
	// summary: The LLM-generated summarization text.
	// firstKeptID: The ID of the earliest message preserved after this compaction.
	// tokens: The token count of the context *before* this compaction occurred.
	AppendCompaction(summary, firstKeptID string, tokens int) (string, error)

	// AppendSessionInfo updates metadata like the session's display name.
	AppendSessionInfo(name string) (string, error)

	// AppendCustomEntry persists arbitrary extension data.
	// customType: Unique string key for the extension.
	// data: Map of data to persist.
	AppendCustomEntry(customType string, data map[string]any) (string, error)

	// SetLabel associates a bookmark string with an entry.
	// targetID: ID of the entry to label.
	// label: The text of the label (pass empty string to clear).
	SetLabel(targetID string, label string) (string, error)

	// Branch moves the leaf pointer to an earlier entry.
	// entryID: The ID of the entry to start branching from.
	Branch(entryID string) error

	// BranchWithSummary moves the leaf pointer and appends a summary of the abandoned path.
	// branchFromID: Entry ID to branch from.
	// summary: LLM summary of the conversation path being abandoned.
	BranchWithSummary(branchFromID string, summary string) (string, error)

	// CreateBranchedSession exports a linear message path to a new session file.
	// leafID: The end of the path to export.
	CreateBranchedSession(leafID string) (string, error)

	// GetContext builds the linear history from the current leaf back to root, resolving summaries.
	GetContext() ([]Entry, error)

	// GetTree returns the full session as a hierarchical tree structure.
	GetTree() ([]TreeNode, error)

	// Close releases any resources (like file handles) held by the session.
	Close() error
}
