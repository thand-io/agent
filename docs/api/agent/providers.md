---
layout: default
title: Provider Management
parent: Agent
grand_parent: API Reference
nav_order: 5
---

# Provider Management

Manage and interact with identity and cloud providers.

## List Providers

Get all available providers with optional capability filtering.

**GET** `/providers`

### Availability

- Server Mode (via `/api/v1/providers`)
- Agent Mode (via `/providers`)

### Query Parameters

- `capability` - Filter by capability (comma-separated): `authenticator`, `authorizer`, `identities`, `notifications`

### Example Usage

```bash
# Get all providers
curl http://localhost:8080/api/v1/providers

# Filter by capability
curl "http://localhost:8080/api/v1/providers?capability=authenticator,authorizer"
```

### Response

```json
{
  "version": "1.0",
  "providers": {
    "aws": {
      "name": "Amazon Web Services",
      "description": "AWS cloud provider with IAM integration",
      "provider": "aws",
      "enabled": true
    },
    "gcp": {
      "name": "Google Cloud Platform",
      "description": "GCP with IAM and identity management",
      "provider": "gcp",
      "enabled": true
    }
  }
}
```

### Notes

- Only available in server mode
- Requires authentication
- Filters providers based on user permissions
- Only returns enabled providers with initialized clients
- Supports both JSON and HTML responses

## Get Provider Details

**GET** `/provider/{provider}`

### Response

```json
{
  "name": "Amazon Web Services",
  "description": "AWS cloud provider with IAM integration",
  "provider": "aws",
  "enabled": true
}
```

## Get Provider Roles

List roles available through a provider.

**GET** `/provider/{provider}/roles`

### Query Parameters

- `q` - Filter roles by search term

### Response

```json
{
  "version": "1.0",
  "provider": "aws",
  "roles": [
    {
      "name": "ReadOnlyAccess",
      "arn": "arn:aws:iam::aws:policy/ReadOnlyAccess",
      "description": "Provides read-only access to AWS services"
    },
    {
      "name": "PowerUserAccess",
      "arn": "arn:aws:iam::aws:policy/PowerUserAccess",
      "description": "Provides full access except user management"
    }
  ]
}
```

## Get Provider Permissions

List permissions available through a provider.

**GET** `/provider/{provider}/permissions`

### Availability

- Server Mode Only

### Query Parameters

- `q` - Filter permissions by search term

### Response

```json
{
  "version": "1.0",
  "provider": "aws",
  "permissions": [
    {
      "name": "ec2:DescribeInstances",
      "description": "Grants permission to describe EC2 instances"
    },
    {
      "name": "s3:GetObject",
      "description": "Grants permission to retrieve objects from S3"
    }
  ]
}
```

## Get Provider Identities

List identities available through a provider.

**GET** `/provider/{provider}/identities`

### Availability

- Server Mode Only

### Query Parameters

- `q` - Filter identities by search term

### Response

```json
{
  "version": "1.0",
  "provider": "aws",
  "identities": [
    {
      "id": "user_123",
      "name": "Alice Smith",
      "email": "alice@example.com"
    }
  ]
}
```

## Authorize Provider Session

Initiate OAuth2 flow for a provider.

**POST** `/provider/{provider}/authorizeSession`

### Request Body

```json
{
  "scopes": ["email", "profile"],
  "state": "encoded_state_token",
  "redirect_uri": "https://localhost:8080/api/v1/auth/callback/aws"
}
```

### Response

```json
{
  "url": "https://provider.com/oauth/authorize?client_id=...&redirect_uri=...&state=...",
  "expires_in": 600
}
```
