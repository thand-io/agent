package kubernetes

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
	sdkWorkflowsRunner "github.com/thand-io/agent/sdk/workflows/runner"
	"go.temporal.io/sdk/workflow"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AuthorizeRole grants access for a user to a role
func (p *kubernetesProvider) AuthorizeRole(
	ctx models.ProviderContext,
	req *models.AuthorizeRoleRequest,
) (*models.AuthorizeRoleResponse, error) {
	if workflowCtx, ok := ctx.(workflow.Context); ok {
		return p.authorizeRoleTemporal(workflowCtx, req)
	}

	localCtx := ctx.(context.Context)

	if !req.IsValid() {
		return nil, fmt.Errorf("user and role must be provided to authorize kubernetes role")
	}

	user := req.GetUser()
	role := req.GetRole()

	// Determine scope based on role configuration
	namespace := p.getNamespaceFromRole(&role.Role)

	if len(namespace) > 0 {
		// Create namespaced Role and RoleBinding
		return p.authorizeNamespacedRole(localCtx, user, &role.Role, namespace)
	} else {
		// Create cluster-wide ClusterRole and ClusterRoleBinding
		return p.authorizeClusterRole(localCtx, user, &role.Role)
	}
}

// RevokeRole removes access for a user from a role
func (p *kubernetesProvider) RevokeRole(
	ctx models.ProviderContext,
	req *models.RevokeRoleRequest,
) (*models.RevokeRoleResponse, error) {
	if workflowCtx, ok := ctx.(workflow.Context); ok {
		return p.revokeRoleTemporal(workflowCtx, req)
	}

	localCtx := ctx.(context.Context)

	if !req.IsValid() {
		return nil, fmt.Errorf("user and role must be provided to revoke kubernetes role")
	}

	user := req.GetUser()
	role := req.GetRole()

	namespace := p.getNamespaceFromRole(&role.Role)

	if len(namespace) > 0 {
		return p.revokeNamespacedRole(localCtx, user, &role.Role, namespace)
	} else {
		return p.revokeClusterRole(localCtx, user, &role.Role)
	}
}

func (p *kubernetesProvider) GetAuthorizedAccessUrl(
	ctx context.Context,
	req *models.AuthorizeRoleRequest,
	resp *models.AuthorizeRoleResponse,
) string {

	// TODO: Detect Kubernetes dashboard URL from cluster config or environment

	return p.GetConfig().GetStringWithDefault(
		"sso_start_url", "https://docs.thand.io/environments/kubernetes/")

}

// authorizeRoleTemporal dispatches a single Temporal activity for role creation.
func (p *kubernetesProvider) authorizeRoleTemporal(
	wfCtx workflow.Context,
	req *models.AuthorizeRoleRequest,
) (*models.AuthorizeRoleResponse, error) {
	if !req.IsValid() {
		return nil, fmt.Errorf("user and role must be provided to authorize kubernetes role")
	}

	identifier := p.GetIdentifier()
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 2 * time.Minute,
		RetryPolicy:         sdkWorkflowsRunner.DefaultRetryPolicy,
	}
	wfCtx = workflow.WithActivityOptions(wfCtx, ao)

	user := req.GetUser()
	role := req.GetRole()
	namespace := p.getNamespaceFromRole(&role.Role)

	var resp models.AuthorizeRoleResponse
	if len(namespace) > 0 {
		if err := workflow.ExecuteActivity(
			wfCtx,
			models.CreateTemporalProviderWorkflowName(identifier, AuthorizeNamespacedRoleActivityName),
			&AuthorizeNamespacedRoleRequest{User: user, Role: &role.Role, Namespace: namespace},
		).Get(wfCtx, &resp); err != nil {
			return nil, fmt.Errorf("AuthorizeNamespacedRole activity failed: %w", err)
		}
	} else {
		if err := workflow.ExecuteActivity(
			wfCtx,
			models.CreateTemporalProviderWorkflowName(identifier, AuthorizeClusterRoleActivityName),
			&AuthorizeClusterRoleRequest{User: user, Role: &role.Role},
		).Get(wfCtx, &resp); err != nil {
			return nil, fmt.Errorf("AuthorizeClusterRole activity failed: %w", err)
		}
	}
	return &resp, nil
}

