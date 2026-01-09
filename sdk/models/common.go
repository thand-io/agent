// Package models provides public SDK types for the thand agent.
// These types are re-exported from the internal models package to provide
// a stable public API for external consumers.
package models

import internal "github.com/thand-io/agent/internal/models"

// BasicConfig represents the fundamental configuration settings for the agent.
// It contains core parameters needed for agent initialization and operation.
type BasicConfig = internal.BasicConfig

// EncodingWrapper provides a wrapper for encoding and decoding data.
// It abstracts the underlying encoding implementation for serialization operations.
type EncodingWrapper = internal.EncodingWrapper
