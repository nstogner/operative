package controller

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/nstogner/operative/pkg/domain"
	"github.com/nstogner/operative/pkg/sandbox"
)

// toolRunIPythonCell executes code in the operative's sandbox.
func (c *Controller) toolRunIPythonCell(ctx context.Context, op *domain.Operative, tc *domain.ToolCall) (*domain.ToolResult, error) {
	code, _ := tc.Input["code"].(string)
	if code == "" {
		return &domain.ToolResult{
			ToolCallID: tc.ID,
			Content:    "Error: 'code' parameter is required",
			IsError:    true,
		}, nil
	}

	delegate := &controllerDelegate{
		ctx:  ctx,
		ctrl: c,
		op:   op,
	}

	result, err := c.sandbox.RunCell(ctx, op.ID, code, delegate)
	if err != nil {
		return nil, fmt.Errorf("running cell: %w", err)
	}

	output := result.Output
	if output == "" {
		output = result.Stdout
		if result.Stderr != "" {
			output += "\n" + result.Stderr
		}
	}

	return &domain.ToolResult{
		ToolCallID: tc.ID,
		Content:    output,
	}, nil
}

// toolUpdateInstructions updates the operative's self-set instructions.
func (c *Controller) toolUpdateInstructions(ctx context.Context, op *domain.Operative, tc *domain.ToolCall) (*domain.ToolResult, error) {
	instructions, _ := tc.Input["instructions"].(string)

	if err := c.operatives.UpdateInstructions(ctx, op.ID, op.AdminInstructions, instructions); err != nil {
		return nil, fmt.Errorf("updating instructions: %w", err)
	}

	return &domain.ToolResult{
		ToolCallID: tc.ID,
		Content:    "Instructions updated successfully.",
	}, nil
}

// toolStoreNote creates a new note for the operative.
func (c *Controller) toolStoreNote(ctx context.Context, op *domain.Operative, tc *domain.ToolCall) (*domain.ToolResult, error) {
	title, _ := tc.Input["title"].(string)
	content, _ := tc.Input["content"].(string)

	note := &domain.Note{
		ID:          uuid.New().String(),
		OperativeID: op.ID,
		Title:       title,
		Content:     content,
	}

	if err := c.notes.CreateNote(ctx, note); err != nil {
		return nil, fmt.Errorf("creating note: %w", err)
	}

	return &domain.ToolResult{
		ToolCallID: tc.ID,
		Content:    fmt.Sprintf("Note stored with ID: %s", note.ID),
	}, nil
}

// toolKeywordSearchNotes searches notes by keyword.
func (c *Controller) toolKeywordSearchNotes(ctx context.Context, op *domain.Operative, tc *domain.ToolCall) (*domain.ToolResult, error) {
	query, _ := tc.Input["query"].(string)

	notes, err := c.notes.KeywordSearch(ctx, op.ID, query)
	if err != nil {
		return nil, fmt.Errorf("keyword search: %w", err)
	}

	var refs []domain.NoteRef
	for _, n := range notes {
		refs = append(refs, domain.NoteRef{ID: n.ID, Title: n.Title})
	}

	b, _ := json.Marshal(refs)
	return &domain.ToolResult{
		ToolCallID: tc.ID,
		Content:    string(b),
	}, nil
}

// toolVectorSearchNotes searches notes by semantic similarity.
func (c *Controller) toolVectorSearchNotes(ctx context.Context, op *domain.Operative, tc *domain.ToolCall) (*domain.ToolResult, error) {
	query, _ := tc.Input["query"].(string)

	notes, err := c.notes.VectorSearch(ctx, op.ID, query)
	if err != nil {
		return &domain.ToolResult{
			ToolCallID: tc.ID,
			Content:    fmt.Sprintf("Vector search not available: %v", err),
			IsError:    true,
		}, nil
	}

	var refs []domain.NoteRef
	for _, n := range notes {
		refs = append(refs, domain.NoteRef{ID: n.ID, Title: n.Title})
	}

	b, _ := json.Marshal(refs)
	return &domain.ToolResult{
		ToolCallID: tc.ID,
		Content:    string(b),
	}, nil
}

// toolGetNote retrieves a note by ID.
func (c *Controller) toolGetNote(ctx context.Context, op *domain.Operative, tc *domain.ToolCall) (*domain.ToolResult, error) {
	id, _ := tc.Input["id"].(string)

	note, err := c.notes.GetNote(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("getting note: %w", err)
	}

	b, _ := json.Marshal(note)
	return &domain.ToolResult{
		ToolCallID: tc.ID,
		Content:    string(b),
	}, nil
}

// toolDeleteNote deletes a note by ID.
func (c *Controller) toolDeleteNote(ctx context.Context, op *domain.Operative, tc *domain.ToolCall) (*domain.ToolResult, error) {
	id, _ := tc.Input["id"].(string)

	if err := c.notes.DeleteNote(ctx, id); err != nil {
		return nil, fmt.Errorf("deleting note: %w", err)
	}

	return &domain.ToolResult{
		ToolCallID: tc.ID,
		Content:    "Note deleted.",
	}, nil
}

// controllerDelegate implements sandbox.Delegate for the controller.
type controllerDelegate struct {
	ctx  context.Context
	ctrl *Controller
	op   *domain.Operative
}

var _ sandbox.Delegate = (*controllerDelegate)(nil)

func (d *controllerDelegate) PromptModel(ctx context.Context, prompt string) (string, error) {
	msgs := []domain.StreamEntry{
		{Role: domain.RoleUser, ContentType: domain.ContentTypeText, Content: prompt},
	}
	messages := entriesToMessages(msgs)

	stream, err := d.ctrl.provider.Stream(ctx, d.op.Model, "", messages)
	if err != nil {
		return "", err
	}
	defer stream.Close()

	msg, err := stream.FullMessage()
	if err != nil {
		return "", err
	}

	for _, c := range msg.Content {
		if c.Type == domain.ContentTypeText {
			return c.Text, nil
		}
	}
	return "", nil
}

func (d *controllerDelegate) PromptSelf(ctx context.Context, message string) error {
	return d.ctrl.stream.Append(ctx, &domain.StreamEntry{
		ID:          uuid.New().String(),
		OperativeID: d.op.ID,
		Role:        domain.RoleSystem,
		ContentType: domain.ContentTypeText,
		Content:     message,
	})
}
