package config

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
)

// MergeIdentities merges missing fields from source into target
func MergeIdentities(target, source *models.Identity) {
	// Merge User fields
	if source.User != nil {
		if target.User == nil {
			target.User = source.User
		} else {
			MergeStrings(&target.User.ID, source.User.ID)
			MergeStrings(&target.User.Username, source.User.Username)
			MergeStrings(&target.User.Email, source.User.Email)
			MergeStrings(&target.User.Name, source.User.Name)
			MergeStrings(&target.User.Source, source.User.Source)
			if target.User.Verified == nil && source.User.Verified != nil {
				target.User.Verified = source.User.Verified
			}
			// Append groups from source, avoiding duplicates
			if len(source.User.Groups) > 0 {
				existingGroups := make(map[string]bool)
				for _, group := range target.User.Groups {
					existingGroups[group] = true
				}
				for _, group := range source.User.Groups {
					if !existingGroups[group] {
						target.User.Groups = append(target.User.Groups, group)
					}
				}
			}
		}
	}

	// Merge Group fields
	if source.Group != nil {
		if target.Group == nil {
			target.Group = source.Group
		} else {
			MergeStrings(&target.Group.ID, source.Group.ID)
			MergeStrings(&target.Group.Parent, source.Group.Parent)
			MergeStrings(&target.Group.Name, source.Group.Name)
			MergeStrings(&target.Group.Email, source.Group.Email)
		}
	}

	// Merge Identity-level fields
	MergeStrings(&target.ID, source.ID)
	MergeStrings(&target.Label, source.Label)
	MergeStrings(&target.Tenant, source.Tenant)
}

// mergeStrings sets target to source if target is empty and source is not
func MergeStrings(target *string, source string) {
	if len(*target) == 0 && len(source) > 0 {
		*target = source
	}
}

const (
	IdentityTypeUser  IdentityType = "user"
	IdentityTypeGroup IdentityType = "group"
	IdentityTypeAll   IdentityType = "all"
)

type IdentityType string

func (c *Config) GetIdentitiesCount() int64 {
	ctx := context.Background()

	identitiesCount := int64(0)

	for _, provider := range c.GetProvidersByCapability(models.IdentityCapabilities...) {
		count, err := provider.ListIdentities(ctx, &models.SearchRequest{})

		if err != nil {
			logrus.WithError(err).
				WithField("provider", provider.GetName()).
				Error("Failed to get identities count from provider")
			continue
		}

		identitiesCount += int64(len(count))
	}

	return identitiesCount
}

// GetIdentity looks up an identity by its identifier.
// The identity string can optionally include a provider prefix (e.g., "aws-prod:username").
// If a prefix is provided, it queries only that specific provider.
// Otherwise, it queries all identity providers, merges results from multiple providers
// (filling in missing User/Group information), and returns the alphabetically first identity
// by mappable identifier.
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

		result, err := provider.GetIdentity(ctx, identityKey)
		if err != nil {
			return nil, fmt.Errorf("failed to get identity '%s' from provider '%s': %w", identityKey, providerID, err)
		}

		return result, nil
	}

	// No provider prefix - query all identity providers
	providerMap := c.GetProvidersByCapability(models.IdentityCapabilities...)

	if len(providerMap) == 0 {
		return nil, fmt.Errorf("identity not found: %s (no identity providers configured)", identity)
	}

	// Query all providers in parallel and collect results
	var wg sync.WaitGroup
	var mu sync.Mutex

	// Collect all results from all providers
	type providerResult struct {
		provider models.Provider
		identity *models.Identity
	}
	results := make([]providerResult, 0)

	for _, provider := range providerMap {
		wg.Add(1)
		go func(p models.Provider) {
			defer wg.Done()

			result, err := p.GetIdentity(ctx, identityKey)
			if err != nil {
				logrus.WithError(err).WithFields(logrus.Fields{
					"provider": p.GetName(),
					"identity": identityKey,
				}).Debug("Failed to get identity from provider")
				return
			}

			if result != nil {
				mu.Lock()
				results = append(results, providerResult{
					provider: p,
					identity: result,
				})
				mu.Unlock()
			}
		}(provider)
	}

	// Wait for all goroutines to complete
	wg.Wait()

	// If no results found, return error
	if len(results) == 0 {
		return nil, fmt.Errorf("identity not found: %s", identityKey)
	}

	// Sort results by mappable identifier first, then by provider name for deterministic ordering.
	// GetMappableIdentifier is expected to return a stable, comparable string that is the same
	// for logically equivalent identities across providers (for example, a normalized email or
	// username). We sort lexicographically on this identifier so that we can reliably pick a
	// canonical "first" identity when multiple providers return matching records. Provider name
	// is used only as a tie-breaker when the mappable identifiers are equal, to keep the overall
	// ordering deterministic but not semantically significant.
	slices.SortFunc(results, func(a, b providerResult) int {
		// First sort by identity's mappable identifier (lexicographical order).
		idCompare := strings.Compare(a.identity.GetMappableIdentifier(), b.identity.GetMappableIdentifier())
		if idCompare != 0 {
			return idCompare
		}
		// If the mappable identifiers are the same, sort by provider name for determinism.
		return strings.Compare(a.provider.GetName(), b.provider.GetName())
	})

	// After sorting, the "alphabetically first" identity is the one with the smallest
	// mappable identifier (and, if equal, the smallest provider name). This identity
	// is treated as the canonical/base record that other identities are merged into.
	baseIdentity := results[0].identity
	baseIdentity.AddProvider(results[0].provider)

	// Merge remaining results into the base identity
	for i := 1; i < len(results); i++ {
		baseIdentity.AddProvider(results[i].provider)
		MergeIdentities(baseIdentity, results[i].identity)
	}

	return baseIdentity, nil
}

