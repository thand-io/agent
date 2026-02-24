package kubernetes

import (
	"context"

	"github.com/thand-io/agent/internal/models"
)

// kubernetesProviderActivities exposes granular Kubernetes provider operations as
// individual Temporal activities.
type kubernetesProviderActivities struct {
	provider *kubernetesProvider
}

// ─────────────────────────────────────────────────────────────────────────────
// Request / response types
// ─────────────────────────────────────────────────────────────────────────────

type AuthorizeNamespacedRoleRequest struct {
	User      *models.User `json:"user"`
	Role      *models.Role `json:"role"`
	Namespace string       `json:"namespace"`
}

type AuthorizeClusterRoleRequest struct {
	User *models.User `json:"user"`
	Role *models.Role `json:"role"`
}

type RevokeNamespacedRoleRequest struct {
	User      *models.User `json:"user"`
	Role      *models.Role `json:"role"`
	Namespace string       `json:"namespace"`
}

type RevokeClusterRoleRequest struct {
	User *models.User `json:"user"`
	Role *models.Role `json:"role"`
}

// ─────────────────────────────────────────────────────────────────────────────
// Activity implementations
// ─────────────────────────────────────────────────────────────────────────────

// AuthorizeNamespacedRole creates a Role and RoleBinding within the given namespace.
func (a *kubernetesProviderActivities) AuthorizeNamespacedRole(
	ctx context.Context,
	req *AuthorizeNamespacedRoleRequest,
) (*models.AuthorizeRoleResponse, error) {
	return a.provider.authorizeNamespacedRole(ctx, req.User, req.Role, req.Namespace)
}

// AuthorizeClusterRole creates a ClusterRole and ClusterRoleBinding.
func (a *kubernetesProviderActivities) AuthorizeClusterRole(
	ctx context.Context,
	req *AuthorizeClusterRoleRequest,
) (*models.AuthorizeRoleResponse, error) {
	return a.provider.authorizeClusterRole(ctx, req.User, req.Role)
}

// RevokeNamespacedRole deletes the RoleBinding within the given namespace.
func (a *kubernetesProviderActivities) RevokeNamespacedRole(
	ctx context.Context,
	req *RevokeNamespacedRoleRequest,
) (*models.RevokeRoleResponse, error) {
	return a.provider.revokeNamespacedRole(ctx, req.User, req.Role, req.Namespace)
}

// RevokeClusterRole deletes the ClusterRoleBinding.
func (a *kubernetesProviderActivities) RevokeClusterRole(
	ctx context.Context,
	req *RevokeClusterRoleRequest,
) (*models.RevokeRoleResponse, error) {
	return a.provider.revokeClusterRole(ctx, req.User, req.Role)
}
