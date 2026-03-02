package services

import (
	"github.com/thand-io/agent/internal/config/services/llm"
	"github.com/thand-io/agent/internal/models"
)

func (e *localClient) configureLargeLanguageModel() models.LargeLanguageModelImpl {

	provider := "local"

	llmConfig := e.config.GetServicesConfig().GetLargeLanguageModelConfig()

	if llmConfig != nil && len(llmConfig.GetProvider()) > 0 {
		provider = llmConfig.GetProvider()
	}

	// Initialise LLM client
	switch provider {
	case "gemini":
		fallthrough
	default:
		return llm.NewGcpLargeLanguageModel(llmConfig)
	}

}
