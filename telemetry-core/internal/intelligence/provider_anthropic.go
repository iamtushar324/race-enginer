package intelligence

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	anthropicAPI            = "https://api.anthropic.com/v1/messages"
	anthropicDefaultModel   = "claude-haiku-4-5-20251001"
	anthropicAPIVersion     = "2023-06-01"
)

type anthropicProvider struct {
	apiKey string
	model  string
	client *http.Client
}

func newAnthropicProvider(apiKey, model string) (*anthropicProvider, error) {
	if apiKey == "" {
		log.Warn().Msg("ANTHROPIC_API_KEY not set — Anthropic provider disabled")
		return &anthropicProvider{}, nil
	}
	if model == "" {
		model = anthropicDefaultModel
	}
	log.Info().Str("model", model).Msg("Anthropic provider initialised")
	return &anthropicProvider{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{Timeout: 60 * time.Second},
	}, nil
}

func (p *anthropicProvider) Available() bool { return p.apiKey != "" }
func (p *anthropicProvider) Close()          {}

// --- request/response types ---

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int32              `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMsg     `json:"messages"`
	Tools     []anthropicTool    `json:"tools,omitempty"`
}

type anthropicMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicTool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"input_schema"`
}

type anthropicResponse struct {
	Content []anthropicContent `json:"content"`
	Error   *anthropicError    `json:"error,omitempty"`
}

type anthropicContent struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

type anthropicError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// --- Complete ---

func (p *anthropicProvider) Complete(ctx context.Context, system, prompt string, cfg ProviderConfig) (string, error) {
	if !p.Available() {
		return "", fmt.Errorf("anthropic provider not available")
	}

	maxTokens := cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 1024
	}

	// For JSON mode, instruct the model to output JSON.
	if cfg.JSONMode {
		system += "\n\nYou MUST respond with valid JSON only. No markdown fencing, no explanation — just the JSON object."
	}

	reqBody := anthropicRequest{
		Model:     p.model,
		MaxTokens: maxTokens,
		System:    system,
		Messages:  []anthropicMsg{{Role: "user", Content: prompt}},
	}

	body, err := p.doRequest(ctx, reqBody)
	if err != nil {
		return "", err
	}

	var resp anthropicResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("anthropic: failed to decode response: %w", err)
	}
	if resp.Error != nil {
		return "", fmt.Errorf("anthropic API error: %s: %s", resp.Error.Type, resp.Error.Message)
	}

	// Extract text from content blocks.
	var texts []string
	for _, block := range resp.Content {
		if block.Type == "text" && block.Text != "" {
			texts = append(texts, block.Text)
		}
	}

	return strings.Join(texts, ""), nil
}

// --- CompleteWithTools ---

func (p *anthropicProvider) CompleteWithTools(ctx context.Context, system, prompt string, cfg ProviderConfig, tools []ToolDef) (*GenerateResult, error) {
	if !p.Available() {
		return &GenerateResult{}, fmt.Errorf("anthropic provider not available")
	}

	maxTokens := cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 1024
	}

	var aTools []anthropicTool
	for _, t := range tools {
		props := make(map[string]interface{})
		for _, param := range t.Parameters {
			props[param.Name] = map[string]string{
				"type":        param.Type,
				"description": param.Description,
			}
		}
		aTools = append(aTools, anthropicTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": props,
				"required":   t.Required,
			},
		})
	}

	reqBody := anthropicRequest{
		Model:     p.model,
		MaxTokens: maxTokens,
		System:    system,
		Messages:  []anthropicMsg{{Role: "user", Content: prompt}},
		Tools:     aTools,
	}

	body, err := p.doRequest(ctx, reqBody)
	if err != nil {
		return nil, err
	}

	var resp anthropicResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("anthropic: failed to decode response: %w", err)
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("anthropic API error: %s: %s", resp.Error.Type, resp.Error.Message)
	}

	result := &GenerateResult{}
	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			result.Text += block.Text
		case "tool_use":
			var args map[string]any
			if len(block.Input) > 0 {
				_ = json.Unmarshal(block.Input, &args)
			}
			result.ToolCalls = append(result.ToolCalls, ToolCall{
				Name: block.Name,
				Args: args,
			})
		}
	}

	return result, nil
}

// --- HTTP helper ---

func (p *anthropicProvider) doRequest(ctx context.Context, reqBody anthropicRequest) ([]byte, error) {
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("anthropic: failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicAPI, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("anthropic: failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", anthropicAPIVersion)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("anthropic: failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic: HTTP %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}
