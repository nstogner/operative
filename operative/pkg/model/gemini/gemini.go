package gemini

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/nstogner/operative/pkg/domain"
	"github.com/nstogner/operative/pkg/model"
	"google.golang.org/genai"
)

// Provider implements model.Provider using the Google Gen AI SDK.
type Provider struct {
	client *genai.Client
}

// Verify interface compliance.
var _ model.Provider = (*Provider)(nil)

// New creates a new Gemini provider.
func New(ctx context.Context, apiKey string) (*Provider, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: apiKey,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create genai client: %w", err)
	}
	return &Provider{client: client}, nil
}

// Name returns the provider identifier.
func (p *Provider) Name() string { return "gemini" }

// List returns available Gemini models.
func (p *Provider) List(ctx context.Context) ([]domain.Model, error) {
	var models []domain.Model
	for m, err := range p.client.Models.All(ctx) {
		if err != nil {
			return nil, err
		}

		// Filter for models that support generateContent.
		supportsGenerate := false
		if !strings.Contains(strings.ToLower(m.Name), "gemma") {
			for _, action := range m.SupportedActions {
				if action == "generateContent" {
					supportsGenerate = true
					break
				}
			}
		}

		if supportsGenerate {
			maxTokens := 0
			if m.InputTokenLimit > 0 {
				maxTokens = int(m.InputTokenLimit)
			}
			models = append(models, domain.Model{
				ID:        m.Name,
				Name:      m.DisplayName,
				Provider:  "gemini",
				MaxTokens: maxTokens,
			})
		}
	}
	return models, nil
}

// Stream sends a conversation context to the LLM and returns a stream.
func (p *Provider) Stream(ctx context.Context, modelName, instructions string, messages []model.Message) (model.ModelStream, error) {
	slog.Debug("Gemini.Stream", "model", modelName, "messageCount", len(messages))

	// Build tool declarations from the tools provided.
	tools := buildToolDeclarations()

	// Convert messages to genai.Content.
	var contents []*genai.Content
	var systemInstruction *genai.Content
	toolNameMap := make(map[string]string) // tool call ID -> name

	if instructions != "" {
		systemInstruction = &genai.Content{
			Parts: []*genai.Part{{Text: instructions}},
		}
	}

	for _, msg := range messages {
		if msg.Role == domain.RoleSystem || msg.Role == domain.RoleCompactionSummary {
			// System role is handled via instructions; compaction summaries are treated as assistant context.
			if msg.Role == domain.RoleCompactionSummary {
				contents = append(contents, &genai.Content{
					Role:  "model",
					Parts: []*genai.Part{{Text: msg.Content[0].Text}},
				})
			}
			continue
		}

		var parts []*genai.Part
		for _, c := range msg.Content {
			switch c.Type {
			case domain.ContentTypeText:
				parts = append(parts, &genai.Part{
					Text:             c.Text,
					ThoughtSignature: c.ThoughtSignature,
				})
			case domain.ContentTypeToolCall:
				if c.ToolCall != nil {
					toolNameMap[c.ToolCall.ID] = c.ToolCall.Name
					parts = append(parts, &genai.Part{
						FunctionCall: &genai.FunctionCall{
							Name: c.ToolCall.Name,
							Args: c.ToolCall.Input,
							ID:   c.ToolCall.ID,
						},
						ThoughtSignature: c.ThoughtSignature,
					})
				}
			case domain.ContentTypeToolResult:
				if c.ToolResult != nil {
					name := toolNameMap[c.ToolResult.ToolCallID]
					if name == "" {
						name = "run_ipython_cell"
					}
					parts = append(parts, &genai.Part{
						FunctionResponse: &genai.FunctionResponse{
							Name: name,
							ID:   c.ToolResult.ToolCallID,
							Response: map[string]any{
								"result": c.ToolResult.Content,
							},
						},
					})
				}
			}
		}

		role := "user"
		if msg.Role == domain.RoleAssistant {
			role = "model"
		} else if msg.Role == domain.RoleTool {
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
		Tools:             tools,
		SystemInstruction: systemInstruction,
	}

	streamCtx, cancel := context.WithCancel(ctx)
	iter := p.client.Models.GenerateContentStream(streamCtx, modelName, contents, config)

	return &geminiStream{
		iter:   iter,
		cancel: cancel,
	}, nil
}

func buildToolDeclarations() []*genai.Tool {
	return []*genai.Tool{
		{
			FunctionDeclarations: []*genai.FunctionDeclaration{
				{
					Name:        "run_ipython_cell",
					Description: "Run a cell of code in the IPython kernel. Returns the result.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"code": {Type: genai.TypeString, Description: "The code to run."},
						},
						Required: []string{"code"},
					},
				},
				{
					Name:        "update_instructions",
					Description: "Update the operative's self-set instructions.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"instructions": {Type: genai.TypeString, Description: "The new instructions."},
						},
						Required: []string{"instructions"},
					},
				},
				{
					Name:        "store_note",
					Description: "Store a searchable note.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"title":   {Type: genai.TypeString, Description: "The note title."},
							"content": {Type: genai.TypeString, Description: "The note content."},
						},
						Required: []string{"title", "content"},
					},
				},
				{
					Name:        "keyword_search_notes",
					Description: "Search notes by keyword.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"query": {Type: genai.TypeString, Description: "The search query."},
						},
						Required: []string{"query"},
					},
				},
				{
					Name:        "vector_search_notes",
					Description: "Search notes by semantic similarity (vector search).",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"query": {Type: genai.TypeString, Description: "The search query."},
						},
						Required: []string{"query"},
					},
				},
				{
					Name:        "get_note",
					Description: "Retrieve a note by its ID.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"id": {Type: genai.TypeString, Description: "The note ID."},
						},
						Required: []string{"id"},
					},
				},
				{
					Name:        "delete_note",
					Description: "Delete a note by its ID.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"id": {Type: genai.TypeString, Description: "The note ID."},
						},
						Required: []string{"id"},
					},
				},
			},
		},
	}
}

// geminiStream wraps the Gemini streaming iterator.
type geminiStream struct {
	iter   func(yield func(*genai.GenerateContentResponse, error) bool)
	cancel context.CancelFunc
}

func (s *geminiStream) FullMessage() (model.Message, error) {
	var fullText strings.Builder
	var toolCalls []model.Content
	var textSignature []byte

	for resp, err := range s.iter {
		if err != nil {
			return model.Message{}, err
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
						id := fc.ID
						if id == "" {
							id = "call-" + uuid.New().String()
						}
						toolCalls = append(toolCalls, model.Content{
							Type: domain.ContentTypeToolCall,
							ToolCall: &domain.ToolCall{
								ID:    id,
								Name:  fc.Name,
								Input: fc.Args,
							},
							ThoughtSignature: part.ThoughtSignature,
						})
					}
				}
			}
		}
	}

	var content []model.Content
	if fullText.Len() > 0 {
		content = append(content, model.Content{
			Type:             domain.ContentTypeText,
			Text:             fullText.String(),
			ThoughtSignature: textSignature,
		})
	}
	content = append(content, toolCalls...)

	return model.Message{
		Role:    domain.RoleAssistant,
		Content: content,
	}, nil
}

func (s *geminiStream) Close() error {
	s.cancel()
	return nil
}
