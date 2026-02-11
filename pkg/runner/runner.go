package runner

import (
	"context"

	"log/slog"

	"github.com/mariozechner/coding-agent/session/pkg/models"
	"github.com/mariozechner/coding-agent/session/pkg/sandbox"
	"github.com/mariozechner/coding-agent/session/pkg/store"
)

// Runner coordinates the execution of agents based on session events.
type Runner struct {
	manager        store.Manager
	model          models.ModelProvider
	modelName      string
	sandboxManager sandbox.Manager
	ErrorChan      chan error
}

func New(manager store.Manager, model models.ModelProvider, modelName string, sandboxManager sandbox.Manager) *Runner {
	return &Runner{
		manager:        manager,
		model:          model,
		modelName:      modelName,
		sandboxManager: sandboxManager,
		ErrorChan:      make(chan error, 10),
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
			sess, err := r.manager.LoadSession(sessionID)
			if err != nil {
				slog.Error("Error loading session", "sessionID", sessionID, "error", err)
				continue
			}

			if err := RunStep(ctx, sess, r.modelName, r.model, r.sandboxManager); err != nil {
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

// StopSession marks a session as ended and stops its sandbox.
func (r *Runner) StopSession(ctx context.Context, sessionID string) error {
	if err := r.manager.SetSessionStatus(sessionID, store.SessionStatusEnded); err != nil {
		return err
	}
	return r.sandboxManager.Stop(ctx, sessionID)
}
