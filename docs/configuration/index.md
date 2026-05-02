---
layout: default
title: Configuration
nav_order: 4  
description: "Thand Agent configuration reference and examples"
has_children: true
---

# Configuration
{: .no_toc }

Complete configuration reference for Thand Agent.
{: .fs-6 .fw-300 }

## Overview

Thand Agent uses YAML configuration files to define behavior, providers, roles, and workflows. Configuration can be provided via:

Configuration is loaded in this order (later sources override earlier ones):

1. Default values
2. Configuration file (`~/.config/thand/config.yaml` by default, or the path set by `THAND_CONFIG_PATH`)
3. Environment variables (prefixed with `THAND_`)
4. Command line flags

`--config` overrides `THAND_CONFIG_PATH`. `THAND_CONFIG_PATH` selects which config file to read, while other `THAND_*` variables override values after the file is loaded.

{: .warning }
> **Important:** If you are using Temporal (recommended for production), you must configure specific Search Attributes in your Temporal Namespace. See **[Temporal Configuration](temporal.md)** for critical setup instructions.

---

## Basic Configuration Structure

```yaml
# Server connection (required)
login:
  endpoint: "https://thand.example.com"

secret: changeme

# Temporal (recommended)
services:
  temporal:
    host: "us-central1.gcp.api.temporal.io"
    port: 7233
    namespace: thand
    api_key: "changeme"


```

---

## Environment Variables

All configuration options can be set via environment variables using the `THAND_` prefix:

```bash
export THAND_LOGIN_ENDPOINT="https://thand.example.com"
export THAND_SECRET="changeme"

# Temporal (recommended)

export THAND_SERVICES_TEMPORAL_HOST="us-central1.gcp.api.temporal.io"
export THAND_SERVICES_TEMPORAL_PORT=7233
export THAND_SERVICES_TEMPORAL_NAMESPACE="thand"
export THAND_SERVICES_TEMPORAL_MTLS_PEM=""
export THAND_SERVICES_TEMPORAL_API_KEY=""
```

Nested configuration uses underscores:

```bash
export THAND_PROVIDERS_AWS_REGION="us-west-2"
```

---

## Sections

- **[Configuration Reference](file)** - Complete reference for all configuration options
- **[Environment](environment)** - Environment-specific configuration
- **[CLI](cli)** - Command-line interface configuration
- **[Providers](providers)** - Provider configurations  
- **[Roles](roles)** - Role definitions and mappings
- **[Workflows](workflows)** - Custom approval workflows
- **[Local Sudo](local-sudo)** - Device-local sudo configuration
