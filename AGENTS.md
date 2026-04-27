# Thand Agent Repository Guide

This repository contains the Thand CLI, local agent, and server runtime for just-in-time access
management. It is a Go codebase with a separate nested `test/` module for heavier functional and
integration suites, plus a Jekyll docs site under `docs/`.

## Architecture Snapshot

- Entry point: `main.go` -> `cmd/cli/`
- CLI commands and local agent startup: `cmd/cli/`
- Server and HTTP API handling: `internal/daemon/`, `internal/api/`
- Config loading, environment wiring, provider/role/workflow registration: `internal/config/`
- Provider integrations and RBAC logic: `internal/providers/<provider>/`
- Workflow orchestration and DSL/runtime helpers: `internal/workflows/`, `sdk/workflows/`
- Shared SDK types and constants: `sdk/`
- Public examples and default config: `config.example.yaml`, `examples/`
- Heavy test suites: `test/functional/`, `test/integration/`
- Public docs and API docs: `docs/`

## First Pass In A Task

Start by reading the files that define the repo's behavior instead of guessing:

1. `README.md`
2. `Makefile`
3. `config.example.yaml`
4. `.github/workflows/test-and-build.yml`
5. The nearest package README or docs page for the subsystem you are touching

If the task is not obviously local to one package, use the `repo-orientation` skill before editing.

## Working Norms

- Keep changes architecture-aware: config loading belongs in `internal/config`, provider behavior
  in `internal/providers/<provider>`, workflow runtime behavior in `internal/workflows`, and
  user-facing command behavior in `cmd/cli`.
- Preserve the existing provider pattern when adding or extending a provider:
  capability/schema/registration plus RBAC, session, and test coverage where applicable.
- Treat `examples/`, `config.example.yaml`, and `docs/` as part of the product. Keep them in sync
  with user-facing behavior.
- `third_party/iam-dataset` is a submodule used by data-generation flows. Do not update submodules
  unless the task actually requires it.
- Prefer small, reviewable changes. Split release/versioning work from feature work.

## Environment And Safety

- Prefer TDD-style verification for code changes when practical: add or update a failing test
  first, confirm it fails on the pre-change behavior, then implement the fix and rerun the targeted
  tests to confirm they pass.
- Do not install project-specific tools globally on the host.
- Prefer the documented workspace scripts and wiring in `docs/development/index.md` and the repo's
  documented commands in `Makefile`, `README.md`, `docs/README.md`, and `RELEASE.md` over ad hoc
  environment setup.
- Keep generated environment files out of tracked repos unless they are intentional workspace
  assets.
- Prefer symlinks, mounts and editable installs over copying code between repos.

## Path Safety

- Never commit absolute paths that point into a user's home directory or other non-standard
  machine-local paths.
- For committed file references, prefer repo-relative paths whenever possible.

## Temporal Development

- Use the `temporal-development` skill for Temporal Workflow Definitions, Activities, local
  activities, message handlers, versioning, and replay-sensitive changes.
- Keep Workflow Definitions deterministic and side-effect free, keep external I/O in Activities,
  and treat replay compatibility as a rollout concern instead of a local refactor.
- For security-sensitive or privileged-access workflow changes, use
  `risky-access-change-review` alongside `temporal-development`.

## Verification Expectations

There is no dedicated repo lint target today. Default to `gofmt` plus the smallest relevant test
commands that match CI behavior.

- Format changed Go files with `gofmt -w`.
- Root Go module verification: `go test ./...`
- Functional tests: `cd test && go test -v -timeout 10m ./functional/...`
- Services integration: `cd test && go test -v -timeout 5m ./integration/services/...`
- Workflow integration: `cd test && go test -v -timeout 15m ./integration/workflows/...`
- Frontend E2E: `make build-linux-amd64` then `cd test && go test -v -timeout 30m
  ./integration/frontend/...`
- Docs site: `cd docs && bundle exec jekyll build`
- Swagger/API artifacts after API annotation changes: `make swagger`
- FlatBuffers/IAM dataset regeneration only when schema or generator inputs change: `make
  generate-data`

Notes:

- CI uses Go `1.26` and `GOEXPERIMENT=jsonv2`; match that when reproducing build or release
  behavior.
- Frontend, functional, and many integration tests depend on Docker/testcontainers. Frontend E2E
  also expects Chromium on Linux in CI.
- The `test/` directory is its own Go module. Root `go test ./...` does not replace the nested
  `test/` suites.
- If a change touches Temporal Workflow Definitions or message handlers, add replay testing and
  `workflowcheck` to the verification plan when practical, and say plainly if either was skipped.

## Risk And Review

This repo handles privileged access. Treat the following as high-risk surfaces:

- auth, login, and session state
- provider grant/revoke logic
- role/workflow resolution
- Temporal configuration, worker registration, and retry/idempotency boundaries
- service install/update flows
- config loading, env var overrides, and secret handling
- public API or CLI behavior

For high-risk changes:

- review both grant and revoke paths, not just the happy path
- check least-privilege impact and audit/logging coverage
- avoid leaking credentials or sensitive data in logs, examples, docs, or tests
- verify failure and retry behavior, especially around Temporal activities/workflows
- use `.github/PULL_REQUEST_TEMPLATE.md` as a practical review checklist

If the task is a review or security-sensitive implementation, use the `risky-access-change-review`
skill.

## Definition Of Done

A normal change is done when:

1. The code and nearby tests are updated coherently.
2. Relevant examples, docs, config, or generated artifacts are updated when public behavior
   changed.
3. The smallest meaningful verification commands have been run.
4. Any skipped verification is called out explicitly.
5. No debug-only code, secrets, or temporary workarounds remain.

## Repo-Local Skills

Use skills for multi-step procedures instead of expanding this file:

- `repo-orientation`
- `verify-change`
- `provider-or-workflow-change`
- `public-surface-change`
- `risky-access-change-review`
- `temporal-development`

Canonical skill sources live in `.agent-shared/skills/`. If you update shared skill text, run
`scripts/sync-agent-skills.sh` to refresh the generated Codex and Claude copies.
