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
	openaiAPI          = "https://api.openai.com/v1/chat/completions"
	openaiDefaultModel = "gpt-4o-mini"
)

type openaiProvider struct {
	apiKey string
	model  string
	client *http.Client
}

func newOpenAIProvider(apiKey, model string) (*openaiProvider, error) {
	if apiKey == "" {
		log.Warn().Msg("OPENAI_API_KEY not set — OpenAI provider disabled")
		return &openaiProvider{}, nil
	}
	if model == "" {
		model = openaiDefaultModel
	}
	log.Info().Str("model", model).Msg("OpenAI provider initialised")
	return &openaiProvider{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{Timeout: 60 * time.Second},
	}, nil
}

func (p *openaiProvider) Available() bool { return p.apiKey != "" }
func (p *openaiProvider) Close()          {}

// --- request/response types ---

type openaiRequest struct {
	Model          string            `json:"model"`
	Messages       []openaiMsg       `json:"messages"`
	MaxTokens      int32             `json:"max_tokens,omitempty"`
	Temperature    float32           `json:"temperature"`
	Tools          []openaiTool      `json:"tools,omitempty"`
	ResponseFormat *openaiRespFormat `json:"response_format,omitempty"`
}

type openaiMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openaiTool struct {
	Type     string         `json:"type"`
	Function openaiFunction `json:"function"`
}

type openaiFunction struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Parameters  interface{} `json:"parameters"`
}

type openaiRespFormat struct {
	Type string `json:"type"`
}

type openaiResponse struct {
	Choices []openaiChoice `json:"choices"`
	Error   *openaiError   `json:"error,omitempty"`
}

type openaiChoice struct {
	Message openaiRespMsg `json:"message"`
}

type openaiRespMsg struct {
	Content   string           `json:"content"`
	ToolCalls []openaiToolCall `json:"tool_calls,omitempty"`
}

type openaiToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function openaiToolCallFunc `json:"function"`
}

type openaiToolCallFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string
}

type openaiError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

// --- Complete ---

func (p *openaiProvider) Complete(ctx context.Context, system, prompt string, cfg ProviderConfig) (string, error) {
	if !p.Available() {
		return "", fmt.Errorf("openai provider not available")
	}

	maxTokens := cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 1024
	}

	messages := []openaiMsg{}
	if system != "" {
		messages = append(messages, openaiMsg{Role: "system", Content: system})
	}
	messages = append(messages, openaiMsg{Role: "user", Content: prompt})

	reqBody := openaiRequest{
		Model:       p.model,
		Messages:    messages,
		MaxTokens:   maxTokens,
		Temperature: cfg.Temperature,
	}

	if cfg.JSONMode {
		reqBody.ResponseFormat = &openaiRespFormat{Type: "json_object"}
		// OpenAI requires "json" in the prompt for json_object mode.
		if system != "" && !strings.Contains(strings.ToLower(system), "json") {
			messages[0].Content += "\n\nYou MUST respond with valid JSON only."
			reqBody.Messages = messages
		}
	}

	body, err := p.doRequest(ctx, reqBody)
	if err != nil {
		return "", err
	}

	var resp openaiResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("openai: failed to decode response: %w", err)
	}
	if resp.Error != nil {
		return "", fmt.Errorf("openai API error: %s: %s", resp.Error.Type, resp.Error.Message)
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("openai: empty choices in response")
	}

	return resp.Choices[0].Message.Content, nil
}

// --- CompleteWithTools ---

func (p *openaiProvider) CompleteWithTools(ctx context.Context, system, prompt string, cfg ProviderConfig, tools []ToolDef) (*GenerateResult, error) {
	if !p.Available() {
		return &GenerateResult{}, fmt.Errorf("openai provider not available")
	}

	maxTokens := cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 1024
	}

	messages := []openaiMsg{}
	if system != "" {
		messages = append(messages, openaiMsg{Role: "system", Content: system})
	}
	messages = append(messages, openaiMsg{Role: "user", Content: prompt})

	var oTools []openaiTool
	for _, t := range tools {
		props := make(map[string]interface{})
		for _, param := range t.Parameters {
			props[param.Name] = map[string]string{
				"type":        param.Type,
				"description": param.Description,
			}
		}
		oTools = append(oTools, openaiTool{
			Type: "function",
			Function: openaiFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters: map[string]interface{}{
					"type":       "object",
					"properties": props,
					"required":   t.Required,
				},
			},
		})
	}

	reqBody := openaiRequest{
		Model:       p.model,
		Messages:    messages,
		MaxTokens:   maxTokens,
		Temperature: cfg.Temperature,
		Tools:       oTools,
	}

	body, err := p.doRequest(ctx, reqBody)
	if err != nil {
		return nil, err
	}

	var resp openaiResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("openai: failed to decode response: %w", err)
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("openai API error: %s: %s", resp.Error.Type, resp.Error.Message)
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("openai: empty choices in response")
	}

	msg := resp.Choices[0].Message
	result := &GenerateResult{Text: msg.Content}

	for _, tc := range msg.ToolCalls {
		if tc.Type == "function" {
			var args map[string]any
			if tc.Function.Arguments != "" {
				_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
			}
			result.ToolCalls = append(result.ToolCalls, ToolCall{
				Name: tc.Function.Name,
				Args: args,
			})
		}
	}

	return result, nil
}

// --- HTTP helper ---

func (p *openaiProvider) doRequest(ctx context.Context, reqBody openaiRequest) ([]byte, error) {
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("openai: failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openaiAPI, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("openai: failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("openai: failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai: HTTP %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}
