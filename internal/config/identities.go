package config

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
)

const (
	IdentityTypeUser  IdentityType = "user"
	IdentityTypeGroup IdentityType = "group"
	IdentityTypeAll   IdentityType = "all"
)

type IdentityType string

// GetIdentity looks up an identity by its identifier.
// The identity string can optionally include a provider prefix (e.g., "aws-prod:username").
// If a prefix is provided, it queries only that specific provider.
// Otherwise, it queries all identity providers and returns the first match.
func (c *Config) GetIdentity(identity string) (*models.Identity, error) {
	ctx := context.Background()

	// Check if the identity has a provider prefix (e.g., "aws-prod:username")
	var providerID string
	var identityKey string

	if colonIdx := strings.Index(identity, ":"); colonIdx != -1 {
		// Has provider prefix
		providerID = identity[:colonIdx]
		identityKey = identity[colonIdx+1:]
	} else {
		// No prefix, use the full identity
		identityKey = identity
	}

	// If we have a specific provider, query only that one
	if len(providerID) != 0 {
		provider, err := c.GetProviderByName(providerID)
		if err != nil {
			return nil, fmt.Errorf("provider '%s' not found: %w", providerID, err)
		}

		result, err := provider.GetClient().GetIdentity(ctx, identityKey)
		if err != nil {
			return nil, fmt.Errorf("failed to get identity '%s' from provider '%s': %w", identityKey, providerID, err)
		}

		return result, nil
	}

	// No provider prefix - query all identity providers
	providerMap := c.GetProvidersByCapability(models.ProviderCapabilityIdentities)

	if len(providerMap) == 0 {
		// No identity providers, create a basic identity from the string
		// Extract username from email if possible
		username := ""
		if atIdx := strings.Index(identity, "@"); atIdx > 0 {
			username = identity[:atIdx]
		}
		return &models.Identity{
			ID:    identity,
			Label: identity,
			User: &models.User{
				Email:    identity,
				Username: username,
				Source:   "", // Empty source means use traditional IAM, not Identity Center
			},
		}, nil
	}

	// Query all providers in parallel and return the first match
	var wg sync.WaitGroup
	resultChan := make(chan *models.Identity, len(providerMap))
	doneChan := make(chan struct{})

	for _, provider := range providerMap {
		wg.Add(1)
		go func(p models.Provider) {
			defer wg.Done()

			result, err := p.GetClient().GetIdentity(ctx, identityKey)
			if err != nil {
				logrus.WithError(err).WithFields(logrus.Fields{
					"provider": p.Name,
					"identity": identityKey,
				}).Debug("Failed to get identity from provider")
				return
			}

			if result != nil {
				select {
				case resultChan <- result:
				case <-doneChan:
					// Another goroutine already found a result
				}
			}
		}(provider)
	}

	// Wait for all goroutines to complete and then close the result channel
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Try to get a result from the channel
	// If channel is closed and empty, result will be nil and ok will be false
	for result := range resultChan {
		if result != nil {
			close(doneChan)
			return result, nil
		}
	}

	// All goroutines finished without finding a result
	// Return a basic identity
	// Extract username from email if possible
	username := ""
	if atIdx := strings.Index(identityKey, "@"); atIdx > 0 {
		username = identityKey[:atIdx]
	}
	return &models.Identity{
		ID:    identity,
		Label: identity,
		User: &models.User{
			Email:    identity,
			Username: username,
			Source:   "", // Empty source means use traditional IAM, not Identity Center
		},
	}, nil
}

