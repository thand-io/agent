package models

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search"
	"github.com/sirupsen/logrus"
)

type ProviderIdentities interface {
	GetIdentity(ctx context.Context, identity string) (*Identity, error)
	ListIdentities(ctx context.Context, searchRequest *SearchRequest) ([]SearchResult[Identity], error)

	// Overrides all existing identities with the provided list
	SetIdentities(identities []Identity)
	// Appends new identities to the existing list
	AddIdentities(identities ...Identity)

	// Some APIs support identities, users, groups service accoutns etc.
	SynchronizeIdentities(ctx context.Context, req *SynchronizeIdentitiesRequest) (*SynchronizeIdentitiesResponse, error)
	// Some require more granular user synchronization
	SynchronizeUsers(ctx context.Context, req *SynchronizeUsersRequest) (*SynchronizeUsersResponse, error)
	// Others require group synchronization
	SynchronizeGroups(ctx context.Context, req *SynchronizeGroupsRequest) (*SynchronizeGroupsResponse, error)
}

func (p *BaseProvider) SynchronizeIdentities(ctx context.Context, req *SynchronizeIdentitiesRequest) (*SynchronizeIdentitiesResponse, error) {
	return nil, ErrNotImplemented
}

func (p *BaseProvider) SynchronizeUsers(ctx context.Context, req *SynchronizeUsersRequest) (*SynchronizeUsersResponse, error) {
	return nil, ErrNotImplemented
}

func (p *BaseProvider) SynchronizeGroups(ctx context.Context, req *SynchronizeGroupsRequest) (*SynchronizeGroupsResponse, error) {
	return nil, ErrNotImplemented
}

// GetIdentity retrieves a specific identity (user or group) from GCP
func (p *BaseProvider) GetIdentity(ctx context.Context, identity string) (*Identity, error) {

	if p.identity == nil || !p.HasAnyCapability(
		IdentityCapabilities...,
	) {
		logrus.Warningln("provider does not support identities capability for provider: ", p.GetIdentifier())
		return nil, fmt.Errorf("provider does not support identities capability")
	}

	// Fast map lookup under read lock
	p.identity.mu.RLock()
	defer p.identity.mu.RUnlock()

	if p.identity.identitiesMap != nil {
		if id, exists := p.identity.identitiesMap[strings.ToLower(identity)]; exists {
			return id, nil
		}
	}

	return nil, fmt.Errorf("identity not found: %s", identity)
}

// ListIdentities lists all identities (users and groups) from the provider
func (p *BaseProvider) ListIdentities(ctx context.Context, searchRequest *SearchRequest) ([]SearchResult[Identity], error) {

	if p.identity == nil || !p.HasAnyCapability(
		IdentityCapabilities...,
	) {
		logrus.Warningln("provider does not support identities capability for provider: ", p.GetIdentifier())
		return nil, fmt.Errorf("provider does not support identities capability")
	}

	p.identity.mu.RLock()
	identities := p.identity.identities
	p.identity.mu.RUnlock()

	// If no filters, return all identities
	if searchRequest == nil || searchRequest.IsEmpty() {
		return ReturnSearchResults(identities), nil
	}

	// Check if search index is ready
	p.identity.mu.RLock()
	identitiesIndex := p.identity.identitiesIndex
	p.identity.mu.RUnlock()

	if identitiesIndex != nil {
		// Use Bleve search for better search capabilities
		return BleveListSearch(ctx, identitiesIndex, func(a *search.DocumentMatch, b Identity) bool {
			return strings.EqualFold(a.ID, b.ID)
		}, identities, searchRequest)
	}

	// Apply filters
	var filtered []Identity
	filterText := strings.ToLower(strings.Join(searchRequest.Terms, " "))
	limit := searchRequest.GetLimit()

	for _, identity := range identities {
		// Check if any filter matches the identity
		if strings.Contains(strings.ToLower(identity.Label), filterText) ||
			strings.Contains(strings.ToLower(identity.ID), filterText) ||
			(identity.User != nil && strings.Contains(strings.ToLower(identity.User.Email), filterText)) ||
			(identity.User != nil && strings.Contains(strings.ToLower(identity.User.Name), filterText)) ||
			(identity.Group != nil && strings.Contains(strings.ToLower(identity.Group.Name), filterText)) ||
			(identity.Group != nil && strings.Contains(strings.ToLower(identity.Group.Email), filterText)) {
			filtered = append(filtered, identity)
			if limit > 0 && len(filtered) >= limit {
				break
			}
		}
	}

	return ReturnSearchResults(filtered), nil
}

