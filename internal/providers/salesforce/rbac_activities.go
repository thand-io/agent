package salesforce

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/thand-io/agent/internal/models"
	"go.temporal.io/sdk/temporal"
)

// salesForceProviderActivities exposes granular Salesforce RBAC operations as
// individual Temporal activities.
type salesForceProviderActivities struct {
	provider *salesForceProvider
}

// ─────────────────────────────────────────────────────────────────────────────
// Request / response types
// ─────────────────────────────────────────────────────────────────────────────

type GetSalesforceRoleProfileRequest struct {
	RoleInherits string `json:"role_inherits"` // raw value from role.Inherits[0]
}

type GetSalesforceRoleProfileResponse struct {
	ProfileID string `json:"profile_id"`
}

type FindOrCreateSalesforceUserRequest struct {
	User      *models.User `json:"user"`
	ProfileID string       `json:"profile_id"`
}

type FindOrCreateSalesforceUserResponse struct {
	SalesforceUserID string `json:"salesforce_user_id"`
	CurrentProfileID string `json:"current_profile_id"`
}

type UpdateSalesforceUserProfileRequest struct {
	SalesforceUserID string `json:"salesforce_user_id"`
	ProfileID        string `json:"profile_id"`
}

type FindSalesforceUserRequest struct {
	UserEmail string `json:"user_email"`
}

type FindSalesforceUserResponse struct {
	SalesforceUserID string `json:"salesforce_user_id"`
	CurrentProfileID string `json:"current_profile_id"`
}

type RevertSalesforceUserProfileRequest struct {
	SalesforceUserID string `json:"salesforce_user_id"`
	CurrentProfileID string `json:"current_profile_id"`
	PriorProfileID   string `json:"prior_profile_id"` // may be empty — activity will look up default
}

// ─────────────────────────────────────────────────────────────────────────────
// Activity implementations
// ─────────────────────────────────────────────────────────────────────────────

// GetSalesforceRoleProfile resolves the Salesforce profile ID for the given role.
func (a *salesForceProviderActivities) GetSalesforceRoleProfile(
	ctx context.Context,
	req *GetSalesforceRoleProfileRequest,
) (*GetSalesforceRoleProfileResponse, error) {
	profileName := strings.TrimPrefix(req.RoleInherits,
		fmt.Sprintf("%s:", a.provider.GetProvider()))

	profileResult, err := a.provider.GetRole(ctx, profileName)
	if err != nil {
		return nil, temporal.NewApplicationErrorWithOptions(
			fmt.Sprintf("failed to get role profile: %v", err),
			"SalesforceGetRoleError",
			temporal.ApplicationErrorOptions{
				NextRetryDelay: 3 * time.Second,
				Cause:          err,
			},
		)
	}
	return &GetSalesforceRoleProfileResponse{ProfileID: profileResult.ID}, nil
}

// FindOrCreateSalesforceUser finds the Salesforce user by email or creates them,
// returning the user ID and their current profile ID.
func (a *salesForceProviderActivities) FindOrCreateSalesforceUser(
	ctx context.Context,
	req *FindOrCreateSalesforceUserRequest,
) (*FindOrCreateSalesforceUserResponse, error) {
	userQuery := "SELECT Id, Name, ProfileId FROM User WHERE Email = ?"
	userResult, err := a.provider.queryWithParams(userQuery, req.User.Email)
	if err != nil {
		return nil, temporal.NewApplicationErrorWithOptions(
			fmt.Sprintf("failed to query user: %v", err),
			"SalesforceUserQueryError",
			temporal.ApplicationErrorOptions{
				NextRetryDelay: 3 * time.Second,
				Cause:          err,
			},
		)
	}

	if len(userResult.Records) == 0 {
		newUserObj, err := a.provider.createUser(req.User, req.ProfileID)
		if err != nil {
			return nil, err
		}
		return &FindOrCreateSalesforceUserResponse{
			SalesforceUserID: newUserObj.ID(),
			CurrentProfileID: req.ProfileID,
		}, nil
	}

	return &FindOrCreateSalesforceUserResponse{
		SalesforceUserID: userResult.Records[0].ID(),
		CurrentProfileID: userResult.Records[0].StringField("ProfileId"),
	}, nil
}

// UpdateSalesforceUserProfile sets the user's profile to the target profile.
func (a *salesForceProviderActivities) UpdateSalesforceUserProfile(
	ctx context.Context,
	req *UpdateSalesforceUserProfileRequest,
) error {
	client := a.provider.client
	userObj := client.SObject("User")
	userObj.Set("Id", req.SalesforceUserID)
	userObj.Set("ProfileId", req.ProfileID)

	result := userObj.Update()
	if result == nil {
		return temporal.NewApplicationErrorWithOptions(
			"failed to update user profile",
			"SalesforceProfileUpdateError",
			temporal.ApplicationErrorOptions{
				NextRetryDelay: 3 * time.Second,
			},
		)
	}
	return nil
}

// FindSalesforceUser looks up a Salesforce user by email.
func (a *salesForceProviderActivities) FindSalesforceUser(
	ctx context.Context,
	req *FindSalesforceUserRequest,
) (*FindSalesforceUserResponse, error) {
	userQuery := "SELECT Id, Name, ProfileId FROM User WHERE Email = ?"
	userResult, err := a.provider.queryWithParams(userQuery, req.UserEmail)
	if err != nil {
		return nil, temporal.NewApplicationErrorWithOptions(
			fmt.Sprintf("failed to query user: %v", err),
			"SalesforceUserQueryError",
			temporal.ApplicationErrorOptions{
				NextRetryDelay: 3 * time.Second,
				Cause:          err,
			},
		)
	}
	if len(userResult.Records) == 0 {
		return nil, fmt.Errorf("user not found in Salesforce")
	}
	return &FindSalesforceUserResponse{
		SalesforceUserID: userResult.Records[0].StringField("Id"),
		CurrentProfileID: userResult.Records[0].StringField("ProfileId"),
	}, nil
}

// RevertSalesforceUserProfile reverts the user's profile to priorProfileID. If
// priorProfileID is empty, a default profile is located and used.
func (a *salesForceProviderActivities) RevertSalesforceUserProfile(
	ctx context.Context,
	req *RevertSalesforceUserProfileRequest,
) error {
	priorProfileID := req.PriorProfileID

	if len(priorProfileID) == 0 {
		defaultProfiles := []string{"Standard User", "Minimum Access - Salesforce"}
		for _, name := range defaultProfiles {
			q := "SELECT Id FROM Profile WHERE Name = ?"
			r, err := a.provider.queryWithParams(q, name)
			if err != nil {
				continue
			}
			if len(r.Records) > 0 {
				priorProfileID = r.Records[0].StringField("Id")
				break
			}
		}
		if len(priorProfileID) == 0 {
			return fmt.Errorf("no suitable default profile found in Salesforce")
		}
	}

	if req.CurrentProfileID == priorProfileID {
		return nil // already on target profile
	}

	client := a.provider.client
	userObj := client.SObject("User")
	userObj.Set("Id", req.SalesforceUserID)
	userObj.Set("ProfileId", priorProfileID)

	result := userObj.Update()
	if result == nil {
		return temporal.NewApplicationErrorWithOptions(
			"failed to revert user profile",
			"SalesforceProfileUpdateError",
			temporal.ApplicationErrorOptions{
				NextRetryDelay: 3 * time.Second,
			},
		)
	}
	return nil
}
