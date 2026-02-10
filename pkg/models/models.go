package models

import (
	"context"

	"github.com/mariozechner/coding-agent/session/pkg/store"
)

// AgentMessage represents a message in the agent's context.
// It mirrors store.MessageEntry but is specific to the active agent loop.
type AgentMessage struct {
	// Role indicates the sender of the message (e.g., user, assistant).
	Role store.MessageRole
	// Content holds the key content parts of the message.
	Content []store.Content
}

// ModelProvider represents a service that provides LLMs (e.g. Gemini, OpenAI).
type ModelProvider interface {
	// List returns the names of available models.
	List(ctx context.Context) ([]string, error)

	// Stream sends a context to the LLM and returns a stream of events/messages.
	Stream(ctx context.Context, modelName string, messages []AgentMessage) (ModelStream, error)
}

// ModelStream abstracts the stream of responses from the model.
type ModelStream interface {
	// FullMessage blocks until the full message is available.
	FullMessage() (AgentMessage, error)
	Close() error
}
