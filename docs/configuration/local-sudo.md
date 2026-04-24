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

For full local macOS integration testing, install the Apple Development-signed privilege-services bundle with:

```bash
export APPLE_TEAM_ID=ABCDE12345
sudo -E make install-macos-privilege-services-dev
```

That installs:

- `/Applications/ThandPrivilegeServices.app`
- `/Library/Application Support/Thand/PrivilegeBroker/bin/thand-macos-privilege-brokerctl`

The installed macOS privilege-services payload is normalized to `root:wheel` ownership and non-user-writable modes during install.

If you only need unsigned layout verification and not real `SMAppService` registration, you can instead use:

```bash
THAND_MACOS_SKIP_SIGNING=1 make package-macos-privilege-services-dev
```

That unsigned mode is limited to bundle layout verification and is not a supported broker runtime path. Full local `SMAppService` and broker testing is expected to use the Apple Development-signed install flow above.

On macOS v1, timed sudo is brokered through the native privilege-services app bundle and daemon. The broker owns sudoers fragments, lease persistence, expiry, and revocation.

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

On macOS v1, command mode is intentionally disabled while the broker only supports timed sudoers grants.

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
- on macOS, revoke and expiry are enforced locally by the broker even if the agent disconnects

## Security Notes

The current generated `device_id` is enough for routing and local development, but it is not yet a cryptographically enrolled device identity.

The macOS privilege broker improves local trust boundaries, but it does not replace future enrolled device identity.

Future work is expected to add stronger enrolled device identity and a more formal device control plane.

## Related Docs

- [Local Sudo Usage](/api/agent/local-sudo.html)
- [Workflow Tasks](/configuration/workflows/tasks.html)
- [Configuration](/configuration/)
- repo developer guide: `DEVELOPMENT.md`
