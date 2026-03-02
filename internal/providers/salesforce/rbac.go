package salesforce

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/simpleforce/simpleforce"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
	sdkWorkflowsRunner "github.com/thand-io/agent/sdk/workflows/runner"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const MetadataPriorProfileKey = "prior_profile"

func (p *salesForceProvider) AuthorizeRole(
	ctx models.ProviderContext,
	req *models.AuthorizeRoleRequest,
) (*models.AuthorizeRoleResponse, error) {
	if workflowCtx, ok := ctx.(workflow.Context); ok {
		return p.authorizeRoleTemporal(workflowCtx, req)
	}
	localCtx, ok := ctx.(context.Context)
	if !ok {
		return nil, fmt.Errorf("invalid context type")
	}

	if !req.IsValid() {
		return nil, fmt.Errorf("user and role must be provided to authorize salesforce role")
	}

	user := req.GetUser()
	role := req.GetRole()

	client := p.client

	// For salesforce we must inherit the role by changing the user's profile

	if len(role.Inherits) > 1 {
		return nil, fmt.Errorf("salesforce roles (profiles) can only inherit one role")
	}

	profileName := strings.TrimPrefix(
		role.Inherits[0], fmt.Sprintf("%s:", p.GetProvider()))

	profileResult, err := p.GetRole(localCtx, profileName)

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

	// First find the user by their email (using parameterized query to prevent injection)
	userQuery := "SELECT Id, Name, ProfileId FROM User WHERE Email = ?"
	userResult, err := p.queryWithParams(userQuery, user.Email)
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

	var primaryUser *simpleforce.SObject

	if len(userResult.Records) == 0 {
		newUserObj, err := p.createUser(user, profileResult.ID)
		if err != nil {
			return nil, err
		}
		primaryUser = newUserObj
	} else {
		primaryUser = &userResult.Records[0]
	}

	salesforceUserId := primaryUser.ID()
	currentProfileId := primaryUser.StringField("ProfileId")

	// Check if user already has the target profile
	if currentProfileId == profileResult.ID {
		logrus.WithFields(logrus.Fields{
			"user_id":    salesforceUserId,
			"user_email": user.Email,
			"profile_id": profileResult.ID,
		}).Info("User already has the target profile in Salesforce, skipping assignment")

		return &models.AuthorizeRoleResponse{
			UserId: salesforceUserId,
			Roles:  []string{profileResult.ID},
			Metadata: map[string]any{
				MetadataPriorProfileKey: currentProfileId,
			},
		}, nil
	}

	// Update user's profile
	userObj := client.SObject("User")
	userObj.Set("Id", salesforceUserId)
	userObj.Set("ProfileId", profileResult.ID)

	result := userObj.Update()
	if result == nil {
		return nil, temporal.NewApplicationErrorWithOptions(
			"failed to update user profile",
			"SalesforceProfileUpdateError",
			temporal.ApplicationErrorOptions{
				NextRetryDelay: 3 * time.Second,
			},
		)
	}

	logrus.WithFields(logrus.Fields{
		"user_id":               salesforceUserId,
		"user_email":            user.Email,
		"profile_id":            profileResult.ID,
		MetadataPriorProfileKey: currentProfileId,
	}).Info("Successfully updated user profile in Salesforce")

	return &models.AuthorizeRoleResponse{
		UserId: salesforceUserId,
		Roles:  []string{profileResult.ID},
		Metadata: map[string]any{
			MetadataPriorProfileKey: currentProfileId,
		},
	}, nil
}

