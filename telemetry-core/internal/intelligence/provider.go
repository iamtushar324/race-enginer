package intelligence

import (
	"context"
	"fmt"
	"strings"
)

// LLMProvider is the interface for all LLM backends (Gemini, Anthropic, OpenAI).
type LLMProvider interface {
	Available() bool
	Complete(ctx context.Context, system, prompt string, cfg ProviderConfig) (string, error)
	CompleteWithTools(ctx context.Context, system, prompt string, cfg ProviderConfig, tools []ToolDef) (*GenerateResult, error)
	Close()
}

// ProviderConfig holds provider-agnostic generation parameters.
type ProviderConfig struct {
	MaxTokens   int32
	Temperature float32
	JSONMode    bool
}

// ToolDef describes a function tool in a provider-agnostic way.
type ToolDef struct {
	Name        string
	Description string
	Parameters  []ToolParam
	Required    []string
}

// ToolParam describes a single parameter of a tool.
type ToolParam struct {
	Name        string
	Type        string // "string", "integer", "boolean", "number"
	Description string
}

// NewProvider creates an LLMProvider by name. Accepted names: "gemini", "anthropic"/"claude", "openai".
// apiKey is the provider-specific key. model is optional (empty = provider default).
func NewProvider(name, apiKey, model string) (LLMProvider, error) {
	switch strings.ToLower(name) {
	case "gemini", "":
		return newGeminiProvider(apiKey, model)
	case "anthropic", "claude":
		return newAnthropicProvider(apiKey, model)
	case "openai":
		return newOpenAIProvider(apiKey, model)
	default:
		return nil, fmt.Errorf("unknown LLM provider: %q (supported: gemini, anthropic, openai)", name)
	}
}
