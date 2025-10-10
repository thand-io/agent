package salesforce

import (
	"context"
	"fmt"
	"strings"

	"github.com/thand-io/agent/internal/models"
)

func (p *salesForceProvider) AuthorizeRole(
	ctx context.Context, user *models.User, role *models.Role,
) (map[string]any, error) {

	client := p.client

	// First find the user by their email (using parameterized query to prevent injection)
	userQuery := "SELECT Id, Name, ProfileId FROM User WHERE Email = ?"
	userResult, err := p.queryWithParams(userQuery, user.Email)
	if err != nil {
		return nil, fmt.Errorf("failed to query user: %w", err)
	}

	if len(userResult.Records) == 0 {
		// TODO Create the user if they don't exist?
		return nil, fmt.Errorf("user not found in Salesforce")
	}

	primaryUser := userResult.Records[0]

	salesforceUserId := primaryUser.StringField("Id")
	currentProfileId := primaryUser.StringField("ProfileId")

	// For salesforce we must inherit the role by changing the user's profile

	if len(role.Inherits) > 1 {
		return nil, fmt.Errorf("salesforce roles (profiles) can only inherit one role")
	}

	profileName := strings.TrimPrefix(
		role.Inherits[0], fmt.Sprintf("%s:", p.GetProvider()))

	profileReesult, err := p.GetRole(ctx, profileName)

	if err != nil {
		return nil, fmt.Errorf("failed to get role profile: %w", err)
	}

	// We need to store the old profile Id so we can revert it on revoke
	salesforceProfile := map[string]any{
		"salesforce": map[string]any{
			"id":              salesforceUserId,
			"current_profile": profileReesult.Id,
			"prior_profile":   currentProfileId,
		},
	}

	// Check if user already has the target profile
	if currentProfileId == profileReesult.Id {
		return salesforceProfile, nil // User already has the correct profile
	}

	// Update user's profile
	userObj := client.SObject("User")
	userObj.Set("Id", salesforceUserId)
	userObj.Set("ProfileId", profileReesult.Id)

	result := userObj.Update()
	if result == nil {
		return nil, fmt.Errorf("failed to update user profile")
	}

	return salesforceProfile, nil
}

// Revoke removes access for a user from a role by reverting to a default profile
func (p *salesForceProvider) RevokeRole(
	ctx context.Context,
	user *models.User,
	role *models.Role,
	metadata map[string]any,
) (map[string]any, error) {
	client := p.client

	// First find the user by their email
	userQuery := "SELECT Id, Name, ProfileId FROM User WHERE Email = ?"
	userResult, err := p.queryWithParams(userQuery, user.Email)
	if err != nil {
		return nil, fmt.Errorf("failed to query user: %w", err)
	}

	if len(userResult.Records) == 0 {
		return nil, fmt.Errorf("user not found in Salesforce")
	}

	salesforceUserId := userResult.Records[0].StringField("Id")
	currentProfileId := userResult.Records[0].StringField("ProfileId")

	// Check if the user currently has the role profile that we want to revoke
	roleProfileQuery := "SELECT Id FROM Profile WHERE Name = ?"
	roleProfileResult, err := p.queryWithParams(roleProfileQuery, role.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to query role profile: %w", err)
	}

	if len(roleProfileResult.Records) == 0 {
		return nil, fmt.Errorf("profile '%s' not found in Salesforce", role.Name)
	}

	roleProfileId := roleProfileResult.Records[0].StringField("Id")

	// If user doesn't have the role profile, nothing to revoke
	if currentProfileId != roleProfileId {
		return nil, nil // User doesn't have this profile, nothing to revoke
	}

	// Find a default profile to assign (typically "Standard User" or similar)
	// You may want to make this configurable based on your organization's needs
	defaultProfileQuery := "SELECT Id FROM Profile WHERE Name = 'Standard User'"
	defaultProfileResult, err := p.queryWithParams(defaultProfileQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to query default profile: %w", err)
	}

	if len(defaultProfileResult.Records) == 0 {
		// If "Standard User" doesn't exist, try "Minimum Access - Salesforce"
		defaultProfileQuery = "SELECT Id FROM Profile WHERE Name = 'Minimum Access - Salesforce'"
		defaultProfileResult, err = p.queryWithParams(defaultProfileQuery)
		if err != nil {
			return nil, fmt.Errorf("failed to query fallback default profile: %w", err)
		}
		if len(defaultProfileResult.Records) == 0 {
			return nil, fmt.Errorf("no suitable default profile found in Salesforce")
		}
	}

	defaultProfileId := defaultProfileResult.Records[0].StringField("Id")

	// Update user's profile to the default profile
	userObj := client.SObject("User")
	userObj.Set("Id", salesforceUserId)
	userObj.Set("ProfileId", defaultProfileId)

	result := userObj.Update()
	if result == nil {
		return nil, fmt.Errorf("failed to update user profile to default")
	}

	return nil, nil
}
