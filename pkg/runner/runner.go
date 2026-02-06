package runner

import (
	"context"

	"log/slog"

	"github.com/mariozechner/coding-agent/session/pkg/models"
	"github.com/mariozechner/coding-agent/session/pkg/session"
	"github.com/mariozechner/coding-agent/session/pkg/tools"
)

// Runner coordinates the execution of agents based on session events.
type Runner struct {
	manager   session.Manager
	model     models.ModelProvider
	modelName string
	tools     *tools.Registry
	ErrorChan chan error
}

func New(manager session.Manager, model models.ModelProvider, modelName string, toolRegistry *tools.Registry) *Runner {
	return &Runner{
		manager:   manager,
		model:     model,
		modelName: modelName,
		tools:     toolRegistry,
		ErrorChan: make(chan error, 10),
	}
}

// Start listens for session events and triggers agent steps.
func (r *Runner) Start(ctx context.Context) error {
	events := r.manager.Subscribe()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case sessionID := <-events:
			// Load session
			sess, err := r.manager.Load(sessionID)
			if err != nil {
				slog.Error("Error loading session", "sessionID", sessionID, "error", err)
				continue
			}

			if err := RunStep(ctx, sess, r.modelName, r.model, r.tools); err != nil {
				slog.Error("Error running step for session", "sessionID", sessionID, "error", err)
				select {
				case r.ErrorChan <- err:
				default:
				}
			}

			sess.Close()
		}
	}
}
