---
layout: default
title: Creating Providers
parent: Development
nav_order: 1
description: Guide to creating and developing new providers for Thand Agent
---

# Creating a New Provider

This guide explains how to develop a new provider for the Thand Agent. Providers are modular components that implement specific capabilities like authentication, RBAC, notifications, and identity management.

## Prerequisites

- Go 1.21+
- Understanding of the Thand Agent architecture
- Access to the `internal/providers` directory

## Provider Structure

All providers must implement the `ProviderImpl` interface defined in `internal/models/provider.go`. To simplify implementation, most providers embed `*models.BaseProvider`, which provides default implementations for many methods.

### The Provider Interface

```go
type ProviderImpl interface {
    Initialize(identifier string, provider Provider) error
    
    // Base methods (handled by BaseProvider)
    GetConfig() *BasicConfig
    GetIdentifier() string
    GetName() string
    GetDescription() string
    GetProvider() string
    
    // Capability checks (handled by BaseProvider)
    GetCapabilities() *ProviderCapabilities
    HasCapability(capability ProviderCapability) bool
    
    // Sub-interfaces
    ProviderNotifier
    ProviderAuthorizor
    ProviderRoleBasedAccessControl
    ProviderIdentities
}
```

## Step-by-Step Implementation

### 1. Create the Provider Package

Create a new directory in `internal/providers/` with your provider name (e.g., `myprovider`).

```bash
mkdir internal/providers/myprovider
```

Create a `provider.go` (or `main.go`) file in that directory.

### 2. Define the Provider Struct

Define your provider struct and embed `*models.BaseProvider`.

```go
package myprovider

import (
    "context"
    "github.com/thand-io/agent/internal/models"
)

const ProviderName = "myprovider"

type MyProvider struct {
    *models.BaseProvider
}
```

### 3. Implement Initialization

Implement the `Initialize` method to set up the base provider and capabilities.

```go
func (p *MyProvider) Initialize(identifier string, provider models.Provider) error {
    // Define what this provider supports
    capabilities := &models.ProviderCapabilities{
        Roles: &models.RolesConfiguration{
            Enabled:        true,
            Synchronizable: true,
            Interval:       60, // Default sync interval in minutes
        },
        Permissions: &models.PermissionsConfiguration{
            SynchronizableConfiguration: models.SynchronizableConfiguration{
                Enabled:        true,
                Synchronizable: true,
            },
            // Set to true if the provider's API natively supports
            // wildcard permission patterns (e.g., "ec2:*").
            // When false (default), wildcards in role definitions are
            // expanded to individual permissions during validation.
            SupportsWildcards: false,
        },
        // Add other capabilities...
    }

    p.BaseProvider = models.NewBaseProvider(
        identifier,
        provider,
        capabilities,
    )

    // Parse custom configuration if needed
    // config := &MyProviderConfig{}
    // if err := provider.Config.Parse(config); err != nil { ... }

    return nil
}
```

### 4. Implement Capabilities

Implement the methods required for the capabilities you enabled.

#### Authorizer (Authentication)

If your provider supports authentication:

```go
func (p *MyProvider) AuthorizeSession(ctx context.Context, auth *models.AuthorizeUser) (*models.AuthorizeSessionResponse, error) {
    // Logic to start authentication flow
    return &models.AuthorizeSessionResponse{
        Url: "https://auth.example.com/login",
    }, nil
}

func (p *MyProvider) CreateSession(ctx context.Context, auth *models.AuthorizeUser) (*models.Session, error) {
    // Logic to create a session after successful auth
    return &models.Session{ ... }, nil
}
```

#### RBAC (Roles & Permissions)

If your provider supports RBAC:

```go
func (p *MyProvider) AuthorizeRole(ctx context.Context, req *models.AuthorizeRoleRequest) (*models.AuthorizeRoleResponse, error) {
    // Logic to grant a role to a user
    return &models.AuthorizeRoleResponse{}, nil
}

func (p *MyProvider) RevokeRole(ctx context.Context, req *models.RevokeRoleRequest) (*models.RevokeRoleResponse, error) {
    // Logic to revoke a role
    return &models.RevokeRoleResponse{}, nil
}

// Implement synchronization methods if Synchronizable is true
func (p *MyProvider) SynchronizeRoles(ctx context.Context, req *models.SynchronizeRolesRequest) (*models.SynchronizeRolesResponse, error) {
    // Logic to fetch roles from the external system
    return &models.SynchronizeRolesResponse{
        Roles: []models.ProviderRole{ ... },
    }, nil
}
```

#### Notifier

If your provider sends notifications:

```go
func (p *MyProvider) SendNotification(ctx context.Context, notification models.NotificationRequest) error {
    // Logic to send email, slack message, etc.
    return nil
}
```

### 5. Register the Provider

Add your provider to `internal/providers/registry.go`.

```go
import (
    // ...
    "github.com/thand-io/agent/internal/providers/myprovider"
)

func init() {
    // ...
    Register(myprovider.ProviderName, func() models.ProviderImpl {
        return &myprovider.MyProvider{}
    })
}
```

## Configuration

Users configure your provider in their `config.yaml`.

```yaml
providers:
  my-instance:
    name: My Provider Instance
    provider: myprovider
    enabled: true
    config:
      api_key: "secret-key"
      endpoint: "https://api.example.com"
```

Access this configuration in your `Initialize` method or other methods via `p.GetConfig()`.

## Best Practices

1.  **Use BaseProvider**: Always embed `BaseProvider` to handle default behaviors and state management.
2.  **Check Capabilities**: Before performing an action, check if the capability is enabled using `p.HasCapability()`.
3.  **Error Handling**: Return meaningful errors. Use `models.ErrNotImplemented` for optional methods you don't support.
4.  **Context**: Always respect the `context.Context` for timeouts and cancellation.
5.  **Logging**: Use the standard logger for debugging and information.

## Testing

Create unit tests for your provider logic. You can use the `internal/models/provider_test.go` utilities to help test common behaviors.
