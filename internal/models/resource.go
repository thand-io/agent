package models

type ProviderResource struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description"`

	// Store additional metadata if needed
	Metadata map[string]any `json:"metadata,omitempty"`

	// Store the underlying provider-specific resource object if needed
	Resource any `json:"-"`
}
