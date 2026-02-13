// Package models provides SDK access to elevation types used for privilege escalation workflows.
// These types are aliases to internal models to avoid circular dependencies while exposing
// the complete elevation API surface to SDK consumers.
//
// The elevation system supports three distinct request patterns:
//   - Static: Predefined roles via query parameters or simple JSON (ElevateStaticRequest)
//   - Dynamic: Runtime role creation with complex scoping (ElevateDynamicRequest)
//   - LLM: Natural language to structured request conversion (ElevateLLMRequest)
//
// All elevation requests flow through workflows for approval management and are tracked
// via ElevateResponse status. The system includes sophisticated identity resolution,
// session management, and provider integration.
package models

import (
	internal "github.com/thand-io/agent/internal/models"
)

// ElevateRequest represents the core elevation request structure used across CLI, API, and workflow systems.
// It contains a role definition, target providers, authentication context, workflow configuration,
// reason for access, duration, target identities, and session information.
//
// Key methods:
//   - IsValid(): Validates required fields (role, providers, reason)
//   - ResolveIdentities(): Complex identity resolution with provider fallback logic
//   - AsDuration(): Parses and validates ISO 8601 duration strings
//   - GetWorkflow(): Resolves workflow from request or role defaults
//
// Usage patterns:
//   - CLI: Built from command flags in cmd/cli/access.go
//   - API: Parsed from JSON POST requests in daemon endpoints
//   - LLM: Generated from natural language via AI processing
//   - Workflows: Serialized into encrypted task context for approval processing
type ElevateRequest = internal.ElevateRequest

// ElevateResponse represents the standard API response for elevation requests.
// Contains workflow ID for tracking, current status phase, and optional output data.
//
// Fields:
//   - WorkflowId: Unique identifier for tracking the elevation workflow
//   - Status: Current workflow phase (Pending, Running, Succeeded, Failed, etc.)
//   - Output: Optional map containing workflow results or error details
//
// Usage:
//   - API responses: Returned from /elevate endpoints
//   - CLI display: Formatted for user-friendly status output
//   - Status tracking: Used with workflow status polling
type ElevateResponse = internal.ElevateResponse

// ElevateDynamicRequest enables runtime role creation without pre-configuration.
// Supports complex nested permission structures, inheritance, and multi-provider scoping.
// Can be submitted via both JSON and HTML form data with bracket notation support.
//
// Key features:
//   - Dynamic role composition with permissions, groups, and resources
//   - Nested scope structure for granular access control
//   - Provider inheritance and multi-tenant support
//   - Form binding with bracket notation (scopes[groups], scopes[users])
//   - Conversion to standard ElevateRequest via internal processing
//
// Usage:
//   - Web forms: Enhanced role creation interface
//   - API: Complex elevation scenarios requiring custom roles
//   - Multi-provider: Scenarios spanning multiple cloud providers
type ElevateDynamicRequest = internal.ElevateDynamicRequest

// ElevateLLMRequest enables AI-assisted elevation via natural language processing.
// Transforms human-readable access requests into structured ElevateRequest objects.
// Provides the simplest interface requiring only a reason field.
//
// AI Processing:
//   - Parses natural language to extract role, provider, and scope requirements
//   - Generates complete ElevateRequest with appropriate permissions
//   - Handles complex scenarios like "I need S3 access for the staging environment"
//
// Usage:
//   - CLI: Interactive access request with 'reason' parameter
//   - Web interface: User-friendly access request forms
//   - API: /elevate/llm endpoint for intelligent request processing
type ElevateLLMRequest = internal.ElevateLLMRequest

// ElevateDynamicRequestScopes defines the nested scope structure for dynamic elevation requests.
// Enables granular control over which users, groups, and domains can receive the dynamically created role.
// Supports both JSON object notation and HTML form bracket notation for web form compatibility.
//
// Fields:
//   - Groups: List of group names/IDs that can receive this role
//   - Users: List of specific user identifiers for role assignment
//   - Domains: List of domain patterns for broad user matching
//
// Form binding examples:
//   - scopes[groups][]=admins&scopes[users][]=john.doe
//   - JSON: {"scopes": {"groups": ["admins"], "users": ["john.doe"]}}
type ElevateDynamicRequestScopes = internal.ElevateDynamicRequestScopes

// ElevateStaticRequest handles simple form-based elevation using predefined roles.
// Designed for GET request query parameters and basic form submissions.
// Includes session management and URL parameter serialization.
//
// Key methods:
//   - GetUrlParams(): Serializes request fields into URL query parameters
//   - GetEncodedSession(): Returns encrypted session token for security
//   - GetSession(): Provides access to the underlying session object
//
// Usage:
//   - Query parameters: GET /elevate?role=admin&provider=aws&reason=maintenance
//   - Simple forms: Basic HTML form submissions without complex nesting
//   - URL generation: Creating shareable elevation request links
//   - Session handling: Manages encoded authentication tokens
type ElevateStaticRequest = internal.ElevateStaticRequest
