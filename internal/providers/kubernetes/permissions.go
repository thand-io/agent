package kubernetes

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

func (p *kubernetesProvider) SynchronizePermissions(ctx context.Context, req *models.SynchronizePermissionsRequest) (*models.SynchronizePermissionsResponse, error) {
	// Discover permissions dynamically from Kubernetes API server
	permissions, err := p.discoverPermissionsFromAPI()
	if err != nil {
		logrus.WithError(err).Warn("Failed to discover permissions from API, falling back to static list")
		// Fallback to a minimal static list if API discovery fails
		permissions = p.getStaticPermissions()
	}

	logrus.WithFields(logrus.Fields{
		"permissions": len(permissions),
	}).Debug("Loaded Kubernetes permissions")

	return &models.SynchronizePermissionsResponse{
		Permissions: permissions,
	}, nil
}

// discoverPermissionsFromAPI dynamically discovers available API resources and verbs from K8s API
func (p *kubernetesProvider) discoverPermissionsFromAPI() ([]models.ProviderPermission, error) {
	client := p.GetClient()
	if client == nil {
		return nil, fmt.Errorf("kubernetes client not available")
	}

	var permissions []models.ProviderPermission

	// Discover API groups and resources
	_, apiResourceLists, err := client.Discovery().ServerGroupsAndResources()
	if err != nil {
		return nil, fmt.Errorf("failed to discover API groups and resources: %w", err)
	}

	// Standard verbs that Kubernetes supports
	standardVerbs := []string{"get", "list", "create", "update", "patch", "delete", "watch"}

	// Process API resources
	for _, apiResourceList := range apiResourceLists {
		for _, resource := range apiResourceList.APIResources {
			// Skip subresources (contain '/')
			if strings.Contains(resource.Name, "/") {
				continue
			}

			var resourceIdentifier string
			if apiResourceList.GroupVersion == "v1" {
				// Core API group
				resourceIdentifier = resource.Name
			} else {
				// Other API groups
				gv := strings.Split(apiResourceList.GroupVersion, "/")
				if len(gv) >= 1 {
					apiGroup := gv[0]
					resourceIdentifier = fmt.Sprintf("%s/%s", apiGroup, resource.Name)
				} else {
					continue
				}
			}

			for _, verb := range standardVerbs {
				if slices.Contains(resource.Verbs, verb) {
					permissionName := fmt.Sprintf("k8s:%s:%s", resourceIdentifier, verb)
					description := fmt.Sprintf("%s %s", cases.Title(language.Und).String(verb), resource.Name)
					if apiResourceList.GroupVersion != "v1" {
						description += fmt.Sprintf(" (%s)", apiResourceList.GroupVersion)
					}

					permissions = append(permissions, models.ProviderPermission{
						ID:          permissionName,
						Name:        permissionName,
						Description: description,
					})
				}
			}
		}
	}

	// Add special permissions
	permissions = append(permissions, models.ProviderPermission{
		ID:          "k8s:*:*",
		Name:        "k8s:*:*",
		Description: "All permissions (admin access)",
	})

	logrus.WithFields(logrus.Fields{
		"discovered": len(permissions),
		"resources":  len(apiResourceLists),
	}).Info("Discovered Kubernetes permissions from API")

	return permissions, nil
}

// getStaticPermissions tries to extract permissions from built-in ClusterRoles as fallback
func (p *kubernetesProvider) getStaticPermissions() []models.ProviderPermission {
	logrus.Warn("Using fallback permissions - trying to extract from built-in ClusterRoles")

	// Try to get permissions from built-in Kubernetes ClusterRoles
	if permissions := p.extractPermissionsFromBuiltinRoles(); len(permissions) > 0 {
		return permissions
	}

	logrus.Warn("Could not extract from ClusterRoles, using minimal hardcoded fallback")

	// Last resort: minimal hardcoded permissions (only the most essential ones)
	return []models.ProviderPermission{
		{Name: "k8s:pods:get", Description: "Get pods"},
		{Name: "k8s:pods:list", Description: "List pods"},
		{Name: "k8s:services:get", Description: "Get services"},
		{Name: "k8s:services:list", Description: "List services"},
		{Name: "k8s:configmaps:get", Description: "Get configmaps"},
		{Name: "k8s:secrets:get", Description: "Get secrets"},
		{Name: "k8s:namespaces:get", Description: "Get namespaces"},
		{Name: "k8s:namespaces:list", Description: "List namespaces"},
		{Name: "k8s:apps/deployments:get", Description: "Get deployments"},
		{Name: "k8s:apps/deployments:list", Description: "List deployments"},
	}
}

// extractPermissionsFromBuiltinRoles extracts permissions from Kubernetes built-in ClusterRoles
func (p *kubernetesProvider) extractPermissionsFromBuiltinRoles() []models.ProviderPermission {
	client := p.GetClient()
	if client == nil {
		return nil
	}

	var permissions []models.ProviderPermission
	permissionSet := make(map[string]string) // dedup permissions

	// Built-in ClusterRoles that contain good permission examples
	builtinRoles := []string{"view", "edit", "admin", "cluster-admin", "system:node", "system:controller:deployment-controller"}

	for _, roleName := range builtinRoles {
		clusterRole, err := client.RbacV1().ClusterRoles().Get(context.TODO(), roleName, metav1.GetOptions{})
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"role":  roleName,
				"error": err,
			}).Debug("Could not get built-in ClusterRole")
			continue
		}

		// Extract permissions from the ClusterRole rules
		for _, rule := range clusterRole.Rules {
			for _, apiGroup := range rule.APIGroups {
				for _, resource := range rule.Resources {
					// Skip subresources and wildcards for cleaner permission list
					if strings.Contains(resource, "/") || resource == "*" {
						continue
					}

					for _, verb := range rule.Verbs {
						if verb == "*" {
							continue
						}

						var permissionName string
						if len(apiGroup) == 0 {
							// Core API group
							permissionName = fmt.Sprintf("k8s:%s:%s", resource, verb)
						} else {
							// Include API group in the resource identifier
							permissionName = fmt.Sprintf("k8s:%s/%s:%s", apiGroup, resource, verb)
						}

						description := fmt.Sprintf("%s %s", cases.Title(language.Und).String(verb), resource)
						if len(apiGroup) > 0 {
							description += fmt.Sprintf(" (%s API group)", apiGroup)
						}
						description += fmt.Sprintf(" [from %s ClusterRole]", roleName)

						permissionSet[permissionName] = description
					}
				}
			}
		}
	}

	// Convert map to slice
	for name, desc := range permissionSet {
		permissions = append(permissions, models.ProviderPermission{
			ID:          strings.ToLower(name),
			Name:        name,
			Description: desc,
		})
	}

	// Add the wildcard permission
	permissions = append(permissions, models.ProviderPermission{
		ID:          "k8s:*:*",
		Name:        "k8s:*:*",
		Description: "All permissions (admin access)",
	})

	if len(permissions) > 0 {
		logrus.WithField("extracted", len(permissions)).Info("Extracted permissions from built-in ClusterRoles")
	}

	return permissions
}
