package sandbox

import "context"

// Result represents the output of a sandbox code execution.
type Result struct {
	// Output is the combined stdout and stderr.
	Output string `json:"output,omitempty"`
	// Stdout is the standard output (if split).
	Stdout string `json:"stdout,omitempty"`
	// Stderr is the standard error (if split).
	Stderr string `json:"stderr,omitempty"`
}

// OperativeLister lists operative IDs for sandbox reconciliation.
// This is a minimal interface to avoid importing the store package.
type OperativeLister interface {
	ListIDs(ctx context.Context) ([]string, error)
}

// Delegate defines the callbacks that sandbox code can invoke
// to interact with the operative's model and stream.
type Delegate interface {
	// PromptModel prompts the operative's model with the given message
	// and returns the model's response. Used from within IPython cells.
	PromptModel(ctx context.Context, prompt string) (string, error)

	// PromptSelf sends a message to the operative's stream as if from the
	// user/system. Does not wait for a response.
	PromptSelf(ctx context.Context, message string) error
}

// Manager defines the interface for managing container sandboxes.
// Each operative gets its own sandbox container.
type Manager interface {
	// Run starts a long-running reconciliation loop that keeps sandbox
	// containers in sync with known operatives. It periodically lists
	// operatives and ensures each has a running container. Containers
	// for unknown operatives are stopped. Blocks until ctx is cancelled.
	Run(ctx context.Context, operatives OperativeLister) error

	// RunCell executes a code cell (IPython) within the sandbox for the
	// given operative. The sandbox must already be running (started by Run).
	// Returns an error if the sandbox is not running.
	RunCell(ctx context.Context, operativeID, code string, delegate Delegate) (*Result, error)

	// Status returns the current status of the sandbox for the given operative.
	// Returns one of: "running", "stopped", "unknown".
	Status(ctx context.Context, operativeID string) (string, error)

	// Close releases any resources held by the manager (e.g. docker client).
	Close() error
}