// Revoke removes access for a user from a role by reverting to the prior profile
func (p *salesForceProvider) RevokeRole(
	ctx models.ProviderContext,
	req *models.RevokeRoleRequest,
) (*models.RevokeRoleResponse, error) {
	if workflowCtx, ok := ctx.(workflow.Context); ok {
		return p.revokeRoleTemporal(workflowCtx, req)
	}
	_, ok := ctx.(context.Context)
	if !ok {
		return nil, fmt.Errorf("invalid context type")
	}

	client := p.client

	user := req.GetUser()

	if req.AuthorizeRoleResponse == nil {
		return nil, fmt.Errorf("no authorize role response found for revocation")
	}

	metadata := req.AuthorizeRoleResponse

	// First find the user by their email
	userQuery := "SELECT Id, Name, ProfileId FROM User WHERE Email = ?"
	userResult, err := p.queryWithParams(userQuery, user.Email)
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

	salesforceUserId := userResult.Records[0].StringField("Id")
	currentProfileId := userResult.Records[0].StringField("ProfileId")

	// Get the profile to revert to from metadata
	priorProfileId, ok := metadata.Metadata[MetadataPriorProfileKey].(string)
	if !ok || len(priorProfileId) == 0 {
		// If no prior profile stored, use a default profile
		defaultProfiles := []string{"Standard User", "Minimum Access - Salesforce"}

		for _, profileName := range defaultProfiles {
			defaultProfileQuery := "SELECT Id FROM Profile WHERE Name = ?"
			defaultProfileResult, err := p.queryWithParams(defaultProfileQuery, profileName)
			if err != nil {
				logrus.Warnf("Failed to query default profile '%s': %v", profileName, err)
				continue
			}

			if len(defaultProfileResult.Records) > 0 {
				priorProfileId = defaultProfileResult.Records[0].StringField("Id")
				break
			}
		}

		if len(priorProfileId) == 0 {
			return nil, fmt.Errorf("no suitable default profile found in Salesforce (tried: %s)", strings.Join(defaultProfiles, ", "))
		}
	}

	// If user already has the prior profile, nothing to do
	if currentProfileId == priorProfileId {
		logrus.WithFields(logrus.Fields{
			"user_id":    salesforceUserId,
			"user_email": user.Email,
			"profile_id": priorProfileId,
		}).Info("User already has the prior profile in Salesforce, nothing to revoke")
		return &models.RevokeRoleResponse{}, nil
	}

	// Update user's profile to the prior profile
	userObj := client.SObject("User")
	userObj.Set("Id", salesforceUserId)
	userObj.Set("ProfileId", priorProfileId)

	result := userObj.Update()
	if result == nil {
		return nil, temporal.NewApplicationErrorWithOptions(
			"failed to update user profile to prior profile",
			"SalesforceProfileUpdateError",
			temporal.ApplicationErrorOptions{
				NextRetryDelay: 3 * time.Second,
			},
		)
	}

	logrus.WithFields(logrus.Fields{
		"user_id":               salesforceUserId,
		"user_email":            user.Email,
		MetadataPriorProfileKey: priorProfileId,
		"current_profile":       currentProfileId,
	}).Info("Successfully revoked Salesforce role and reverted to prior profile")

	return &models.RevokeRoleResponse{}, nil
}

// authorizeRoleTemporal sequences Salesforce role authorization as three Temporal activities.
func (p *salesForceProvider) authorizeRoleTemporal(
	wfCtx workflow.Context,
	req *models.AuthorizeRoleRequest,
) (*models.AuthorizeRoleResponse, error) {
	if !req.IsValid() {
		return nil, fmt.Errorf("user and role must be provided to authorize salesforce role")
	}

	identifier := p.GetIdentifier()
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 2 * time.Minute,
		RetryPolicy:         sdkWorkflowsRunner.DefaultRetryPolicy,
	}
	wfCtx = workflow.WithActivityOptions(wfCtx, ao)

	user := req.GetUser()
	role := req.GetRole()

	if len(role.Inherits) == 0 {
		return nil, fmt.Errorf("salesforce roles (profiles) must inherit at least one role")
	}
	if len(role.Inherits) > 1 {
		return nil, fmt.Errorf("salesforce roles (profiles) can only inherit one role")
	}

	// Step 1 — resolve the Salesforce profile ID
	var profileResp GetSalesforceRoleProfileResponse
	if err := workflow.ExecuteActivity(
		wfCtx,
		models.CreateTemporalProviderWorkflowName(identifier, GetSalesforceRoleProfileActivityName),
		&GetSalesforceRoleProfileRequest{RoleInherits: role.Inherits[0]},
	).Get(wfCtx, &profileResp); err != nil {
		return nil, fmt.Errorf("GetSalesforceRoleProfile activity failed: %w", err)
	}

	// Step 2 — find or create the Salesforce user
	var userResp FindOrCreateSalesforceUserResponse
	if err := workflow.ExecuteActivity(
		wfCtx,
		models.CreateTemporalProviderWorkflowName(identifier, FindOrCreateSalesforceUserActivityName),
		&FindOrCreateSalesforceUserRequest{
			User:      user,
			ProfileID: profileResp.ProfileID,
		},
	).Get(wfCtx, &userResp); err != nil {
		return nil, fmt.Errorf("FindOrCreateSalesforceUser activity failed: %w", err)
	}

	// Step 3 — update the user's profile (only if needed)
	if userResp.CurrentProfileID != profileResp.ProfileID {
		if err := workflow.ExecuteActivity(
			wfCtx,
			models.CreateTemporalProviderWorkflowName(identifier, UpdateSalesforceUserProfileActivityName),
			&UpdateSalesforceUserProfileRequest{
				SalesforceUserID: userResp.SalesforceUserID,
				ProfileID:        profileResp.ProfileID,
			},
		).Get(wfCtx, nil); err != nil {
			return nil, fmt.Errorf("UpdateSalesforceUserProfile activity failed: %w", err)
		}
	}

	return &models.AuthorizeRoleResponse{
		UserId: userResp.SalesforceUserID,
		Roles:  []string{profileResp.ProfileID},
		Metadata: map[string]any{
			MetadataPriorProfileKey: userResp.CurrentProfileID,
		},
	}, nil
}

