> Generated from:
> `.agent-shared/skills/provider-or-workflow-change.md`
> Edit the shared source and run `scripts/sync-agent-skills.sh`.

# Provider Or Workflow Change

Use this skill when changing provider integrations, role resolution, workflow definitions, or the
wiring that connects those pieces.

## Required Inputs

- Provider name, role name, or workflow name
- Intended behavior change
- Whether the change affects authorization, revocation, session handling, or examples/docs

## Steps

1. Identify the owning package and follow the existing pattern before adding new files:
   - Provider packages usually split logic across `main.go`, `capabilities.go`, `schema.go`,
     RBAC/auth/session helpers, and tests.
   - Workflow-related behavior is usually wired through `internal/workflows/`,
     `internal/config/workflows.go`, and `internal/config/temporal*.go`.
2. Trace registration and config loading, not just the leaf implementation:
   - `internal/providers/registry.go`
   - `internal/config/providers.go`
   - `internal/config/roles.go`
   - `internal/config/workflows.go`
   - `config.example.yaml`, `examples/providers/`, `examples/roles/`, `examples/workflows/`
3. If the change touches Temporal Workflow Definitions, Activities, local activities, message
   handlers, retries, or versioning, use the `temporal-development` skill for the detailed
   determinism, handler-safety, and compatibility checklist.
4. Keep grant and revoke behavior symmetric and review idempotency/retry behavior around Temporal
   activity boundaries.
5. Add or update tests in the nearest package, then choose the matching nested `test/` suite if
   runtime behavior changed. For Go Workflow Definition changes, include replay testing and
   `workflowcheck` per `temporal-development` when practical.
6. Update docs/examples whenever a user would need to configure or invoke the new behavior
   differently.
7. If the task touches the workflow DSL or runtime semantics, inspect `internal/workflows/DSL.md`
   and `sdk/workflows/` before finalizing.

## Completion Criteria

- Registration, config, examples, and docs are consistent with the implementation.
- Grant/revoke or start/stop behavior is covered, not just creation.
- Appropriate package and integration tests are updated or explicitly deferred.

