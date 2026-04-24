---
layout: default
title: Local Sudo
parent: Agent
grand_parent: API Reference
nav_order: 5
---

# Local Sudo

Local sudo lets thand request short-lived privileged access on a specific registered device.

Use this feature when you want a normal thand approval workflow to grant temporary local administrative access on a machine that is running a thand agent.

## Request Types

Local sudo supports two modes:

- timed access, which grants sudo for a bounded lease
- command mode, which runs a specific command and cleans up immediately afterward

## Requesting Timed Local Sudo

CLI example:

```bash
thand request sudo --device 11111111-2222-3333-4444-555555555555 --duration 30m --reason "System maintenance"
```

If `--device` is omitted, the CLI defaults to the current machine's canonical `device_id`.
If `--device` is provided explicitly, the CLI uses that exact value, even if it is empty.

Static web example:

```text
/api/v1/elevate?role=local_sudo&device=11111111-2222-3333-4444-555555555555&duration=PT30M&reason=System+maintenance
```

## Requesting Command Mode

CLI example:

```bash
thand request sudo --device 11111111-2222-3333-4444-555555555555 --command softwareupdate --command -i --command -a --reason "Install updates"
```

Command mode defaults to a short duration window and removes the local grant immediately after the command finishes.

## Device Availability

If the target device is offline, local sudo does not fail immediately.

Authorize waits for a fresh device route for a bounded window:

- up to the requested sudo duration
- capped at 5 minutes

If the device does not reconnect in that window, the request fails instead of succeeding unexpectedly later.

## Workflow Behavior

Local sudo resolves device-local execution details such as the target device and local account mapping as part of the internal execution-planning work that runs at the start of `authorize`.

That planning step reads shared device policy from the Temporal-backed device-definition registry rather than depending on the handling server having the target device configured locally.

If you are authoring or reviewing workflows, see [Workflow Tasks](/configuration/workflows/tasks.html) for the `authorize` lifecycle and execution-planning behavior.

## Revoke Behavior

Timed revoke is reconciliation-oriented.

- if the device is online, revoke is dispatched immediately
- if the device is offline, revoke remains pending until the device reconnects and the server can reconcile state

Timed access is still expected to expire locally on the device based on the local lease. The pending revoke exists so the workflow can converge and leave an accurate audit trail.

## Copy / Resume URLs

The static request page preserves the `device` field in copied request URLs so the target device stays attached when the request is reopened later.

The `device` value is the canonical `device_id`. Operators can print the current machine's device ID with `thand config device-id`.

Live device routing is also keyed by that same `device_id`.

## Related Docs

- [Local Sudo Configuration](/configuration/local-sudo.html)
- [Elevation (Access Request) Endpoints](/api/agent/elevation.html)
- [Workflow Tasks](/configuration/workflows/tasks.html)
