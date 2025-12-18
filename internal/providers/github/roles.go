package github

import (
	"github.com/thand-io/agent/internal/models"
)

// Never synchronize roles from GitHub as they are
// statically defined by GitHub and cannot be modified
func (p *githubProvider) CanSynchronizeRoles() bool {
	return false
}

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
