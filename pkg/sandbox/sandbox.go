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

// Manager defines the interface for managing sandboxes.
type Manager interface {
	// RunCell executes a code cell (ipython) within the sandbox for the given session.
	// This should lazily initialize the sandbox if it's not running.
	RunCell(ctx context.Context, sessionID string, code string) (*Result, error)

	// Stop terminates the sandbox for the given session.
	Stop(ctx context.Context, sessionID string) error

	// Close releases any resources held by the manager (e.g. docker client).
	Close() error
}
