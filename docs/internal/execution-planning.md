---
layout: default
title: Execution Planning
parent: Internal
nav_order: 2
---

# Execution Planning

This document describes the internal `execution_plan` contract and the rules that `authorize` and `revoke` rely on.

## Why Execution Planning Exists

The workflow engine needs a deterministic point where a request stops being user-facing intent and becomes execution-ready work.

That planning step now happens inside a single Temporal activity invoked by `authorize`. The activity:

- reads the normalized request after validation and approvals are complete
- resolves the provider, identity, and device data needed for execution
- materializes provider-native authorization requests
- stores the resulting `execution_plan` in workflow context/history

This keeps mutable lookups out of workflow code while still letting `authorize` and `revoke` stay generic.

Device-local request shaping is handled by internal execution-plan decorators. That keeps action-specific logic, such as local sudo device-policy enrichment, together without teaching the Temporal activity about individual request types.

## What the Execution Plan Contains

The execution plan is an immutable execution snapshot for the rest of the workflow. It contains one or more canonical authorization requests, already shaped for later provider execution.

Each entry includes:

- a stable `EntryID`
- the provider name
- the canonical `device_id` used for routing
- a fully materialized provider authorization request

The plan is an internal contract. It is not a user-facing API and should not be treated as a public workflow output.

## Execution Contract

The execution contract is:

1. workflow input and approvals produce the final request intent
2. `authorize` calls the execution-plan activity once
3. that activity writes `execution_plan` into workflow context/history
4. `authorize` consumes the recorded plan
5. `revoke` later consumes the same recorded plan

`authorize` is the only task that may create the plan. `revoke` must reuse the recorded plan and fails clearly if it is missing.

## Routing Rule

Routing stays intentionally simple:

- if `DeviceID == ""`, execution stays on the parent workflow queue
- if `DeviceID != ""`, execution is device-scoped and dispatch waits for a fresh route to that device

Execution planning is responsible for deciding whether a request becomes device-scoped by setting `DeviceID` on the stored authorization request.

`authorize` does not need to know why the request is device-scoped. It only routes based on `DeviceID`.

## Ordering Requirements

Execution planning is required before any access-granting provider work starts, but it is not a user-facing workflow task anymore.

The intended ordering is:

- `validate -> authorize`
- `validate -> approvals -> authorize`

Put `authorize` after approvals, forms, or any other step that can still change the final request shape. That guarantees the execution-plan activity snapshots the final request, not an intermediate one.

## Failure Semantics and Constraints

- execution planning is the snapshot point for request shaping
- if execution planning fails, the workflow fails before access is granted
- if policy changes after the plan is recorded, the in-flight workflow keeps using the recorded snapshot
- `revoke` depends on the previously recorded request shape and does not attempt to infer it later

This separation keeps the execution model predictable and makes failure points easier to reason about.