// revokeRoleTemporal dispatches a single Temporal activity for role removal.
func (p *kubernetesProvider) revokeRoleTemporal(
	wfCtx workflow.Context,
	req *models.RevokeRoleRequest,
) (*models.RevokeRoleResponse, error) {
	if !req.IsValid() {
		return nil, fmt.Errorf("user and role must be provided to revoke kubernetes role")
	}

	identifier := p.GetIdentifier()
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 2 * time.Minute,
		RetryPolicy:         sdkWorkflowsRunner.DefaultRetryPolicy,
	}
	wfCtx = workflow.WithActivityOptions(wfCtx, ao)

	user := req.GetUser()
	role := req.GetRole()
	namespace := p.getNamespaceFromRole(&role.Role)

	var resp models.RevokeRoleResponse
	if len(namespace) > 0 {
		if err := workflow.ExecuteActivity(
			wfCtx,
			models.CreateTemporalProviderWorkflowName(identifier, RevokeNamespacedRoleActivityName),
			&RevokeNamespacedRoleRequest{User: user, Role: &role.Role, Namespace: namespace},
		).Get(wfCtx, &resp); err != nil {
			return nil, fmt.Errorf("RevokeNamespacedRole activity failed: %w", err)
		}
	} else {
		if err := workflow.ExecuteActivity(
			wfCtx,
			models.CreateTemporalProviderWorkflowName(identifier, RevokeClusterRoleActivityName),
			&RevokeClusterRoleRequest{User: user, Role: &role.Role},
		).Get(wfCtx, &resp); err != nil {
			return nil, fmt.Errorf("RevokeClusterRole activity failed: %w", err)
		}
	}
	return &resp, nil
}

// authorizeNamespacedRole creates Role and RoleBinding for namespace-scoped access
func (p *kubernetesProvider) authorizeNamespacedRole(
	ctx context.Context,
	user *models.User,
	role *models.Role,
	namespace string,
) (*models.AuthorizeRoleResponse, error) {

	client := p.GetClient()
	roleName := role.GetName()

	// Create or update Role
	k8sRole := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleName,
			Namespace: namespace,
			Labels: map[string]string{
				"thand.io/managed": "true",
				"thand.io/role":    roleName,
			},
		},
		Rules: p.convertPermissionsToRules(role.Permissions),
	}

	_, err := client.RbacV1().Roles(namespace).Create(ctx, k8sRole, metav1.CreateOptions{})
	if err != nil {
		// If role exists, update it using proper error checking
		if apierrors.IsAlreadyExists(err) {
			_, err = client.RbacV1().Roles(namespace).Update(ctx, k8sRole, metav1.UpdateOptions{})
			if err != nil {
				return nil, fmt.Errorf("failed to update role: %w", err)
			}
		} else {
			return nil, fmt.Errorf("failed to create role: %w", err)
		}
	}

	// Create RoleBinding
	bindingName := fmt.Sprintf("%s-%s", roleName, p.sanitizeUserIdentifier(user))
	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      bindingName,
			Namespace: namespace,
			Labels: map[string]string{
				"thand.io/managed": "true",
				"thand.io/role":    roleName,
				"thand.io/user":    p.sanitizeUserIdentifier(user),
			},
		},
		Subjects: []rbacv1.Subject{
			{
				Kind: "User",
				Name: p.getUserIdentifier(user),
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "Role",
			Name:     roleName,
			APIGroup: "rbac.authorization.k8s.io",
		},
	}

	logFields := logrus.Fields{
		"user":      user.GetIdentity(),
		"role":      role.Name,
		"namespace": namespace,
		"binding":   bindingName,
	}

	_, err = client.RbacV1().
		RoleBindings(namespace).
		Create(ctx, roleBinding, metav1.CreateOptions{})
	if err != nil {
		// If role binding exists, update it using proper error checking
		if apierrors.IsAlreadyExists(err) {
			_, err = client.RbacV1().
				RoleBindings(namespace).
				Update(ctx, roleBinding, metav1.UpdateOptions{})
			if err != nil {
				logrus.WithError(err).
					WithFields(logFields).
					Error("Failed to update role binding")
				return nil, fmt.Errorf("failed to update role binding: %w", err)
			}
		} else {
			logrus.WithError(err).
				WithFields(logFields).
				Error("Failed to create role binding")
			return nil, fmt.Errorf("failed to create role binding: %w", err)
		}
	}

	// Log successful authorization
	logrus.WithFields(logFields).
		Info("Successfully authorized user to namespaced role")

	return &models.AuthorizeRoleResponse{
		Metadata: map[string]any{
			"roleName":    roleName,
			"bindingName": bindingName,
			"namespace":   namespace,
			"scope":       "namespaced",
		},
	}, nil
}

