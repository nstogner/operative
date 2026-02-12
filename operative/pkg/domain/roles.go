package domain

// Role defines the sender of a stream entry.
type Role string

const (
	// RoleUser indicates a message from the user.
	RoleUser Role = "user"
	// RoleAssistant indicates a message from the model/assistant.
	RoleAssistant Role = "assistant"
	// RoleTool indicates a tool result.
	RoleTool Role = "tool"
	// RoleSystem indicates a system-level message (e.g. sandbox restart notice).
	RoleSystem Role = "system"
	// RoleCompactionSummary indicates a summary replacing compacted entries.
	RoleCompactionSummary Role = "compaction_summary"
)

// Stream entry content types.
const (
	ContentTypeText       = "text"
	ContentTypeToolCall   = "tool_call"
	ContentTypeToolResult = "tool_result"
)
