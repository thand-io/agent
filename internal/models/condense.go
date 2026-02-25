// Package models provides permission condensation and expansion utilities.
//
// These functions handle grouping, expanding, and wildcard-subsumption of
// colon-separated permission strings such as "ec2:DescribeInstances" or
// "k8s:pods:get,list,watch". GCP-style dot-separated permissions
// (e.g. "compute.instances.start") are treated as atomic and never condensed.
package models

import (
	"slices"
	"sort"
	"strings"

	"github.com/sirupsen/logrus"
)

// MaxPermissions is the maximum number of permissions (allow + deny) per role.
// It is also used as a safety limit inside CondenseActions.
const MaxPermissions = 500

// IsCondensablePermission returns true if the permission can be condensed with others.
// GCP-style permissions (with dots in the action part) are not condensable.
func IsCondensablePermission(permission string) bool {
	idx := strings.LastIndex(permission, ":")
	if idx == -1 {
		return false
	}
	// If last segment contains a dot, it's a GCP-style permission (not condensable)
	return !strings.Contains(permission[idx+1:], ".")
}

// ExpandCondensedActions expands "k8s:pods:get,list" into ["k8s:pods:get", "k8s:pods:list"].
// GCP-style permissions are returned as-is.
func ExpandCondensedActions(permission string) []string {
	if !IsCondensablePermission(permission) {
		return []string{permission}
	}

	idx := strings.LastIndex(permission, ":")
	if idx == -1 || !strings.Contains(permission[idx+1:], ",") {
		return []string{permission}
	}

	resource := permission[:idx]
	actions := strings.Split(permission[idx+1:], ",")
	result := make([]string, 0, len(actions))

	for _, action := range actions {
		action = strings.TrimSpace(action)
		if len(action) != 0 {
			result = append(result, resource+":"+action)
		}
	}
	return result
}

// CondenseActions groups permissions by resource and condenses their actions.
// Handles wildcards: "ec2:*" subsumes "ec2:DescribeInstances".
//
// Algorithm:
//  1. Separate atomic (non-condensable like GCP) from condensable permissions
//  2. Track wildcard permissions to subsume specific ones
//  3. Group condensable permissions by resource
//  4. Merge and sort actions for each resource
//  5. Filter out permissions subsumed by wildcards
func CondenseActions(permissions []string) []string {
	if len(permissions) == 0 {
		return nil
	}

	// Enforce upper bound to prevent resource exhaustion
	if len(permissions) > MaxPermissions {
		logrus.Errorf("CondenseActions: permissions slice length %d exceeds maximum %d; returning nil",
			len(permissions), MaxPermissions)
		return nil
	}

	// Pre-allocate with reasonable capacity
	atomic := make([]string, 0, len(permissions)/2)           // Non-condensable permissions
	byResource := make(map[string][]string, len(permissions)) // resource -> actions
	wildcards := make(map[string]bool, len(permissions)/4)    // Tracks wildcard prefixes

	for _, perm := range permissions {
		if before, ok := strings.CutSuffix(perm, ":*"); ok {
			wildcards[before] = true
		}

		if !IsCondensablePermission(perm) {
			atomic = append(atomic, perm)
			continue
		}

		idx := strings.LastIndex(perm, ":")
		resource, action := perm[:idx], perm[idx+1:]
		byResource[resource] = append(byResource[resource], action)
	}

	// Filter out items subsumed by wildcards
	result := make([]string, 0, len(atomic)+len(byResource))

	for _, perm := range atomic {
		if !IsSubsumedByWildcard(perm, wildcards) {
			result = append(result, perm)
		}
	}

	for resource, actions := range byResource {
		// Check if this resource has a wildcard - if so, only output the wildcard
		if slices.Contains(actions, "*") {
			result = append(result, resource+":*")
			continue
		}

		// Check if this resource is subsumed by a DIFFERENT wildcard
		// (A wildcard shouldn't subsume itself)
		isSubsumed := false
		for prefix := range wildcards {
			// Skip if this is the same resource (self-subsumption)
			if prefix == resource {
				continue
			}
			// Check if resource is under a wildcard prefix
			if strings.HasPrefix(resource, prefix+":") {
				isSubsumed = true
				break
			}
		}

		if isSubsumed {
			continue
		}

		if len(actions) == 1 {
			result = append(result, resource+":"+actions[0])
		} else {
			sort.Strings(actions)
			result = append(result, resource+":"+strings.Join(actions, ","))
		}
	}

	sort.Strings(result)
	return result
}

// IsSubsumedByWildcard checks if an item is covered by a wildcard.
func IsSubsumedByWildcard(item string, wildcards map[string]bool) bool {
	for prefix := range wildcards {
		if strings.HasPrefix(item, prefix+":") && item != prefix+":*" {
			return true
		}
	}
	return false
}
