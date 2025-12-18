---
layout: default
title: Configuration
parent: Agent
grand_parent: API Reference
nav_order: 11
---

# Configuration Endpoints

Agent registration and configuration management.

## API Configuration

Get service configuration including endpoints, capabilities, and authentication methods.

**GET** `/.well-known/api-configuration`

### Availability

- Server Mode
- Agent Mode
- Client Mode

### Response

```json
{
  "serviceName": "Thand Agent",
  "serviceType": "agent",
  "version": "1.0.0",
  "baseUrl": "http://localhost:8080",
  "apiBasePath": "/api/v1",
  "authEndpoint": "http://localhost:8080/auth",
  "authMethods": ["session", "bearer"],
  "docsUrl": "http://localhost:8080/swagger/index.html",
  "openApiSpec": "http://localhost:8080/swagger/doc.json",
  "capabilities": {
    "temporal": false,
    "vault": false
  }
}
```

## Pre-flight Check

Validate configuration before registration.

**POST** `/preflight`

### Availability

- Server Mode Only

Currently a stub endpoint for future pre-flight validation.

## Register Agent

Register an agent with the server.

**POST** `/register`

### Availability

- Server Mode Only

### Request Body

```json
{
  "environment": {
    "name": "production",
    "description": "Production environment configuration"
  }
}
```

### Response

```json
{
  "success": true,
  "services": {
    "temporal": {
      "enabled": true,
      "namespace": "thand",
      "task_queue": "thand-task-queue"
    },
    "llm": {
      "enabled": true,
      "provider": "openai"
    }
  },
  "roles": {
    "definitions": {
      "admin": {
        "name": "admin",
        "description": "Administrator role",
        "providers": ["aws", "gcp"]
      }
    }
  },
  "providers": {
    "definitions": {
      "aws": {
        "name": "AWS",
        "provider": "aws"
      }
    }
  },
  "workflows": {
    "definitions": {
      "approval": {
        "name": "approval",
        "steps": []
      }
    }
  }
}
```

### Configuration Sync

The registration response contains the complete configuration for the agent, including:

- **Roles**: Access control definitions and permissions.
- **Providers**: Configuration for upstream identity and cloud providers.
- **Workflows**: Approval and automation workflow definitions.

If the upstream server has a newer version of the configuration, the agent will update its local configuration to match the server's state. This ensures that policies and configurations are consistent across the infrastructure.

## Post-flight Check

Validate configuration after registration.

**POST** `/postflight`

### Availability

- Server Mode Only

Currently a stub endpoint for future post-flight validation.

## Sync Configuration

Get current configuration state.

**GET** `/sync`

### Availability

- Server Mode Only

### Response

```json
{
  "version": "1.0.0 (git: abc123)",
  "timestamp": "2024-01-15T10:30:00Z"
}
```