// authorizeClusterRole creates ClusterRole and ClusterRoleBinding for cluster-wide access
func (p *kubernetesProvider) authorizeClusterRole(
	ctx context.Context,
	user *models.User,
	role *models.Role,
) (*models.AuthorizeRoleResponse, error) {

	client := p.GetClient()
	roleName := role.GetName()

	// Create or update ClusterRole
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: roleName,
			Labels: map[string]string{
				"thand.io/managed": "true",
				"thand.io/role":    roleName,
			},
		},
		Rules: p.convertPermissionsToRules(role.Permissions),
	}

	_, err := client.RbacV1().
		ClusterRoles().
		Create(ctx, clusterRole, metav1.CreateOptions{})
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			_, err = client.RbacV1().
				ClusterRoles().
				Update(ctx, clusterRole, metav1.UpdateOptions{})
			if err != nil {
				return nil, fmt.Errorf("failed to update cluster role: %w", err)
			}
		} else {
			return nil, fmt.Errorf("failed to create cluster role: %w", err)
		}
	}

	// Create ClusterRoleBinding
	bindingName := fmt.Sprintf("%s-%s", roleName, p.sanitizeUserIdentifier(user))
	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: bindingName,
			Labels: map[string]string{
				"thand.io/managed": "true",
				"thand.io/role":    roleName,
				"thand.io/user":    p.sanitizeUserIdentifier(user),
			},
		},
		Subjects: []rbacv1.Subject{
			{
				Kind: "User",
				Name: p.getUserIdentifier(user),
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "ClusterRole",
			Name:     roleName,
			APIGroup: "rbac.authorization.k8s.io",
		},
	}

	logFields := logrus.Fields{
		"user":    user.GetIdentity(),
		"role":    role.Name,
		"binding": bindingName,
	}

	_, err = client.RbacV1().
		ClusterRoleBindings().
		Create(ctx, clusterRoleBinding, metav1.CreateOptions{})
	if err != nil {
		// If cluster role binding exists, update it using proper error checking
		if apierrors.IsAlreadyExists(err) {
			_, err = client.RbacV1().
				ClusterRoleBindings().
				Update(ctx, clusterRoleBinding, metav1.UpdateOptions{})
			if err != nil {
				logrus.WithError(err).
					WithFields(logFields).
					Error("Failed to update cluster role binding")
				return nil, fmt.Errorf("failed to update cluster role binding: %w", err)
			}
		} else {
			logrus.WithError(err).
				WithFields(logFields).
				Error("Failed to create cluster role binding")
			return nil, fmt.Errorf("failed to create cluster role binding: %w", err)
		}
	}

	// Log successful authorization
	logrus.WithFields(logFields).
		Info("Successfully authorized user to cluster role")

	return &models.AuthorizeRoleResponse{
		Metadata: map[string]any{
			"roleName":    roleName,
			"bindingName": bindingName,
			"scope":       "cluster",
		},
	}, nil
}

