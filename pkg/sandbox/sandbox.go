package sandbox

import "context"

// Result represents the output of a sandbox execution.
type Result struct {
	// Output is the combined stdout and stderr (if not split).
	Output string `json:"output,omitempty"`
	// Stdout is the standard output (if split).
	Stdout string `json:"stdout,omitempty"`
	// Stderr is the standard error (if split).
	Stderr string `json:"stderr,omitempty"`
}

// Delegate defines the callbacks that the sandbox can invoke.
type Delegate interface {
	// PromptModel prompts the agent's model with the given message.
	PromptModel(ctx context.Context, prompt string) (string, error)
	// PromptSelf sends a message to the agent's session (as if from the user/system) but does not return a response.
	PromptSelf(ctx context.Context, message string) error
}

// Manager defines the interface for managing sandboxes.
type Manager interface {
	// RunCell executes a code cell (ipython) within the sandbox for the given session.
	// This should lazily initialize the sandbox if it's not running.
	RunCell(ctx context.Context, sessionID string, code string, delegate Delegate) (*Result, error)

	// Stop terminates the sandbox for the given session.
	Stop(ctx context.Context, sessionID string) error

	// Close releases any resources held by the manager (e.g. docker client).
	Close() error
}
