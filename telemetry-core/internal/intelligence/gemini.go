// Package intelligence provides LLM-powered race analysis capabilities.
// It wraps the Google Gemini API and coordinates the advisor, translator,
// and analyst goroutines that form the "brain" of the race engineer.
package intelligence

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/generative-ai-go/genai"
	"github.com/rs/zerolog/log"
	"google.golang.org/api/option"
)

// GeminiClient wraps the Google Generative AI SDK. All methods are nil-safe:
// if no API key was provided, calls return empty strings without error.
type GeminiClient struct {
	client *genai.Client
}

// NewGeminiClient creates a Gemini client. If apiKey is empty, returns a
// nil-safe stub that returns fallback responses.
func NewGeminiClient(ctx context.Context, apiKey string) *GeminiClient {
	if apiKey == "" {
		log.Warn().Msg("GEMINI_API_KEY not set — LLM features disabled")
		return &GeminiClient{}
	}

	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		log.Error().Err(err).Msg("Failed to create Gemini client — LLM features disabled")
		return &GeminiClient{}
	}

	log.Info().Msg("Gemini client initialised")
	return &GeminiClient{client: client}
}

// Available returns true if the Gemini client is configured and ready.
func (g *GeminiClient) Available() bool {
	return g.client != nil
}

// Close releases the underlying gRPC connection.
func (g *GeminiClient) Close() {
	if g.client != nil {
		g.client.Close()
	}
}

// GenerateConfig holds parameters for a generation call.
type GenerateConfig struct {
	Model            string       // e.g. "gemini-2.5-flash"
	System           string       // system instruction
	Temperature      float32      // 0.0–2.0
	MaxTokens        int32        // max output tokens
	ResponseMIMEType string       // "application/json" for structured output
	ResponseSchema   *genai.Schema // JSON schema for response shape
}

// DefaultConfig returns a config suitable for most race engineer prompts.
func DefaultConfig() GenerateConfig {
	return GenerateConfig{
		Model:       "gemini-2.5-flash",
		Temperature: 0.3,
		MaxTokens:   500,
	}
}

// Generate sends a prompt to Gemini and returns the text response.
// Returns empty string if the client is not available.
func (g *GeminiClient) Generate(ctx context.Context, prompt string, cfg GenerateConfig) string {
	if g.client == nil {
		return ""
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	model := g.client.GenerativeModel(cfg.Model)
	model.SetTemperature(cfg.Temperature)
	model.SetMaxOutputTokens(cfg.MaxTokens)
	if cfg.System != "" {
		model.SystemInstruction = genai.NewUserContent(genai.Text(cfg.System))
	}

	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		log.Error().Err(err).Msg("Gemini generate failed")
		return ""
	}

	return extractText(resp)
}

// GenerateJSON sends a prompt to any LLMProvider with JSONMode enabled and
// unmarshals the result into the provided type T.
func GenerateJSON[T any](p LLMProvider, ctx context.Context, prompt, system string, cfg ProviderConfig) (T, error) {
	var zero T
	if !p.Available() {
		return zero, fmt.Errorf("LLM provider not available")
	}

	cfg.JSONMode = true
	text, err := p.Complete(ctx, system, prompt, cfg)
	if err != nil {
		return zero, fmt.Errorf("LLM generate failed: %w", err)
	}

	if text == "" {
		return zero, fmt.Errorf("LLM returned empty response")
	}

	// Strip markdown fencing that models sometimes wrap around JSON responses.
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	var result T
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return zero, fmt.Errorf("failed to unmarshal JSON response: %w (raw: %s)", err, text)
	}

	return result, nil
}

// GenerateResult holds the response from a tool-calling generation.
type GenerateResult struct {
	Text      string
	ToolCalls []ToolCall
}

// ToolCall represents a function call requested by the model.
type ToolCall struct {
	Name string
	Args map[string]any
}

// GenerateWithTools sends a prompt with function declarations and returns
// both text and any tool calls the model wants to make.
func (g *GeminiClient) GenerateWithTools(ctx context.Context, prompt string, cfg GenerateConfig, tools []*genai.Tool) GenerateResult {
	if g.client == nil {
		return GenerateResult{}
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	model := g.client.GenerativeModel(cfg.Model)
	model.SetTemperature(cfg.Temperature)
	model.SetMaxOutputTokens(cfg.MaxTokens)
	if cfg.System != "" {
		model.SystemInstruction = genai.NewUserContent(genai.Text(cfg.System))
	}
	model.Tools = tools

	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		log.Error().Err(err).Msg("Gemini generate with tools failed")
		return GenerateResult{}
	}

	result := GenerateResult{Text: extractText(resp)}

	// Extract tool calls from response candidates.
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

	return result
}

// extractText pulls the text content from a Gemini response.
func extractText(resp *genai.GenerateContentResponse) string {
	if resp == nil {
		return ""
	}
	for _, cand := range resp.Candidates {
		if cand.Content == nil {
			continue
		}
		for _, part := range cand.Content.Parts {
			if t, ok := part.(genai.Text); ok {
				return string(t)
			}
		}
	}
	return ""
}