func (p *BaseProvider) buildIdentitiyIndices() error {
	// Placeholder for building indices
	startTime := time.Now()
	defer func() {
		elapsed := time.Since(startTime)
		logrus.Debugf("Built identity search indices in %s", elapsed)
	}()

	identityMapping := bleve.NewIndexMapping()
	identityIndex, err := bleve.NewMemOnly(identityMapping)
	if err != nil {
		logrus.WithError(err).Error("Failed to create identity search index for provider: ", p.GetIdentifier())
		return fmt.Errorf("failed to create identity search index: %v", err)
	}

	// Index identities
	p.identity.mu.RLock()
	identities := p.identity.identities
	p.identity.mu.RUnlock()

	for _, identity := range identities {
		if err := identityIndex.Index(identity.ID, identity); err != nil {
			logrus.WithError(err).Errorf("Failed to index identity %s for provider: %s", identity.ID, p.GetIdentifier())
			return fmt.Errorf("failed to index identity %s: %v", identity.ID, err)
		}
	}

	p.identity.mu.Lock()
	p.identity.identitiesIndex = identityIndex
	identityCount := len(p.identity.identities)
	p.identity.mu.Unlock()

	logrus.WithFields(logrus.Fields{
		"identities": identityCount,
	}).Debug("Identity search indices ready for provider: ", p.GetIdentifier())

	return nil
}

func CreateKeysFromIdentity(i Identity) []string {
	var keys []string
	keys = append(keys, i.ID)
	keys = append(keys, i.Label)
	if i.User != nil && len(i.User.Email) != 0 {
		keys = append(keys, i.User.Email)
	}
	if i.Group != nil {
		if len(i.Group.Name) != 0 {
			keys = append(keys, i.Group.Name)
		}
		if len(i.Group.Email) != 0 {
			keys = append(keys, i.Group.Email)
		}
	}
	return keys
}

func (p *BaseProvider) SetIdentities(identities []Identity) {
	p.SetIdentitiesWithKey(identities, CreateKeysFromIdentity)
}

func (p *BaseProvider) SetIdentitiesWithKey(
	identities []Identity,
	keyFunc func(i Identity) []string,
) {

	if p.identity == nil {
		logrus.Warningln("provider has no identity support")
		return
	}

	p.identity.mu.Lock()
	defer p.identity.mu.Unlock()

	if p.identity.identities == nil {
		p.identity.identities = make([]Identity, 0)
	}

	p.identity.identities = identities

	// Build the identities map
	p.identity.identitiesMap = make(map[string]*Identity)
	for i := range identities {

		identity := identities[i]
		keys := keyFunc(identity)

		for _, key := range keys {
			p.identity.identitiesMap[strings.ToLower(key)] = &identity
		}
	}

	logrus.WithFields(logrus.Fields{
		"total_identities": len(p.identity.identities),
	}).Debug("Set provider identities")

	// Trigger reindex
	go func() {
		err := p.buildIdentitiyIndices()
		if err != nil {
			logrus.WithError(err).Error("Failed to build identity search indices for provider: ", p.GetIdentifier())
			return
		}
	}()
}

func (p *BaseProvider) AddIdentities(identities ...Identity) {
	// Take existing identities and append new ones
	if p.identity == nil {
		logrus.Warningln("provider has no identity support for provider: ", p.GetIdentifier())
		return
	}

	// Hold a single write lock for the entire read-modify-write to prevent
	// concurrent Add* calls from overwriting each other's changes (TOCTOU race).
	p.identity.mu.Lock()

	if p.identity.identities == nil {
		p.identity.identities = make([]Identity, 0)
	}
	if p.identity.identitiesMap == nil {
		p.identity.identitiesMap = make(map[string]*Identity)
	}

	filtered := FilterDuplicates(
		identities,
		p.identity.identitiesMap,
		CreateKeysFromIdentity,
	)

	existingCount := len(p.identity.identities)

	if len(filtered) > 0 {
		p.identity.identities = append(p.identity.identities, filtered...)

		// Update the map in place for the newly added identities.
		for i := range filtered {
			identity := &p.identity.identities[existingCount+i]
			for _, key := range CreateKeysFromIdentity(filtered[i]) {
				p.identity.identitiesMap[strings.ToLower(key)] = identity
			}
		}
	}

	totalCount := len(p.identity.identities)
	p.identity.mu.Unlock()

	logrus.WithFields(logrus.Fields{
		"existing": existingCount,
		"new":      len(identities),
		"added":    len(filtered),
		"total":    totalCount,
	}).Debug("Adding identities to provider: ", p.GetIdentifier())

	if len(filtered) > 0 {
		// Trigger reindex asynchronously (buildIdentitiyIndices acquires its own lock).
		go func() {
			if err := p.buildIdentitiyIndices(); err != nil {
				logrus.WithError(err).Error("Failed to build identity search indices for provider: ", p.GetIdentifier())
			}
		}()
	}
}
