package models

import (
	"strings"

	"github.com/thand-io/agent/internal/common"
	sdkConstants "github.com/thand-io/agent/sdk/constants"
)

// Identity represents either a user or a group in the system.
// It serves as a unified abstraction for access control subjects,
// allowing policies to reference both users and groups consistently.
type Identity struct {
	// ID is the unique identifier for this identity. This will most likely be an email for users
	// or a group name for groups. This will be used to tie identities across providers.
	ID string `json:"id"`
	// Label is a human-readable name or description for this identity.
	Label string `json:"label"`

	// Tenant represents the tenant or organization this identity belongs to.
	// This is useful for multi-tenant systems where identities are scoped to specific tenants.
	Tenant string `json:"tenant,omitempty"`

	// User contains the user details if this identity represents a user.
	// Will be nil if this identity represents a group.
	User *User `json:"user,omitempty"`
	// Group contains the group details if this identity represents a group.
	// Will be nil if this identity represents a user.
	Group *Group `json:"group,omitempty"`

	// The providers this identity is associated with
	// Format is map[provider_name]provider_type
	Providers map[string]string `json:"providers,omitempty"`
}

func (i *Identity) GetId() string {
	return i.ID
}

func (i *Identity) String() string {
	if i.IsUser() {
		return i.User.String()
	} else if i.IsGroup() {
		return i.Group.String()
	}
	return ""
}

func (i *Identity) GetEmail() string {
	if i.User != nil {
		return i.User.Email
	} else if i.Group != nil {
		return i.Group.Email
	}
	return ""
}

func (i *Identity) Equals(other *Identity) bool {

	if other == nil {
		return false
	}

	if i.IsUser() && other.IsUser() {
		return i.User.Equals(other.User)
	} else if i.IsGroup() && other.IsGroup() {
		return i.Group.Equals(other.Group)
	}

	return false

}

func (i *Identity) GetMappableIdentifier() string {
	return strings.ToLower(strings.TrimSpace(i.mapableIdentifier()))
}

func (i *Identity) mapableIdentifier() string {

	if i.User != nil {
		return i.User.GetMappableIdentifier()
	} else if i.Group != nil {
		return i.Group.GetMappableIdentifier()
	}

	return i.ID

}

func (i *Identity) EncodeBytes() []byte {
	return NewEncodingWrapper(
		sdkConstants.ENCODED_IDENTITY,
		i,
	).encodeBytes()
}

func (i *Identity) EncodeBase64() string {
	return NewEncodingWrapper(
		sdkConstants.ENCODED_IDENTITY,
		i,
	).EncodeBase64()
}

func NewIdentityFromBytes(input []byte) (*Identity, error) {
	wrapper := NewDecodingWrapper(sdkConstants.ENCODED_IDENTITY)
	decodedWrapper, err := wrapper.DecodeBytes(input)
	if err != nil {
		return nil, err
	}
	var identity Identity
	if err := common.ConvertInterfaceToInterface(decodedWrapper.Data, &identity); err != nil {
		return nil, err
	}
	return &identity, nil
}

func NewIdentityFromBase64(input string) (*Identity, error) {
	wrapper := NewDecodingWrapper(sdkConstants.ENCODED_IDENTITY)
	decodedWrapper, err := wrapper.Decode(input)
	if err != nil {
		return nil, err
	}
	var identity Identity
	if err := common.ConvertInterfaceToInterface(decodedWrapper.Data, &identity); err != nil {
		return nil, err
	}
	return &identity, nil
}

func (i *Identity) GetLabel() string {
	return i.Label
}

func (i *Identity) GetUser() *User {
	return i.User
}

func (i *Identity) GetGroup() *Group {
	return i.Group
}

func (i *Identity) IsUser() bool {
	return i.User != nil
}

func (i *Identity) IsGroup() bool {
	return i.Group != nil
}

func (i *Identity) GetProviders() map[string]string {
	return i.Providers
}

func (i *Identity) AddProvider(provider Provider) {

	if provider == nil {
		return
	}

	// Check if provider already exists
	if i.Providers == nil {
		i.Providers = make(map[string]string)
	}
	if _, exists := i.Providers[provider.GetIdentifier()]; !exists {
		i.Providers[provider.GetIdentifier()] = provider.GetProvider()
	}
}

type IdentitiesResponse struct {
	Identities []SearchResult[Identity] `json:"identities"`
	Providers  int                      `json:"providers"`
}
