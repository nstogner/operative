package gemini_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nstogner/operative/pkg/domain"
	"github.com/nstogner/operative/pkg/model"
	"github.com/nstogner/operative/pkg/model/gemini"
)

func setupProvider(t *testing.T) *gemini.Provider {
	t.Helper()
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		t.Skip("Skipping: GEMINI_API_KEY not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	provider, err := gemini.New(ctx, apiKey)
	if err != nil {
		t.Fatalf("gemini.New: %v", err)
	}
	return provider
}

// TestIntegrationGeminiName verifies the provider name.
func TestIntegrationGeminiName(t *testing.T) {
	p := setupProvider(t)
	if p.Name() != "gemini" {
		t.Errorf("Name() = %q, want %q", p.Name(), "gemini")
	}
}

// TestIntegrationGeminiListModels verifies that List returns available models.
func TestIntegrationGeminiListModels(t *testing.T) {
	p := setupProvider(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	models, err := p.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("No models found")
	}

	// Verify model structure.
	for _, m := range models {
		if m.ID == "" {
			t.Error("Model has empty ID")
		}
		if m.Provider != "gemini" {
			t.Errorf("Model %s has provider %q, want %q", m.ID, m.Provider, "gemini")
		}
		t.Logf("Model: %s (%s) maxTokens=%d", m.ID, m.Name, m.MaxTokens)
	}
}

// TestIntegrationGeminiStreamBasic verifies a simple text response from the model.
func TestIntegrationGeminiStreamBasic(t *testing.T) {
	p := setupProvider(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	msgs := []model.Message{
		{
			Role: domain.RoleUser,
			Content: []model.Content{
				{Type: domain.ContentTypeText, Text: "Reply with exactly: HELLO"},
			},
		},
	}

	stream, err := p.Stream(ctx, "gemini-2.0-flash", "", msgs)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer stream.Close()

	resp, err := stream.FullMessage()
	if err != nil {
		t.Fatalf("FullMessage: %v", err)
	}

	if resp.Role != domain.RoleAssistant {
		t.Errorf("Role = %q, want %q", resp.Role, domain.RoleAssistant)
	}
	if len(resp.Content) == 0 {
		t.Fatal("Response has no content")
	}
	if resp.Content[0].Type != domain.ContentTypeText {
		t.Errorf("Content type = %q, want %q", resp.Content[0].Type, domain.ContentTypeText)
	}
	if resp.Content[0].Text == "" {
		t.Error("Response text is empty")
	}

	t.Logf("Response: %s", resp.Content[0].Text)
}

// TestIntegrationGeminiStreamWithSystemInstruction verifies system instructions work.
func TestIntegrationGeminiStreamWithSystemInstruction(t *testing.T) {
	p := setupProvider(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	msgs := []model.Message{
		{
			Role: domain.RoleUser,
			Content: []model.Content{
				{Type: domain.ContentTypeText, Text: "What is your name?"},
			},
		},
	}

	instructions := "You are a helpful assistant named TestBot. Always introduce yourself by name."
	stream, err := p.Stream(ctx, "gemini-2.0-flash", instructions, msgs)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer stream.Close()

	resp, err := stream.FullMessage()
	if err != nil {
		t.Fatalf("FullMessage: %v", err)
	}

	text := resp.Content[0].Text
	if !strings.Contains(strings.ToLower(text), "testbot") {
		t.Errorf("Expected 'TestBot' in response, got: %s", text)
	}
	t.Logf("Response: %s", text)
}

// TestIntegrationGeminiStreamToolCall verifies the model can request a tool call.
func TestIntegrationGeminiStreamToolCall(t *testing.T) {
	p := setupProvider(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	msgs := []model.Message{
		{
			Role: domain.RoleUser,
			Content: []model.Content{
				{Type: domain.ContentTypeText, Text: "Please calculate 9*9 using the ipython tool."},
			},
		},
	}

	stream, err := p.Stream(ctx, "gemini-2.0-flash", "Use the run_ipython_cell tool to execute code when asked to calculate.", msgs)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer stream.Close()

	resp, err := stream.FullMessage()
	if err != nil {
		t.Fatalf("FullMessage: %v", err)
	}

	// Look for a tool call in the response.
	foundToolCall := false
	for _, c := range resp.Content {
		if c.Type == domain.ContentTypeToolCall && c.ToolCall != nil {
			foundToolCall = true
			t.Logf("Tool call: %s (id=%s) args=%v", c.ToolCall.Name, c.ToolCall.ID, c.ToolCall.Input)
			if c.ToolCall.Name != "run_ipython_cell" {
				t.Errorf("Expected tool name %q, got %q", "run_ipython_cell", c.ToolCall.Name)
			}
		}
	}
	if !foundToolCall {
		// The model might respond with text instead; log what we got.
		for _, c := range resp.Content {
			t.Logf("Content: type=%s text=%q", c.Type, c.Text)
		}
		t.Error("Expected a tool call but none were returned")
	}
}

// TestIntegrationGeminiMultiTurn verifies multi-turn conversation works.
func TestIntegrationGeminiMultiTurn(t *testing.T) {
	p := setupProvider(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	msgs := []model.Message{
		{
			Role:    domain.RoleUser,
			Content: []model.Content{{Type: domain.ContentTypeText, Text: "Remember: the secret word is BANANA."}},
		},
		{
			Role:    domain.RoleAssistant,
			Content: []model.Content{{Type: domain.ContentTypeText, Text: "Got it. The secret word is BANANA."}},
		},
		{
			Role:    domain.RoleUser,
			Content: []model.Content{{Type: domain.ContentTypeText, Text: "What is the secret word?"}},
		},
	}

	stream, err := p.Stream(ctx, "gemini-2.0-flash", "", msgs)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer stream.Close()

	resp, err := stream.FullMessage()
	if err != nil {
		t.Fatalf("FullMessage: %v", err)
	}

	text := resp.Content[0].Text
	if !strings.Contains(strings.ToUpper(text), "BANANA") {
		t.Errorf("Expected 'BANANA' in response, got: %s", text)
	}
	t.Logf("Response: %s", text)
}
