package gemini_test

import (
	"context"
	"os"
	"testing"

	"github.com/mariozechner/coding-agent/session/pkg/models"
	"github.com/mariozechner/coding-agent/session/pkg/models/gemini"
	"github.com/mariozechner/coding-agent/session/pkg/store"
)

func TestGemini_Integration(t *testing.T) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		t.Skip("GEMINI_API_KEY not set")
	}

	ctx := context.Background()
	modelProvider, err := gemini.New(ctx, apiKey)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	// 1. List Models
	modelsList, err := modelProvider.List(ctx)
	if err != nil {
		t.Fatalf("Failed to list models: %v", err)
	}
	t.Logf("Available models: %v", modelsList)

	// 2. Test specific model from the issue
	// User session file said: "model":"models/gemini-2.5-flash-image"

	// Let's try to infer if "models/gemini-2.5-flash-image" is valid.
	// We'll stream hello.

	msgs := []models.AgentMessage{
		{
			Role: store.RoleUser,
			Content: []store.Content{
				{Type: store.ContentTypeText, Text: &store.TextContent{Content: "Hello"}},
			},
		},
	}

	t.Run("GenerateContent", func(t *testing.T) {
		// Use a likely valid model first to verify valid key/connectivity
		validModel := "gemini-2.0-flash-exp"
		// Check if validModel is in listing
		found := false
		for _, m := range modelsList {
			if m == "models/"+validModel || m == validModel {
				found = true
				break
			}
		}
		if !found {
			t.Logf("Warning: %s not found in list, trying anyway (might be alias)", validModel)
		}

		stream, err := modelProvider.Stream(ctx, validModel, "", msgs)
		if err != nil {
			t.Fatalf("Stream failed for %s: %v", validModel, err)
		}
		defer stream.Close()

		msg, err := stream.FullMessage()
		if err != nil {
			t.Fatalf("FullMessage failed: %v", err)
		}
		t.Logf("Response from %s: %s", validModel, msg.Content[0].Text.Content)
	})

	t.Run("GenerateContent_SuspiciousModel", func(t *testing.T) {
		// The store file bad 'models/gemini-2.5-flash-image'
		// The Runner usually prepends 'models/' or the provider handles it?
		// Logic in gemini.New doesn't seem to mangle names.

		// Try exact string from session
		modelName := "models/gemini-2.5-flash-image"

		stream, err := modelProvider.Stream(ctx, modelName, "", msgs)
		if err != nil {
			t.Logf("Stream creation failed for %s: %v (Expected if model invalid)", modelName, err)
			return
		}
		defer stream.Close()

		msg, err := stream.FullMessage()
		if err != nil {
			t.Logf("FullMessage failed for %s: %v (Expected if model invalid)", modelName, err)
		} else {
			t.Logf("Response from %s: %s", modelName, msg.Content[0].Text.Content)
		}
	})
}
