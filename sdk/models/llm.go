package models

import (
	internal "github.com/thand-io/agent/internal/models"
)

// LargeLanguageModelService provides an interface to Large Language Model (LLM) services
// for AI-powered features in Thand workflows. This service enables natural language processing
// capabilities including:
//   - Natural language access request parsing and intent recognition
//   - Automated policy and role generation from descriptions
//   - Intelligent approval routing and decision support
//   - Conversational interfaces for workflow interactions
//
// Supported providers include:
//   - Google Gemini (Gemini Pro, Gemini Flash)
//
// The service handles authentication, request formatting, and response parsing across
// different LLM providers with a unified interface. Configure the LLM service in your
// config.yaml under services.llm with provider-specific credentials and model settings.
type LargeLanguageModelService = internal.LargeLanguageModelImpl
