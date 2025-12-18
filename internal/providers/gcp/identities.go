package gcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
	"google.golang.org/api/cloudresourcemanager/v1"
)

// SynchronizeIdentities fetches and caches user and group identities from GCP IAM
func (p *gcpProvider) SynchronizeIdentities(ctx context.Context, req *models.SynchronizeIdentitiesRequest) (*models.SynchronizeIdentitiesResponse, error) {
	startTime := time.Now()
	defer func() {
		elapsed := time.Since(startTime)
		logrus.Debugf("Refreshed GCP identities in %s", elapsed)
	}()

	if p.crmClient == nil {
		return nil, fmt.Errorf("GCP CRM client is not initialized")
	}

	projectId := p.GetProjectId()

	if len(projectId) == 0 {
		return nil, fmt.Errorf("GCP project ID is not configured")
	}

	// Get current IAM policy to extract all members - request version 3 for conditions support
	policy, err := p.crmClient.Projects.GetIamPolicy(projectId, &cloudresourcemanager.GetIamPolicyRequest{
		Options: &cloudresourcemanager.GetPolicyOptions{
			RequestedPolicyVersion: 3,
		},
	}).Do()

	if err != nil {
		return nil, fmt.Errorf("failed to get IAM policy: %w", err)
	}

	var identities []models.Identity
	memberSet := make(map[string]bool) // To deduplicate members across bindings

	var userCount, groupCount int

	// Extract unique members from all bindings
	for _, binding := range policy.Bindings {
		for _, member := range binding.Members {
			if memberSet[member] {
				continue // Skip duplicates
			}
			memberSet[member] = true

			identity, identityType := parseMemberToIdentity(member)
			if identity == nil {
				continue
			}

			switch identityType {
			case "user":
				userCount++
			case "group":
				groupCount++
			}

			identities = append(identities, *identity)
		}
	}

	logrus.WithFields(logrus.Fields{
		"users":      userCount,
		"groups":     groupCount,
		"total":      len(identities),
		"project_id": projectId,
	}).Debug("Refreshed GCP identities from IAM policy")

	return &models.SynchronizeIdentitiesResponse{
		Identities: identities,
	}, nil
}

// parseMemberToIdentity converts a GCP IAM member string to an Identity
// Member format: "user:email@example.com", "group:group@example.com"
// Only users and groups are supported for now
func parseMemberToIdentity(member string) (*models.Identity, string) {
	parts := strings.SplitN(member, ":", 2)
	if len(parts) != 2 {
		return nil, ""
	}

	memberType := parts[0]
	memberValue := parts[1]

	switch memberType {
	case "user":
		// Regular user account
		name := extractNameFromEmail(memberValue)
		return &models.Identity{
			ID:    memberValue,
			Label: name,
			User: &models.User{
				ID:    memberValue,
				Email: memberValue,
				Username: func() string {
					username, _, found := strings.Cut(memberValue, "@")
					if !found {
						return memberValue
					}
					return username
				}(),
				Name:   name,
				Source: "gcp",
			},
		}, "user"

	case "group":
		// Google group
		name := extractNameFromEmail(memberValue)
		return &models.Identity{
			ID:    memberValue,
			Label: name,
			Group: &models.Group{
				ID:    memberValue,
				Name:  name,
				Email: memberValue,
			},
		}, "group"

	default:
		// Skip service accounts, domains, allUsers, allAuthenticatedUsers, etc.
		return nil, ""
	}
}

// extractNameFromEmail extracts a display name from an email address
func extractNameFromEmail(email string) string {
	// Try to extract name from email format (e.g., "john.doe@example.com" -> "John Doe")
	parts := strings.Split(email, "@")
	if len(parts) == 0 {
		return email
	}

	localPart := parts[0]
	// Replace dots, underscores, and hyphens with spaces
	name := strings.ReplaceAll(localPart, ".", " ")
	name = strings.ReplaceAll(name, "_", " ")
	name = strings.ReplaceAll(name, "-", " ")

	// Capitalize each word
	words := strings.Fields(name)
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(word[:1]) + strings.ToLower(word[1:])
		}
	}

	return strings.Join(words, " ")
}
