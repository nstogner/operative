package gemini

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"strings"

	"github.com/google/generative-ai-go/genai"
	"github.com/google/uuid"
	"github.com/mariozechner/coding-agent/session/pkg/models"
	"github.com/mariozechner/coding-agent/session/pkg/sandbox"
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
		names = append(names, model.Name)
	}
	return names, nil
}

// Stream sends a context to the LLM and returns a stream.
func (m *GeminiModel) Stream(ctx context.Context, modelName string, messages []models.AgentMessage) (models.ModelStream, error) {
	slog.Debug("Gemini.Stream: Request Parameters", "model", modelName, "messageCount", len(messages))
	gm := m.client.GenerativeModel(modelName)

	// Configure Tools
	gm.Tools = []*genai.Tool{
		{
			FunctionDeclarations: []*genai.FunctionDeclaration{
				{
					Name:        sandbox.ToolNameRunIPythonCell,
					Description: "Run a cell of code in the IPython kernel. Returns the result of running the cell.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"code": {
								Type:        genai.TypeString,
								Description: "The code to run.",
							},
						},
						Required: []string{"code"},
					},
				},
			},
		},
	}

	// Convert AgentMessages to genai.Content
	var genaiHistory []*genai.Content

	for _, msg := range messages {
		var parts []genai.Part
		for _, c := range msg.Content {
			switch c.Type {
			case session.ContentTypeText:
				parts = append(parts, genai.Text(c.Text.Content))
			case session.ContentTypeToolUse:
				parts = append(parts, genai.FunctionCall{
					Name: c.ToolUse.Name,
					Args: c.ToolUse.Input,
				})
			case session.ContentTypeToolResult:
				parts = append(parts, genai.FunctionResponse{
					Response: map[string]any{
						"result": c.ToolResult.Content,
					},
				})
			}
		}

		role := "user"
		if msg.Role == session.RoleAssistant {
			role = "model"
		}
		// FunctionResponse must be 'function' role in some APIs, but Gemini uses 'user' for function results usually.
		// Actually, standard is: User sends FunctionCall -> Model. Model sends FunctionCall steps. User sends FunctionResponse.
		// So FunctionResponse role is indeed 'user' contextually (or 'function' if supported).
		// The Go SDK docs say: "For FunctionResponse, use "user" role or "function" role?"
		// Most examples use 'user' for function responses.
		// If the message contains ToolResult, it's effectively from the 'environment' (User side).
		if msg.Role == session.RoleTool {
			role = "user"
		}

		if len(parts) > 0 {
			genaiHistory = append(genaiHistory, &genai.Content{
				Role:  role,
				Parts: parts,
			})
		}
	}

	// Create chat session
	cs := gm.StartChat()
	if len(genaiHistory) > 0 {
		// All but last
		cs.History = genaiHistory[:len(genaiHistory)-1]
	}

	lastMsg := messages[len(messages)-1]
	// Convert last message parts
	var lastParts []genai.Part
	// Same conversion logic for the new message
	// Usually the last message is just Text (User query) or ToolResult.
	for _, c := range lastMsg.Content {
		switch c.Type {
		case session.ContentTypeText:
			lastParts = append(lastParts, genai.Text(c.Text.Content))
		case session.ContentTypeToolUse:
			lastParts = append(lastParts, genai.FunctionCall{
				Name: c.ToolUse.Name,
				Args: c.ToolUse.Input,
			})
		case session.ContentTypeToolResult:
			lastParts = append(lastParts, genai.FunctionResponse{
				Name: sandbox.ToolNameRunIPythonCell,
				Response: map[string]any{
					"result": c.ToolResult.Content,
				},
			})
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
	var toolCalls []session.Content

	slog.Debug("Aggregating Gemini response stream")

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
					} else if fc, ok := part.(genai.FunctionCall); ok {
						toolCalls = append(toolCalls, session.Content{
							Type: session.ContentTypeToolUse,
							ToolUse: &session.ToolUseContent{
								ID:    "call-" + uuid.New().String(), // Generate ID
								Name:  fc.Name,
								Input: fc.Args,
							},
						})
					}
				}
			}
		}
	}

	content := []session.Content{}
	if fullText.Len() > 0 {
		content = append(content, session.Content{
			Type: session.ContentTypeText,
			Text: &session.TextContent{Content: fullText.String()},
		})
	}
	content = append(content, toolCalls...)

	msg := models.AgentMessage{
		Role:    session.RoleAssistant,
		Content: content,
	}

	return msg, nil
}

func (s *geminiStream) Close() error {
	return nil
}