// GetIdentitiesWithFilter retrieves identities from all identity providers that support identity listing.
// It applies an optional filter to narrow down the results.
// If no identity providers are found, it returns the current user as the only identity.
// The identityType parameter can be used to filter results by type (user, group, or all).
func (c *Config) GetIdentitiesWithFilter(user *models.User, identityType IdentityType, filter ...string) ([]models.Identity, error) {

	// the user can be nil here if there is no authenticated user context

	// Filter out empty strings from the filter
	filter = common.FilterEmpty(filter...)

	// Find providers with identity capabilities
	providerMap := c.GetProvidersByCapabilityWithUser(user, models.ProviderCapabilityIdentities)

	var identities []models.Identity

	// If no identity providers found, return just the current user
	if len(providerMap) == 0 {
		// Apply filter to current user if specified
		if len(filter) > 0 {
			userFields := []string{strings.ToLower(user.Name), strings.ToLower(user.Email)}
			matchesFilter := slices.ContainsFunc(userFields, func(field string) bool {
				return strings.Contains(field, strings.ToLower(filter[0]))
			})
			if !matchesFilter {
				// User doesn't match filter, return empty list
				identities = []models.Identity{}
			} else if user != nil {
				// The default user matches the filter
				identities = []models.Identity{
					{
						ID:    user.Email,
						Label: user.Name,
						User:  user,
					},
				}
			} // No user context, return empty list
		} else if user != nil {
			// No filter, return the default user
			identities = []models.Identity{
				{
					ID:    user.Email,
					Label: user.Name,
					User:  user,
				},
			}
		}

	} else {

		// Query all identity providers in parallel
		ctx := context.Background()
		var wg sync.WaitGroup
		var mu sync.Mutex

		// Map to aggregate identities by name (to avoid duplicates across providers)
		identityMap := make(map[string]models.Identity)

		// Channel to collect errors
		errorChan := make(chan error, len(providerMap))

		for _, provider := range providerMap {
			wg.Add(1)
			go func(p models.Provider) {
				defer wg.Done()

				// Query identities from this provider with filter
				var identities []models.Identity
				var err error

				identities, err = p.GetClient().ListIdentities(ctx, filter...)

				if err != nil {
					logrus.WithError(err).
						WithField("provider", p.Name).
						Error("Failed to get identities from provider")
					errorChan <- err
					return
				}

				// Add identities to the map (thread-safe)
				mu.Lock()
				for _, identity := range identities {

					if identityType == IdentityTypeUser && identity.User == nil {
						continue
					}
					if identityType == IdentityTypeGroup && identity.Group == nil {
						continue
					}

					// Use identity ID as key to avoid duplicates
					// If the same identity exists from multiple providers, keep the first one
					if foundIdentity, exists := identityMap[identity.GetId()]; !exists {
						identity.AddProvider(&p)
						identityMap[identity.GetId()] = identity
					} else {
						// Add provider to identity
						foundIdentity.AddProvider(&p)

						// Also check if we need to update User or Group info
						// with any missing details
						if identity.User != nil && foundIdentity.User == nil {
							foundIdentity.User = identity.User
						}
						if identity.Group != nil && foundIdentity.Group == nil {
							foundIdentity.Group = identity.Group
						}
						identityMap[identity.GetId()] = foundIdentity
					}
				}
				mu.Unlock()
			}(provider)
		}

		// Wait for all goroutines to complete
		wg.Wait()
		close(errorChan)

		// Collect all errors
		var foundErrors []error
		for err := range errorChan {
			if err != nil {
				foundErrors = append(foundErrors, err)
			}
		}

		// If there were errors, just log them
		if len(foundErrors) > 0 {
			logrus.WithError(errors.Join(foundErrors...)).
				Error("Errors occurred while retrieving identities")
		}

		// Convert map to slice
		identities = make([]models.Identity, 0, len(identityMap))
		for _, identity := range identityMap {
			identities = append(identities, identity)
		}

		// Always include the current user in the identities list if not already present
		// This ensures users can always request access for themselves
		if user != nil && (identityType == IdentityTypeUser || identityType == IdentityTypeAll) {
			userAlreadyIncluded := false
			for _, identity := range identities {
				if identity.ID == user.Email {
					userAlreadyIncluded = true
					break
				}
			}
			if !userAlreadyIncluded {
				// Add current user at the beginning of the list
				currentUserIdentity := models.Identity{
					ID:    user.Email,
					Label: user.Name,
					User:  user,
				}
				identities = append([]models.Identity{currentUserIdentity}, identities...)
			}
		}
	}

	return identities, nil

}