// revokeRoleTemporal sequences Salesforce role revocation as two Temporal activities.
func (p *salesForceProvider) revokeRoleTemporal(
	wfCtx workflow.Context,
	req *models.RevokeRoleRequest,
) (*models.RevokeRoleResponse, error) {
	identifier := p.GetIdentifier()
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 2 * time.Minute,
		RetryPolicy:         sdkWorkflowsRunner.DefaultRetryPolicy,
	}
	wfCtx = workflow.WithActivityOptions(wfCtx, ao)

	user := req.GetUser()

	var priorProfileID string
	if req.AuthorizeRoleResponse != nil && req.AuthorizeRoleResponse.Metadata != nil {
		priorProfileID, _ = req.AuthorizeRoleResponse.Metadata[MetadataPriorProfileKey].(string)
	}

	// Step 1 — find the Salesforce user
	var userResp FindSalesforceUserResponse
	if err := workflow.ExecuteActivity(
		wfCtx,
		models.CreateTemporalProviderWorkflowName(identifier, FindSalesforceUserActivityName),
		&FindSalesforceUserRequest{UserEmail: user.Email},
	).Get(wfCtx, &userResp); err != nil {
		return nil, fmt.Errorf("FindSalesforceUser activity failed: %w", err)
	}

	// Step 2 — revert the user's profile
	if err := workflow.ExecuteActivity(
		wfCtx,
		models.CreateTemporalProviderWorkflowName(identifier, RevertSalesforceUserProfileActivityName),
		&RevertSalesforceUserProfileRequest{
			SalesforceUserID: userResp.SalesforceUserID,
			CurrentProfileID: userResp.CurrentProfileID,
			PriorProfileID:   priorProfileID,
		},
	).Get(wfCtx, nil); err != nil {
		return nil, fmt.Errorf("RevertSalesforceUserProfile activity failed: %w", err)
	}

	return &models.RevokeRoleResponse{}, nil
}

func (p *salesForceProvider) GetAuthorizedAccessUrl(
	ctx context.Context,
	req *models.AuthorizeRoleRequest,
	resp *models.AuthorizeRoleResponse,
) string {

	return p.GetConfig().GetStringWithDefault(
		"sso_start_url",
		p.GetConfig().GetStringWithDefault(
			"endpoint", "https://login.salesforce.com/",
		),
	)

}

func (p *salesForceProvider) createUser(user *models.User, profileId string) (*simpleforce.SObject, error) {
	logrus.WithField("email", user.Email).Info("User not found in Salesforce, creating new user")

	username := user.GetUsername()

	if len(username) == 0 {
		return nil, fmt.Errorf("cannot create salesforce user without username")
	}

	newUser := p.client.SObject("User")
	newUser.Set("Username", user.Email) // Username must be in email format
	newUser.Set("Email", user.Email)

	firstName := user.GetFirstName()
	lastName := user.GetLastName()

	// Salesforce firstname and lastname cannot be empty
	if len(firstName) == 0 {
		firstName = "Unknown"
	}
	if len(lastName) == 0 {
		lastName = "Unknown"
	}

	newUser.Set("FirstName", firstName)
	newUser.Set("LastName", lastName)
	// Salesforce Alias field has a maximum length of 8 characters
	alias := username
	if len(alias) > 8 {
		alias = alias[:8]
	}
	newUser.Set("Alias", alias)

	newUser.Set("TimeZoneSidKey", "America/Los_Angeles")
	newUser.Set("LocaleSidKey", "en_US")
	newUser.Set("EmailEncodingKey", "UTF-8")
	newUser.Set("LanguageLocaleKey", "en_US")
	newUser.Set("ProfileId", profileId)

	result := newUser.Create()
	if result == nil {
		return nil, temporal.NewApplicationErrorWithOptions(
			"failed to create user in Salesforce",
			"SalesforceUserCreateError",
			temporal.ApplicationErrorOptions{
				NextRetryDelay: 3 * time.Second,
			},
		)
	}

	return result, nil
}
