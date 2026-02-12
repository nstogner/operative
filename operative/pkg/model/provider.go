package model

import (
	"context"

	"github.com/nstogner/operative/pkg/domain"
)

// Message represents a message in the model's conversation context.
type Message struct {
	// Role indicates the sender (user, assistant, tool, system).
	Role domain.Role
	// Content holds the message parts.
	Content []Content
}

// Content represents a single component of a message.
type Content struct {
	Type string // "text", "tool_call", "tool_result"

	// Text content (when Type == "text").
	Text string `json:"text,omitempty"`

	// Tool call (when Type == "tool_call").
	ToolCall *domain.ToolCall `json:"tool_call,omitempty"`

	// Tool result (when Type == "tool_result").
	ToolResult *domain.ToolResult `json:"tool_result,omitempty"`

	// ThoughtSignature is an opaque signature for the model's internal state.
	// Must be round-tripped back to the model on the next request.
	ThoughtSignature []byte `json:"thought_signature,omitempty"`
}

// Provider represents a service that provides LLMs (e.g. Gemini, OpenAI).
type Provider interface {
	// Name returns the provider's identifier (e.g. "gemini", "openai").
	Name() string

	// List returns the available models from this provider.
	List(ctx context.Context) ([]domain.Model, error)

	// Stream sends a conversation context to the LLM and returns a stream of responses.
	// modelName identifies which model to use (e.g. "gemini-2.0-flash").
	// instructions is the system prompt.
	// messages is the conversation history.
	Stream(ctx context.Context, modelName, instructions string, messages []Message) (ModelStream, error)
}

// ModelStream abstracts the stream of responses from the model.
type ModelStream interface {
	// FullMessage blocks until the complete response is available and returns it.
	FullMessage() (Message, error)

	// Close releases resources associated with this stream.
	Close() error
}