// convertPermissionsToRules converts thand permissions to Kubernetes RBAC rules
func (p *kubernetesProvider) convertPermissionsToRules(permissions models.RolePermissions) []rbacv1.PolicyRule {
	var rules []rbacv1.PolicyRule

	// Group permissions by API group and resource
	ruleMap := make(map[string]*rbacv1.PolicyRule)

	// Process Allow statements (Kubernetes RBAC is allow-only)
	for _, stmt := range permissions.Allow {
		for _, operation := range stmt.Operations {
			rule := p.parsePermission(operation)
			if rule != nil {
				key := fmt.Sprintf("%s:%s", strings.Join(rule.APIGroups, ","), strings.Join(rule.Resources, ","))
				if existingRule, exists := ruleMap[key]; exists {
					// Merge verbs
					existingRule.Verbs = append(existingRule.Verbs, rule.Verbs...)
					existingRule.Verbs = p.deduplicateSlice(existingRule.Verbs)
				} else {
					ruleMap[key] = rule
				}
			}
		}
	}

	// Log warning for Deny statements (Kubernetes RBAC doesn't support deny)
	if len(permissions.Deny) > 0 {
		logrus.Warnf("Kubernetes RBAC doesn't support deny permissions, skipping %d deny statements", len(permissions.Deny))
	}

	// Convert map back to slice
	for _, rule := range ruleMap {
		rules = append(rules, *rule)
	}

	return rules
}

// parsePermission converts a permission string to PolicyRule
func (p *kubernetesProvider) parsePermission(permission string) *rbacv1.PolicyRule {
	// Expected formats:
	// "k8s:pods:get" -> get pods in core API group
	// "k8s:apps/deployments:list,watch" -> list,watch deployments in apps API group
	// "k8s:*/secrets:get,create" -> get,create secrets in all namespaces

	parts := strings.Split(permission, ":")
	if len(parts) != 3 {
		logrus.WithField("permission", permission).Warn("Invalid permission format, expected 'prefix:resource:verbs'")
		return nil // Invalid format
	}

	// Validate prefix
	if parts[0] != "k8s" {
		logrus.WithField("permission", permission).Warn("Invalid permission prefix, expected 'k8s'")
		return nil
	}

	apiGroup := ""
	resource := parts[1]
	verbs := strings.Split(parts[2], ",")

	// Validate verbs are not empty
	if len(verbs) == 0 || (len(verbs) == 1 && len(verbs[0]) == 0) {
		logrus.WithField("permission", permission).Warn("Invalid permission: no verbs specified")
		return nil
	}

	// Validate and sanitize verbs
	validVerbs := []string{}
	allowedVerbs := map[string]bool{
		"get": true, "list": true, "create": true, "update": true,
		"patch": true, "delete": true, "watch": true, "deletecollection": true,
	}

	for _, verb := range verbs {
		verb = strings.TrimSpace(verb)
		if len(verb) == 0 {
			continue
		}
		if !allowedVerbs[verb] {
			logrus.WithFields(logrus.Fields{
				"permission": permission,
				"verb":       verb,
			}).Warn("Invalid verb in permission")
			continue
		}
		validVerbs = append(validVerbs, verb)
	}

	if len(validVerbs) == 0 {
		logrus.WithField("permission", permission).Warn("No valid verbs found in permission")
		return nil
	}

	// Parse API group and resource
	if strings.Contains(resource, "/") {
		groupResource := strings.Split(resource, "/")
		if len(groupResource) == 2 {
			apiGroup = groupResource[0]
			resource = groupResource[1]
		} else {
			logrus.WithField("permission", permission).Warn("Invalid API group/resource format")
			return nil
		}
	}

	// Validate resource name (basic validation)
	if len(resource) == 0 {
		logrus.WithField("permission", permission).Warn("Empty resource name in permission")
		return nil
	}

	rule := &rbacv1.PolicyRule{
		APIGroups: []string{apiGroup},
		Resources: []string{resource},
		Verbs:     validVerbs,
	}

	return rule
}

// Security helper functions
func (p *kubernetesProvider) getUserIdentifier(user *models.User) string {
	// Prefer email for OIDC integration, fallback to username
	if len(user.Email) > 0 {
		return user.Email
	}
	return user.Username
}

