---
layout: default
title: Local Sudo
parent: Configuration
nav_order: 8
---

# Local Sudo Configuration

Local sudo is configured in two parts:

- the local provider enables the device-local execution backend
- per-device policy decides who can request sudo on a given machine and which local account to use

## Required Configuration

Minimal device policy example:

```yaml
devices:
  device-alpha:
    device_id: "11111111-2222-3333-4444-555555555555"
    name: "Example Workstation"
    enabled: true
    local_elevation:
      enabled: true
      accounts:
        - email: "user@example.com"
          local_username: "localuser"
```

Production agents use a generated machine-derived `device_id`. You can print the current machine's value with:

```bash
thand config device-id
```

For deterministic dev/CI setups, non-production builds may override the generated value with `THAND_DEV_DEVICE_ID_OVERRIDE`. That override path is intentionally not available in production binaries.

## Provider Configuration

If you are using the built-in local provider defaults, you usually do not need to restate the provider stanza at all.

The embedded `local_sudo` role ships with the timed sudo workflow only. Command mode remains available to custom roles and workflows, but it is not part of the default local sudo role.

Example:

```yaml
providers:
  local-elevation:
    provider: local
    enabled: true
```

## Account Mapping

Per-device account mappings decide which local account receives sudo.

You can match by:

- `identity`
- `email`
- `username`

Example:

```yaml
devices:
  device-alpha:
    device_id: "11111111-2222-3333-4444-555555555555"
    enabled: true
    local_elevation:
      enabled: true
      accounts:
        - identity: "identity-abc123"
          local_username: "localuser"
        - email: "user@example.com"
          local_username: "localuser"
```

## Allowed Modes

You can restrict a device to specific local-sudo modes.

```yaml
devices:
  device-alpha:
    device_id: "11111111-2222-3333-4444-555555555555"
    local_elevation:
      enabled: true
      allowed_modes:
        - timed
        - command
```

If `allowed_modes` is omitted, both timed and command mode are allowed at the device-policy layer, but the embedded `local_sudo` role still exposes only the timed workflow by default.

## Guardrails

You can add guardrails for unsafe local targets.

```yaml
devices:
  device-alpha:
    device_id: "11111111-2222-3333-4444-555555555555"
    local_elevation:
      enabled: true
      denied_usernames:
        - root
        - daemon
        - nobody
      allowed_uid_ranges:
        - "1000-60000"
```

`denied_usernames` blocks sensitive local accounts even if they are mapped accidentally.

`allowed_uid_ranges` constrains requests to human-style local accounts instead of system accounts.

## Operational Notes

- local sudo routes only to fresh live agent registration state
- live routes are published by agents to the shared Temporal device-route registry
- local-sudo execution planning reads device policy from the shared Temporal device-definition registry
- the device identity is the canonical `device_id`
- operators can print the local machine device ID with `thand config device-id`
- `thand request sudo` defaults to the current machine when `--device` is omitted
- static `execution_target` routing is no longer used
- local-sudo execution planning runs internally at the start of `authorize`
- authorize waits for the device for a bounded window

## Related Docs

- [Local Sudo Usage](/api/agent/local-sudo.html)
- [Workflow Tasks](/configuration/workflows/tasks.html)
- [Configuration](/configuration/)
