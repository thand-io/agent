package models

import (
	"context"

	"google.golang.org/genai"
)

type LargeLanguageModelConfig struct {
	Provider string `mapstructure:"provider" required:"true" validate:"omitempty,oneof=openai gemini anthropic"` // e.g. openai, azure, etc
	APIKey   string `mapstructure:"api_key" required:"true"`
	BaseURL  string `mapstructure:"base_url"` // e.g. https://api.openai.com/v1
	Model    string `mapstructure:"model"`    // e.g. gpt-4, gpt-3.5-turbo, etc
}

type LargeLanguageModelImpl interface {
	Initialize() error
	GenerateContent(
		ctx context.Context,
		model string,
		contents []*genai.Content,
		config *genai.GenerateContentConfig,
	) (*genai.GenerateContentResponse, error)
	GetModelName() string
}
