package llm

import (
	"context"
	"errors"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
	"google.golang.org/genai"
)

/*
Best for
Image understanding
Video understanding
Audio understanding
*/
var GoogleGenerativeModelName = "gemini-2.5-flash"

type googleGenerativeClient struct {
	config *models.LargeLanguageModelConfig
	model  string
	client *genai.Client
}

func NewGcpLargeLanguageModel(config *models.LargeLanguageModelConfig) models.LargeLanguageModelImpl {

	modelName := GoogleGenerativeModelName

	if config != nil && len(config.Model) > 0 {
		modelName = config.Model
	}

	return &googleGenerativeClient{
		config: config,
		model:  modelName,
	}
}

func (a *googleGenerativeClient) Initialize() error {

	geminiApiKey := a.config.APIKey

	if len(geminiApiKey) == 0 {
		return errors.New("missing Gemini API key")
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  geminiApiKey,
		Backend: genai.BackendGeminiAPI,
	})

	if err != nil {
		logrus.WithError(err).Errorln("failed to create Google Generative AI client")
		return err
	}

	a.client = client

	return nil
}

func (c *googleGenerativeClient) GetGenerativeAI() *genai.Client {
	return c.client
}

func (c *googleGenerativeClient) GetModelName() string {
	return c.model
}

func (c *googleGenerativeClient) GenerativeModel() *genai.Models {

	if c == nil {
		return nil
	}

	return c.GetGenerativeAI().Models
}

func (c *googleGenerativeClient) GenerateContent(
	ctx context.Context,
	model string,
	contents []*genai.Content,
	config *genai.GenerateContentConfig,
) (*genai.GenerateContentResponse, error) {

	logrus.WithFields(logrus.Fields{
		"model":    model,
		"contents": len(contents),
	}).Debug("Generating content with Google Generative AI")

	return c.GenerativeModel().GenerateContent(ctx, model, contents, config)
}
