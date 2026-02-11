package gemini_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/mariozechner/coding-agent/session/pkg/models"
	"github.com/mariozechner/coding-agent/session/pkg/models/gemini"
	"github.com/mariozechner/coding-agent/session/pkg/store"
)

func TestIntegration_Gemini(t *testing.T) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		t.Skip("Skipping Gemini integration test: GEMINI_API_KEY not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1. Initialize
	model, err := gemini.New(ctx, apiKey)
	if err != nil {
		t.Fatalf("Failed to create model: %v", err)
	}
	defer model.Close()

	// 2. List Models
	t.Log("Listing models...")
	modelsList, err := model.List(ctx)
	if err != nil {
		t.Fatalf("Failed to list models: %v", err)
	}
	if len(modelsList) == 0 {
		t.Fatal("No models found")
	}

	for _, name := range modelsList {
		t.Logf("Found Model: %s", name)
	}

	targetModel := modelsList[0]
	t.Logf("Attempting to use model: %s", targetModel)

	// 4. Stream Call
	msgs := []models.AgentMessage{
		{
			Role: store.RoleUser,
			Content: []store.Content{
				{Type: store.ContentTypeText, Text: &store.TextContent{Content: "Hello, just verify you work."}},
			},
		},
	}

	stream, err := model.Stream(ctx, targetModel, "", msgs)
	if err != nil {
		t.Fatalf("Stream creation failed: %v", err)
	}
	defer stream.Close()

	resp, err := stream.FullMessage()
	if err != nil {
		t.Fatalf("FullMessage failed: %v", err)
	}

	if len(resp.Content) > 0 {
		if resp.Content[0].Text != nil {
			t.Logf("Response: %v", resp.Content[0].Text.Content)
		} else {
			t.Logf("Response content type: %s", resp.Content[0].Type)
		}
	} else {
		t.Log("Response empty")
	}
}