func (p *kubernetesProvider) sanitizeUserIdentifier(user *models.User) string {
	identifier := p.getUserIdentifier(user)
	// Replace invalid characters for Kubernetes resource names
	identifier = strings.ReplaceAll(identifier, "@", "-at-")
	identifier = strings.ReplaceAll(identifier, ".", "-")
	identifier = strings.ToLower(identifier)
	return identifier
}

func (p *kubernetesProvider) getNamespaceFromRole(role *models.Role) string {
	// Check if role has namespace-specific targets in permission statements
	for _, stmt := range role.Permissions.Allow {
		for _, target := range stmt.Targets {
			if strings.Contains(target, "namespace:") {
				parts := strings.Split(target, ":")
				if len(parts) >= 2 {
					return parts[1]
				}
			}
		}
	}
	return "" // Empty string means cluster-wide
}

func (p *kubernetesProvider) deduplicateSlice(slice []string) []string {
	seen := make(map[string]bool)
	result := []string{}

	for _, item := range slice {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}

	return result
}

// Revocation functions
func (p *kubernetesProvider) revokeNamespacedRole(
	ctx context.Context,
	user *models.User,
	role *models.Role,
	namespace string,
) (*models.RevokeRoleResponse, error) {

	client := p.GetClient()
	bindingName := role.GetName()

	// Check if RoleBinding exists before attempting to delete
	_, err := client.RbacV1().
		RoleBindings(namespace).
		Get(ctx, bindingName, metav1.GetOptions{})
	if err != nil {
		// If the binding doesn't exist, consider it already revoked
		if apierrors.IsNotFound(err) {
			return &models.RevokeRoleResponse{}, nil
		}
		return nil, fmt.Errorf("failed to check role binding existence: %w", err)
	}

	logFields := logrus.Fields{
		"user":      user.GetIdentity(),
		"role":      role.Name,
		"namespace": namespace,
		"binding":   bindingName,
		"scope":     "namespaced",
	}

	// Delete RoleBinding
	err = client.RbacV1().RoleBindings(namespace).Delete(ctx, bindingName, metav1.DeleteOptions{})
	if err != nil {
		logrus.WithError(err).
			WithFields(logFields).
			Error("Failed to delete role binding")
		return nil, fmt.Errorf("failed to delete role binding: %w", err)
	}

	// Log successful revocation
	logrus.WithFields(logFields).
		Info("Successfully revoked user access to namespaced role")

	return &models.RevokeRoleResponse{}, nil
}

func (p *kubernetesProvider) revokeClusterRole(
	ctx context.Context,
	user *models.User,
	role *models.Role,
) (*models.RevokeRoleResponse, error) {

	client := p.GetClient()
	bindingName := role.GetName()

	// Check if ClusterRoleBinding exists before attempting to delete
	_, err := client.RbacV1().
		ClusterRoleBindings().
		Get(ctx, bindingName, metav1.GetOptions{})
	if err != nil {
		// If the binding doesn't exist, consider it already revoked
		if apierrors.IsNotFound(err) {
			return &models.RevokeRoleResponse{}, nil
		}
		return nil, fmt.Errorf("failed to check cluster role binding existence: %w", err)
	}

	logFields := logrus.Fields{
		"user":    user.GetIdentity(),
		"role":    role.Name,
		"binding": bindingName,
		"scope":   "cluster",
	}

	// Delete ClusterRoleBinding
	err = client.RbacV1().
		ClusterRoleBindings().
		Delete(ctx, bindingName, metav1.DeleteOptions{})
	if err != nil {
		logrus.WithError(err).
			WithFields(logFields).
			Error("Failed to delete cluster role binding")
		return nil, fmt.Errorf("failed to delete cluster role binding: %w", err)
	}

	// Log successful revocation
	logrus.WithError(err).
		WithFields(logFields).
		Info("Successfully revoked user access to cluster role")

	return &models.RevokeRoleResponse{}, nil
}
