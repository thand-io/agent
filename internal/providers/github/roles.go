package github

import (
	"context"
	"fmt"
	"strings"

	"github.com/thand-io/agent/internal/models"
)

/*
All-repository read: Grants read access to all repositories in the organization.
All-repository write: Grants write access to all repositories in the organization.
All-repository triage: Grants triage access to all repositories in the organization.
All-repository maintain: Grants maintenance access to all repositories in the organization.
All-repository admin: Grants admin access to all repositories in the organization.
CI/CD admin: Grants admin access to manage Actions policies, runners, runner groups, hosted compute network configurations, secrets, variables, and usage metrics for an organization.
Security manager: Grants the ability to manage security policies, security alerts, and security configurations for an organization and all its repositories.
App Manager: Grants the ability to create, edit, and delete all GitHub Apps in an organization.
*/
var GitHubOrganisationRoles = []models.ProviderRole{{
	Name:        "All-repository read",
	Description: "Grants read access to all repositories in the organization.",
}, {
	Name:        "All-repository write",
	Description: "Grants write access to all repositories in the organization.",
}, {
	Name:        "All-repository triage",
	Description: "Grants triage access to all repositories in the organization.",
}, {
	Name:        "All-repository maintain",
	Description: "Grants maintenance access to all repositories in the organization.",
}, {
	Name:        "All-repository admin",
	Description: "Grants admin access to all repositories in the organization.",
}, {
	Name:        "CI/CD admin",
	Description: "Grants admin access to manage Actions policies, runners, runner groups, hosted compute network configurations, secrets, variables, and usage metrics for an organization.",
}, {
	Name:        "Security manager",
	Description: "Grants the ability to manage security policies, security alerts, and security configurations for an organization and all its repositories.",
}, {
	Name:        "App Manager",
	Description: "Grants the ability to create, edit, and delete all GitHub Apps in an organization.",
}}

var GitHubRoles = append(
	[]models.ProviderRole{},
	GitHubOrganisationRoles...,
)

func (p *githubProvider) GetRole(ctx context.Context, role string) (*models.ProviderRole, error) {
	for _, r := range GitHubRoles {
		if strings.Compare(r.Name, role) == 0 {
			return &r, nil
		}
	}
	return nil, fmt.Errorf("role not found: %s", role)
}

func (p *githubProvider) ListRoles(ctx context.Context, filters ...string) ([]models.ProviderRole, error) {
	// TODO: Implement GitHub ListRoles logic
	return GitHubRoles, nil
}
