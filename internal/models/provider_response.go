package models

import "github.com/hashicorp/go-version"

// ProvidersResponse represents the response for a providers query
type ProvidersResponse struct {
	Version   *version.Version                 `json:"version"`
	Providers []SearchResult[ProviderResponse] `json:"providers"`
	Meta      ResponseMeta                     `json:"meta"`
}

type ProviderResponse struct {
	Version      *version.Version      `json:"version,omitempty"`
	ID           string                `json:"id"`
	Name         string                `json:"name"`
	Description  string                `json:"description"`
	Provider     string                `json:"provider"` // e.g. aws, gcp, azure
	Capabilities *ProviderCapabilities `json:"capabilities"`
	Enabled      bool                  `json:"enabled"`
}
