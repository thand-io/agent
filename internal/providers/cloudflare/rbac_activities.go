package cloudflare

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudflare/cloudflare-go"
	"github.com/thand-io/agent/internal/models"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
)

// cloudflareProviderActivities exposes Cloudflare account member operations as
// individual Temporal activities.
type cloudflareProviderActivities struct {
	provider *cloudflareProvider
}

// ─────────────────────────────────────────────────────────────────────────────
// Request / response types
// ─────────────────────────────────────────────────────────────────────────────

type AuthorizeAccountMemberRequest struct {
	User *models.User `json:"user"`
	Role *models.Role `json:"role"`
}

type AuthorizeAccountMemberResponse struct {
	MemberID string `json:"member_id"`
	Status   string `json:"status"`
	Updated  bool   `json:"updated"`
}

type RevokeAccountMemberRequest struct {
	UserEmail string `json:"user_email"`
	MemberID  string `json:"member_id"` // may be empty — activity will look it up
}

// ─────────────────────────────────────────────────────────────────────────────
// Activity implementations
// ─────────────────────────────────────────────────────────────────────────────

// AuthorizeAccountMember builds the policy set for the role and either creates a
// new account member or updates an existing one. The operation is idempotent.
func (a *cloudflareProviderActivities) AuthorizeAccountMember(
	ctx context.Context,
	req *AuthorizeAccountMemberRequest,
) (*AuthorizeAccountMemberResponse, error) {
	accountID := a.provider.GetAccountID()
	accountRC := cloudflare.AccountIdentifier(accountID)

	params := cloudflare.CreateAccountMemberParams{
		EmailAddress: req.User.Email,
	}

	if err := a.provider.buildMembershipFromRole(ctx, &params, req.Role); err != nil {
		return nil, fmt.Errorf("failed to build policies: %w", err)
	}

	// Check if the member already exists
	existingMember, err := a.provider.findAccountMember(ctx, req.User.Email)
	if err == nil && existingMember != nil {
		existingMember.Roles = nil

		updatedMember, err := a.provider.client.UpdateAccountMember(ctx, accountID, existingMember.ID, *existingMember)
		if err != nil {
			return nil, fmt.Errorf("failed to update account member: %w", err)
		}
		return &AuthorizeAccountMemberResponse{
			MemberID: updatedMember.ID,
			Status:   updatedMember.Status,
			Updated:  true,
		}, nil
	}

	params.Roles = nil

	member, err := a.provider.client.CreateAccountMember(ctx, accountRC, params)
	if err != nil {
		attempt := activity.GetInfo(ctx).Attempt
		return nil, temporal.NewApplicationErrorWithOptions(
			fmt.Sprintf("failed to create cloudflare account member on attempt %d", attempt),
			"CloudflareAccountMemberCreationError",
			temporal.ApplicationErrorOptions{
				NextRetryDelay: 3 * time.Second,
				Cause:          err,
			},
		)
	}

	return &AuthorizeAccountMemberResponse{
		MemberID: member.ID,
		Status:   member.Status,
	}, nil
}

// RevokeAccountMember deletes an account member. If MemberID is empty the member is
// looked up by email first.
func (a *cloudflareProviderActivities) RevokeAccountMember(
	ctx context.Context,
	req *RevokeAccountMemberRequest,
) error {
	memberID := req.MemberID
	accountID := a.provider.GetAccountID()

	if len(memberID) == 0 {
		members, _, err := a.provider.client.AccountMembers(ctx, accountID, cloudflare.PaginationOptions{})
		if err != nil {
			return fmt.Errorf("failed to list account members: %w", err)
		}
		for _, m := range members {
			if m.User.Email == req.UserEmail {
				memberID = m.ID
				break
			}
		}
		if len(memberID) == 0 {
			return fmt.Errorf("user %s not found in account members", req.UserEmail)
		}
	}

	if err := a.provider.client.DeleteAccountMember(ctx, accountID, memberID); err != nil {
		return fmt.Errorf("failed to delete account member: %w", err)
	}
	return nil
}
