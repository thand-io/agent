---
layout: default
title: ADR: Device Routing Phase 1
parent: Internal
nav_order: 2
---

# ADR: Device Routing Phase 1

## Status

Accepted.

## Context

Device-local workflows need a way to target a specific machine.

At the same time, the project is not yet ready to introduce full cryptographic device identity or a dedicated device control plane.

## Decision

Phase 1 uses:

- first-class device definitions on the server
- canonical `device_id` as the device-matching key
- `/register` as bootstrap/config sync only
- live route tracking from agent-published device-route state
- shared Temporal device-definition and device-route registries on `thand_device_registry`
- periodic route refresh from running agents
- fresh-route checks for device-targeted workflow dispatch

It explicitly does not use:

- provider tenants as device identifiers
- indefinite waiting for device-targeted authorize steps

## Rationale

This choice gives us the smallest useful device substrate that:

- removes stale static routing
- supports reconnect-aware device execution
- keeps the design generic for future device-local workflows
- leaves room for a later secure identity redesign

## Alternatives Considered

### Model devices as provider tenants

Rejected because tenants are provider-scoped account concepts, not machine execution concepts.

### Require immediate online presence with no waiting

Rejected because short outages and restarts are normal. A bounded wait is a better operator experience for authorize, while revoke can retry for reconciliation.

### Solve cryptographic device identity first

Rejected for phase 1 because it would block useful device plumbing behind a larger security redesign.

## Consequences

Positive consequences:

- simpler config
- more honest routing model
- reusable device-targeted execution layer

Negative consequences:

- the current generated `device_id` is still a client-presented identity and not yet a cryptographically enrolled device credential
- device config and routing now depend on internal shared Temporal registries that need future hardening and operational polish
- later phases will need migration work for stronger enrollment
