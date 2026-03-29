package intelligence

import (
	"context"
	"fmt"
	"time"

	"github.com/google/generative-ai-go/genai"
	"github.com/rs/zerolog/log"
	"google.golang.org/api/option"
)

const geminiDefaultModel = "gemini-2.5-flash"

type geminiProvider struct {
	client *genai.Client
	model  string
}

func newGeminiProvider(apiKey, model string) (*geminiProvider, error) {
	if apiKey == "" {
		log.Warn().Msg("GEMINI_API_KEY not set — Gemini provider disabled")
		return &geminiProvider{}, nil
	}
	if model == "" {
		model = geminiDefaultModel
	}

	client, err := genai.NewClient(context.Background(), option.WithAPIKey(apiKey))
	if err != nil {
		log.Error().Err(err).Msg("Failed to create Gemini client — provider disabled")
		return &geminiProvider{}, nil
	}

	log.Info().Str("model", model).Msg("Gemini provider initialised")
	return &geminiProvider{client: client, model: model}, nil
}

func (p *geminiProvider) Available() bool { return p.client != nil }

func (p *geminiProvider) Close() {
	if p.client != nil {
		p.client.Close()
	}
}

func (p *geminiProvider) Complete(ctx context.Context, system, prompt string, cfg ProviderConfig) (string, error) {
	if p.client == nil {
		return "", fmt.Errorf("gemini provider not available")
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	model := p.client.GenerativeModel(p.model)
	model.SetTemperature(cfg.Temperature)
	if cfg.MaxTokens > 0 {
		model.SetMaxOutputTokens(cfg.MaxTokens)
	}
	if system != "" {
		model.SystemInstruction = genai.NewUserContent(genai.Text(system))
	}
	if cfg.JSONMode {
		model.ResponseMIMEType = "application/json"
	}

	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return "", fmt.Errorf("gemini generate failed: %w", err)
	}

	return extractText(resp), nil
}

func (p *geminiProvider) CompleteWithTools(ctx context.Context, system, prompt string, cfg ProviderConfig, tools []ToolDef) (*GenerateResult, error) {
	if p.client == nil {
		return &GenerateResult{}, fmt.Errorf("gemini provider not available")
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	model := p.client.GenerativeModel(p.model)
	model.SetTemperature(cfg.Temperature)
	if cfg.MaxTokens > 0 {
		model.SetMaxOutputTokens(cfg.MaxTokens)
	}
	if system != "" {
		model.SystemInstruction = genai.NewUserContent(genai.Text(system))
	}

	// Convert ToolDef → genai.Tool
	model.Tools = convertToolDefs(tools)

	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return nil, fmt.Errorf("gemini generate with tools failed: %w", err)
	}

	result := &GenerateResult{Text: extractText(resp)}
	for _, cand := range resp.Candidates {
		if cand.Content == nil {
			continue
		}
		for _, part := range cand.Content.Parts {
			if fc, ok := part.(genai.FunctionCall); ok {
				result.ToolCalls = append(result.ToolCalls, ToolCall{
					Name: fc.Name,
					Args: fc.Args,
				})
			}
		}
	}

	return result, nil
}

// convertToolDefs converts provider-agnostic ToolDefs to Gemini's genai.Tool format.
func convertToolDefs(tools []ToolDef) []*genai.Tool {
	var gTools []*genai.Tool
	for _, t := range tools {
		props := make(map[string]*genai.Schema)
		for _, param := range t.Parameters {
			props[param.Name] = &genai.Schema{
				Type:        toGenaiType(param.Type),
				Description: param.Description,
			}
		}
		gTools = append(gTools, &genai.Tool{
			FunctionDeclarations: []*genai.FunctionDeclaration{
				{
					Name:        t.Name,
					Description: t.Description,
					Parameters: &genai.Schema{
						Type:       genai.TypeObject,
						Properties: props,
						Required:   t.Required,
					},
				},
			},
		})
	}
	return gTools
}

func toGenaiType(t string) genai.Type {
	switch t {
	case "string":
		return genai.TypeString
	case "integer":
		return genai.TypeInteger
	case "number":
		return genai.TypeNumber
	case "boolean":
		return genai.TypeBoolean
	default:
		return genai.TypeString
	}
}