// GetIdentitiesWithFilter retrieves identities from all identity providers that support identity listing.
// It applies an optional filter to narrow down the results.
// If no identity providers are found, it returns the current user as the only identity.
// The identityType parameter can be used to filter results by type (user, group, or all).
// the user can be nil here if there is no authenticated user context
func (c *Config) GetIdentitiesWithFilter(
	user *models.User,
	identityType IdentityType,
	searchRequest *models.SearchRequest,
) ([]models.SearchResult[models.Identity], error) {

	// Create our slice to hold identities
	identities := []models.SearchResult[models.Identity]{}

	// Find providers with identity capabilities
	providerMap := c.GetProvidersByCapabilityWithUser(user, models.IdentityCapabilities...)

	// If no identity providers found, return just the current user
	if len(providerMap) == 0 {
		// Apply filter to current user if specified
		if searchRequest != nil && len(searchRequest.Terms) > 0 {
			userFields := []string{strings.ToLower(user.Name), strings.ToLower(user.Email)}
			matchesFilter := slices.ContainsFunc(userFields, func(field string) bool {
				return strings.Contains(field, strings.ToLower(searchRequest.Terms[0]))
			})
			if matchesFilter && user != nil {
				// The default user matches the filter
				identities = []models.SearchResult[models.Identity]{{
					Result: models.Identity{
						ID:    user.Email,
						Label: user.Name,
						User:  user,
					},
				}}
			}
		}

	} else {

		// Query all identity providers in parallel
		ctx := context.Background()
		var wg sync.WaitGroup
		var mu sync.Mutex

		// Map to aggregate identities by name (to avoid duplicates across providers)
		identityMap := make(map[string]models.SearchResult[models.Identity])

		// Channel to collect errors
		errorChan := make(chan error, len(providerMap))

		for _, provider := range providerMap {
			wg.Add(1)
			go func(p models.Provider) {
				defer wg.Done()

				// Query identities from this provider with filter
				var identities []models.SearchResult[models.Identity]
				var err error

				identities, err = p.ListIdentities(ctx, searchRequest)

				if err != nil {
					logrus.WithError(err).
						WithField("provider", p.GetName()).
						Error("Failed to get identities from provider")
					errorChan <- err
					return
				}

				// Add identities to the map (thread-safe)
				mu.Lock()
				for _, identityResult := range identities {

					identity := identityResult.Result

					if identityType == IdentityTypeUser && identity.User == nil {
						continue
					}
					if identityType == IdentityTypeGroup && identity.Group == nil {
						continue
					}

					identity.AddProvider(p)

					mappableIdentifier := identity.GetMappableIdentifier()

					var applyResult models.Identity

					// Use identity ID as key to avoid duplicates
					// If the same identity exists from multiple providers, keep the first one
					if foundIdentity, exists := identityMap[mappableIdentifier]; !exists {

						applyResult = identity

					} else {

						// Also check if we need to update User or Group info
						// with any missing details
						if identity.User != nil && foundIdentity.Result.User == nil {
							foundIdentity.Result.User = identity.User
						}
						if identity.Group != nil && foundIdentity.Result.Group == nil {
							foundIdentity.Result.Group = identity.Group
						}

						applyResult = foundIdentity.Result
					}

					identityMap[mappableIdentifier] = models.SearchResult[models.Identity]{
						Result: applyResult,
						Score:  identityResult.Score,
						ID:     identityResult.ID,
						Reason: identityResult.Reason,
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
		for _, identity := range identityMap {
			identities = append(identities, identity)
		}
	}

	// If no results, no filter, and the identity type includes users,
	// return the current user as the only result
	if len(identities) == 0 &&
		(searchRequest == nil || searchRequest.IsEmpty()) &&
		user != nil &&
		(identityType == IdentityTypeUser || identityType == IdentityTypeAll) {
		identities = append(identities, models.SearchResult[models.Identity]{
			Result: models.Identity{
				ID:    user.Email,
				Label: user.Name,
				User:  user,
			},
		})
	}

	return identities, nil

}
