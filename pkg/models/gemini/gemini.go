package gemini

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"strings"

	"github.com/google/generative-ai-go/genai"
	"github.com/mariozechner/coding-agent/session/pkg/models"
	"github.com/mariozechner/coding-agent/session/pkg/session"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

const (
	// LevelTrace is a custom log level for detailed HTTP traffic.
	LevelTrace = slog.Level(-8)
)

// GeminiModel implements models.ModelProvider using the Google Gemini API.
type GeminiModel struct {
	client *genai.Client
}

// New creates a new GeminiModel.
func New(ctx context.Context, apiKey string) (*GeminiModel, error) {
	httpClient := &http.Client{
		Transport: &loggingTransport{
			base:   http.DefaultTransport,
			apiKey: apiKey,
		},
	}
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey), option.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("failed to create gemai client: %w", err)
	}
	return &GeminiModel{client: client}, nil
}

type loggingTransport struct {
	base   http.RoundTripper
	apiKey string
}

func (t *loggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// If API key is provided and not already in headers/query, add it.
	// We do this because passing a custom http.Client often bypasses
	// the library's automatic API key injection.
	if t.apiKey != "" && req.Header.Get("x-goog-api-key") == "" && req.URL.Query().Get("key") == "" {
		req = req.Clone(req.Context())
		req.Header.Set("x-goog-api-key", t.apiKey)
	}

	if !slog.Default().Enabled(req.Context(), LevelTrace) {
		return t.base.RoundTrip(req)
	}

	// Dump request
	reqDump, err := httputil.DumpRequestOut(req, true)
	if err != nil {
		slog.Debug("Failed to dump Gemini request", "error", err)
	} else {
		slog.Debug("Gemini REST Request", "url", req.URL.String(), "dump", string(reqDump))
	}

	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	// Dump response
	// For streaming, don't dump body to avoid consuming it/blocking.
	// Gemini streaming uses alt=sse or Content-Type: text/event-stream.
	isStream := strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") ||
		strings.Contains(req.URL.Query().Get("alt"), "sse")

	respDump, err := httputil.DumpResponse(resp, !isStream)
	if err != nil {
		slog.Debug("Failed to dump Gemini response", "error", err)
	} else {
		slog.Debug("Gemini REST Response", "isStream", isStream, "dump", string(respDump))
	}

	return resp, nil
}

// Close releases resources.
func (m *GeminiModel) Close() {
	m.client.Close()
}

// List returns available models.
func (m *GeminiModel) List(ctx context.Context) ([]string, error) {
	iter := m.client.ListModels(ctx)
	var names []string
	for {
		model, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		slog.Debug("Found Gemini model", "name", model.Name)
		// model.Name typically looks like "models/gemini-pro"
		// We might want to strip "models/" prefix or keep it depending on what Stream needs.
		// The error message "models/gemini-1.5-flash is not found" implies it was looked up AS "models/gemini-1.5-flash".
		// If we pass "gemini-1.5-flash" to GenerativeModel, the client might use it as is.
		// Let's just return the raw name from the API for now to see what they look like.
		names = append(names, model.Name)
	}
	return names, nil
}

// Stream sends a context to the LLM and returns a stream.
func (m *GeminiModel) Stream(ctx context.Context, modelName string, messages []models.AgentMessage) (models.ModelStream, error) {
	slog.Debug("Gemini.Stream: Request Parameters", "model", modelName, "messageCount", len(messages))
	gm := m.client.GenerativeModel(modelName)

	// Convert AgentMessages to genai.Content
	var genaiHistory []*genai.Content

	for _, msg := range messages {
		var parts []genai.Part
		for _, c := range msg.Content {
			switch c.Type {
			case session.ContentTypeText:
				parts = append(parts, genai.Text(c.Text.Content))
			case session.ContentTypeToolUse:
				// Map tool calls if we had them defined in the model.
			case session.ContentTypeToolResult:
				// Map tool results
			}
		}

		role := "user"
		if msg.Role == session.RoleAssistant {
			role = "model" // Gemini uses 'model' for assistant
		}

		if len(parts) > 0 {
			genaiHistory = append(genaiHistory, &genai.Content{
				Role:  role,
				Parts: parts,
			})
		}
	}

	slog.Debug("Converted messages to GenAI history", "historyLen", len(genaiHistory))
	for i, h := range genaiHistory {
		slog.Debug("History item", "index", i, "role", h.Role, "partsLen", len(h.Parts))
	}

	cs := gm.StartChat()
	if len(genaiHistory) > 0 {
		cs.History = genaiHistory[:len(genaiHistory)-1]
	}

	lastMsg := messages[len(messages)-1]
	var lastParts []genai.Part
	for _, c := range lastMsg.Content {
		if c.Type == session.ContentTypeText {
			lastParts = append(lastParts, genai.Text(c.Text.Content))
		}
	}

	slog.Debug("Sending message stream", "lastPartsLen", len(lastParts))
	for i, p := range lastParts {
		if txt, ok := p.(genai.Text); ok {
			slog.Debug("Part", "index", i, "text", string(txt))
		}
	}

	iter := cs.SendMessageStream(ctx, lastParts...)
	return &geminiStream{iter: iter}, nil
}

// geminiStream wrapper
type geminiStream struct {
	iter *genai.GenerateContentResponseIterator
}

func (s *geminiStream) FullMessage() (models.AgentMessage, error) {
	var fullText strings.Builder

	slog.Debug("Aggregating Gemini response stream")

	// Iterate through the stream to aggregate response
	for {
		resp, err := s.iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return models.AgentMessage{}, err
		}

		for _, cand := range resp.Candidates {
			if cand.Content != nil {
				for _, part := range cand.Content.Parts {
					if txt, ok := part.(genai.Text); ok {
						fullText.WriteString(string(txt))
					}
				}
			}
		}
	}

	msg := models.AgentMessage{
		Role: session.RoleAssistant,
		Content: []session.Content{
			{
				Type: session.ContentTypeText,
				Text: &session.TextContent{Content: fullText.String()},
			},
		},
	}

	slog.Debug("Gemini.FullMessage: Response Struct", "role", msg.Role, "contentLen", len(fullText.String()))

	return msg, nil
}

func (s *geminiStream) Close() error {
	return nil
}
