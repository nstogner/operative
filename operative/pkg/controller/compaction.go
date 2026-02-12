package controller

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/nstogner/operative/pkg/domain"
	"github.com/nstogner/operative/pkg/model"
)

const (
	// DefaultCompactionThreshold is the fraction of the max context window at which
	// the stream should be compacted. 0.6 means compact when usage reaches 60%.
	DefaultCompactionThreshold = 0.6
)

// checkAndCompact checks whether the stream needs compaction and triggers it if so.
// Compaction removes older entries and replaces them with a model-generated summary.
func (c *Controller) checkAndCompact(ctx context.Context, op *domain.Operative, entries []domain.StreamEntry) error {
	if len(entries) < 10 {
		// Don't bother compacting very short streams.
		return nil
	}

	threshold := op.CompactionThreshold
	if threshold <= 0 {
		threshold = DefaultCompactionThreshold
	}

	// Estimate token usage (rough heuristic: ~4 chars per token).
	totalChars := 0
	for _, e := range entries {
		totalChars += len(e.Content)
	}
	estimatedTokens := totalChars / 4

	// Look up the model to get max context window.
	models, err := c.provider.List(ctx)
	if err != nil {
		return fmt.Errorf("listing models for compaction check: %w", err)
	}

	maxTokens := 0
	for _, m := range models {
		if m.ID == op.Model {
			maxTokens = m.MaxTokens
			break
		}
	}
	if maxTokens == 0 {
		// Can't determine context window, skip compaction.
		return nil
	}

	if float64(estimatedTokens) < float64(maxTokens)*threshold {
		// Under threshold, no compaction needed.
		return nil
	}

	slog.Info("Stream compaction triggered",
		"operativeID", op.ID,
		"estimatedTokens", estimatedTokens,
		"maxTokens", maxTokens,
		"threshold", threshold,
	)

	return c.compact(ctx, op, entries)
}

// compact performs stream compaction by asking the model to summarize older entries.
func (c *Controller) compact(ctx context.Context, op *domain.Operative, entries []domain.StreamEntry) error {
	// Find a safe compaction point: around 50% of entries from the beginning,
	// but never split in the middle of a tool_call/tool_result pair.
	splitIdx := len(entries) / 2
	for splitIdx > 0 {
		entry := entries[splitIdx]
		// Don't split right after a tool call (before the result).
		if entry.ContentType == domain.ContentTypeToolCall {
			splitIdx--
			continue
		}
		// Don't split on a tool result (keep it with its call).
		if entry.Role == domain.RoleTool {
			splitIdx--
			continue
		}
		break
	}

	if splitIdx <= 1 {
		// Not enough entries to compact.
		return nil
	}

	entriesToCompact := entries[:splitIdx]

	// Use the compaction model (or main model if not specified).
	compactionModel := op.CompactionModel
	if compactionModel == "" {
		compactionModel = op.Model
	}

	// Build the compaction prompt.
	prompt := "You are summarizing a conversation history for context compaction. " +
		"Create a dense, comprehensive summary of the following conversation that preserves:\n" +
		"- Key decisions and outcomes\n" +
		"- Important code/files that were created or modified\n" +
		"- Current state of any ongoing tasks\n" +
		"- Any instructions or preferences the user expressed\n\n" +
		"Be thorough but concise. This summary will replace the original messages.\n\n" +
		"CONVERSATION TO SUMMARIZE:\n"

	for _, e := range entriesToCompact {
		prompt += fmt.Sprintf("[%s] %s\n", e.Role, e.Content)
	}

	// Call the model to generate the summary.
	messages := []model.Message{
		{
			Role:    domain.RoleUser,
			Content: []model.Content{{Type: domain.ContentTypeText, Text: prompt}},
		},
	}

	stream, err := c.provider.Stream(ctx, compactionModel, "You are a conversation summarizer.", messages)
	if err != nil {
		return fmt.Errorf("calling model for compaction: %w", err)
	}
	defer stream.Close()

	msg, err := stream.FullMessage()
	if err != nil {
		return fmt.Errorf("getting compaction summary: %w", err)
	}

	summary := ""
	for _, content := range msg.Content {
		if content.Type == domain.ContentTypeText {
			summary = content.Text
			break
		}
	}

	if summary == "" {
		return fmt.Errorf("model returned empty compaction summary")
	}

	// Append the compaction summary entry. Old entries remain immutable in the DB
	// but GetEntries will now return entries starting from this compaction entry.
	return c.stream.Compact(ctx, op.ID, summary)
}
