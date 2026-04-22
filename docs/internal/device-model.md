---
layout: default
title: Device Model
parent: Internal
nav_order: 1
---

# Device Model

This document describes the long-term device architecture for thand, the first phase now implemented in code, and the work intentionally deferred.

## Why Devices Exist

Local execution targets such as laptops, desktops, and servers have different lifecycle and security requirements than:

- users, which represent human principals
- providers, which represent integrations such as AWS or JumpCloud
- tenants, which represent provider-scoped accounts or resource containers

A device is therefore modeled as a first-class server-managed object. Devices let the server answer questions like:

- which machine should receive a device-local workflow
- which device-local policy applies to that machine
- whether the machine is currently connected and routable

Operators can print the local machine's current device ID with `thand config device-id`.
Non-production builds may override the generated value for deterministic testing, but production binaries always use the generated machine-derived `device_id`.

## What A Device Is

In the current model, a device has:

- a stable device ID
- human-readable metadata such as `name` and `description`

Runtime connection state is tracked separately from static device policy. That runtime state currently includes:

- `task_queue`
- `last_seen_at`
- derived freshness / connected status

## Why Devices Are Not Tenants

Provider tenants and devices look superficially similar because both can affect routing and authorization scope, but they solve different problems.

Tenants are provider-scoped. A tenant says which account or org inside a provider a request applies to. Devices are execution-scoped. A device says which machine should run a workflow or local action.

Using tenants for devices would overload provider semantics with machine lifecycle concerns such as:

- agent registration
- live route freshness
- local reconciliation after reconnect
- future privileged-helper transport

That coupling would make both models harder to reason about, so devices remain separate.

## Target Architecture

The intended architecture is:

1. The server owns device definitions and device policy.
2. An agent represents one device, running as a system-level service rather than a per-user helper.
3. `/register` bootstraps config only; running agents publish live route state directly to Temporal.
4. Device-targeted workflows route through that live route only.

Today the canonical `device_id` is machine-derived. Longer term, device registration should use a stronger enrolled identity, but keep the same `device_id` abstraction boundary.

## Phase 1: What Is Implemented Now

Phase 1 establishes the basic device substrate without yet solving strong device identity.

Implemented now:

- first-class `Device` definitions in config
- live device connection state tracked in the shared Temporal device-route registry
- shared device definitions tracked in a Temporal device-definition registry
- periodic device registration refresh from the agent
- route freshness checks using `last_seen_at`
- device-targeted provider child workflows using a fresh live route
- bounded waiting for authorize when a device is temporarily offline
- retrying revoke reconciliation when the device is offline

Not yet implemented:

- cryptographic device enrollment or proof-of-possession identity behind the existing `device_id` abstraction
- a dedicated control-plane service for device config
- device discovery UI or richer device selection UX
- a privileged local helper split from the main agent

Phase 1 routes only from fresh live registration state.

## Current Routing Model

Today, routing works like this:

1. An agent registers with the server.
2. The server returns bootstrap/config data to the agent.
3. The agent publishes its current `task_queue` and `last_seen_at` to the shared Temporal device-route registry on `thand_device_registry`.
4. Servers publish configured device definitions to the shared Temporal device-definition registry on `thand_device_registry`.
5. Device-targeted workflows query shared device policy during execution planning and ask for a fresh route before dispatch.
6. If the route is missing or stale, authorize waits for a bounded window and revoke keeps retrying for reconciliation.

This gives us a cleaner failure model:

- authorize should not succeed much later than requested
- revoke should converge once the device reconnects
- timed local enforcement should not depend solely on centralized connectivity

## Consequences of the Current Design

The current phase-1 design has a few important consequences:

- devices are now a generic execution substrate, not a sudo-only feature
- routing depends on liveness, not static config
- agents are treated as per-device services, not per-user services
- the machine-derived `device_id` is now the single routing identity used across registration, planning, and dispatch

## Known Shortcomings

The biggest gap is still device identity hardening.

Today the server matches a connecting agent to a device through the generated `device_id`. That is enough for phase 1 plumbing and local development, but it is not strong enough for a final design because it is still based on client-presented identity.

Other gaps:

- no dedicated control-plane API for device configuration
- no secure enrollment story yet
- shared device registries are still internal Temporal workflows rather than a broader device control-plane service
- no independent privileged helper transport yet on Linux or Windows
- no explicit multi-agent-per-device design, because the current assumption is one system agent per device

## Future Phases

Future work should cover at least:

### Strong device identity

- enrolled device credentials
- challenge / proof-of-possession registration
- authenticated binding between device record and live route

### Dedicated device config distribution

- server-managed per-device policy delivery
- explicit control-plane lifecycle for devices
- eventual separation between interactive user login and device bootstrap

### Privileged local helper

- OS-native trust checks between the unprivileged agent, broker, and notifier
- narrow local lease/enforcer contract with persisted expiry and restart reconciliation
- future Linux and Windows helpers that match the same broker client abstraction

### Better UX

- device discovery APIs
- device picker UX
- clearer offline / reconnect status in local-device workflows
