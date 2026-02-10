package gemini

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"strings"

	"github.com/google/uuid"
	"github.com/mariozechner/coding-agent/session/pkg/models"
	"github.com/mariozechner/coding-agent/session/pkg/sandbox"
	"github.com/mariozechner/coding-agent/session/pkg/store"
	"google.golang.org/genai"
)

const (
	// LevelTrace is a custom log level for detailed HTTP traffic.
	LevelTrace = slog.Level(-8)
)

// GeminiModel implements models.ModelProvider using the Google Gen AI SDK.
type GeminiModel struct {
	client *genai.Client
}

// New creates a new GeminiModel using the new google.golang.org/genai SDK.
func New(ctx context.Context, apiKey string) (*GeminiModel, error) {
	httpClient := &http.Client{
		Transport: &loggingTransport{
			base:   http.DefaultTransport,
			apiKey: apiKey,
		},
	}

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:     apiKey,
		HTTPClient: httpClient,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create genai client: %w", err)
	}
	return &GeminiModel{client: client}, nil
}

type loggingTransport struct {
	base   http.RoundTripper
	apiKey string
}

func (t *loggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Add API key if missing (genai SDK usually handles this but we keep it for robustness with custom client)
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
	// genai.Client doesn't have a Close() method in the same way, but it's good practice
}

// List returns available models.
func (m *GeminiModel) List(ctx context.Context) ([]string, error) {
	var names []string
	for model, err := range m.client.Models.All(ctx) {
		if err != nil {
			return nil, err
		}

		// Filter for models that support generateContent
		supportsGenerate := false
		if !strings.Contains(strings.ToLower(model.Name), "gemma") {
			for _, action := range model.SupportedActions {
				if action == "generateContent" {
					supportsGenerate = true
					break
				}
			}
		}

		if supportsGenerate {
			slog.Debug("Found Gemini model", "name", model.Name)
			names = append(names, model.Name)
		}
	}
	return names, nil
}

// Stream sends a context to the LLM and returns a stream.
func (m *GeminiModel) Stream(ctx context.Context, modelName string, messages []models.AgentMessage) (models.ModelStream, error) {
	slog.Debug("Gemini.Stream: Request Parameters", "model", modelName, "messageCount", len(messages))

	// Configure Tools
	tools := []*genai.Tool{
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
	var contents []*genai.Content
	toolMap := make(map[string]string)

	for _, msg := range messages {
		var parts []*genai.Part
		for _, c := range msg.Content {
			switch c.Type {
			case store.ContentTypeText:
				parts = append(parts, &genai.Part{
					Text:             c.Text.Content,
					ThoughtSignature: c.Text.ThoughtSignature,
				})
			case store.ContentTypeToolUse:
				toolMap[c.ToolUse.ID] = c.ToolUse.Name
				parts = append(parts, &genai.Part{
					FunctionCall: &genai.FunctionCall{
						Name: c.ToolUse.Name,
						Args: c.ToolUse.Input,
						ID:   c.ToolUse.ID,
					},
					ThoughtSignature: c.ToolUse.ThoughtSignature,
				})
			case store.ContentTypeToolResult:
				name := toolMap[c.ToolResult.ToolUseID]
				if name == "" {
					name = sandbox.ToolNameRunIPythonCell
				}
				parts = append(parts, &genai.Part{
					FunctionResponse: &genai.FunctionResponse{
						Name: name,
						ID:   c.ToolResult.ToolUseID,
						Response: map[string]any{
							"result": c.ToolResult.Content,
						},
					},
				})
			}
		}

		role := "user"
		if msg.Role == store.RoleAssistant {
			role = "model"
		} else if msg.Role == store.RoleTool {
			role = "user"
		}

		if len(parts) > 0 {
			contents = append(contents, &genai.Content{
				Role:  role,
				Parts: parts,
			})
		}
	}

	config := &genai.GenerateContentConfig{
		Tools: tools,
	}

	streamCtx, cancel := context.WithCancel(ctx)
	iter := m.client.Models.GenerateContentStream(streamCtx, modelName, contents, config)

	return &geminiStream{
		iter:   iter,
		cancel: cancel,
	}, nil
}

// geminiStream wrapper
type geminiStream struct {
	iter   func(yield func(*genai.GenerateContentResponse, error) bool)
	cancel context.CancelFunc
}

func (s *geminiStream) FullMessage() (models.AgentMessage, error) {
	var fullText strings.Builder
	var toolCalls []store.Content

	slog.Debug("Aggregating Gemini response stream")

	var textSignature []byte
	for resp, err := range s.iter {
		if err != nil {
			return models.AgentMessage{}, err
		}
		if resp == nil {
			continue
		}

		for _, cand := range resp.Candidates {
			if cand.Content != nil {
				for _, part := range cand.Content.Parts {
					if part.Text != "" {
						if len(part.ThoughtSignature) > 0 {
							textSignature = part.ThoughtSignature
						}
						fullText.WriteString(part.Text)
					}
					if part.FunctionCall != nil {
						fc := part.FunctionCall
						// Ensure the tool call has an ID. If model didn't provide one, generate it.
						id := fc.ID
						if id == "" {
							id = "call-" + uuid.New().String()
						}
						toolCalls = append(toolCalls, store.Content{
							Type: store.ContentTypeToolUse,
							ToolUse: &store.ToolUseContent{
								ID:               id,
								Name:             fc.Name,
								Input:            fc.Args,
								ThoughtSignature: part.ThoughtSignature,
							},
						})
					}
				}
			}
		}
	}

	content := []store.Content{}
	if fullText.Len() > 0 {
		content = append(content, store.Content{
			Type: store.ContentTypeText,
			Text: &store.TextContent{
				Content:          fullText.String(),
				ThoughtSignature: textSignature,
			},
		})
	}
	content = append(content, toolCalls...)

	msg := models.AgentMessage{
		Role:    store.RoleAssistant,
		Content: content,
	}

	return msg, nil
}

func (s *geminiStream) Close() error {
	s.cancel()
	return nil
}
