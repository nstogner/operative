package tools

import (
	"context"
)

// Tool defines the interface that all agent tools must implement.
type Tool interface {
	Name() string
	Description() string
	InputSchema() map[string]any // Simple representation of JSON schema
	Execute(ctx context.Context, input map[string]any) (any, error)
}

// Registry manages the available tools.
type Registry struct {
	tools map[string]Tool
}

// NewRegistry creates a new, empty registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register adds a tool to the registry.
func (r *Registry) Register(t Tool) {
	r.tools[t.Name()] = t
}

// Get returns a tool by name.
func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// List returns all registered tools.
func (r *Registry) List() []Tool {
	var list []Tool
	for _, t := range r.tools {
		list = append(list, t)
	}
	return list
}
