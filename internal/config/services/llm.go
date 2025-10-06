package services

import (
	"github.com/thand-io/agent/internal/config/services/llm"
	"github.com/thand-io/agent/internal/models"
)

func (e *localClient) configureLargeLanguageModel() models.LargeLanguageModelImpl {

	provider := "local"

	llmConfig := e.GetServicesConfig().GetLLMConfig()

	if e.config.LargeLanguageModel != nil && len(e.config.LargeLanguageModel.Provider) > 0 {
		provider = e.config.LargeLanguageModel.Provider
	}

	// Initialise LLM client
	switch provider {
	case "gemini":
		fallthrough
	default:
		return llm.NewGcpLargeLanguageModel(llmConfig)
	}

}
