> Generated from:
> `.agent-shared/skills/repo-orientation.md`
> Edit the shared source and run `scripts/sync-agent-skills.sh`.

# Repo Orientation

Use this skill when you are entering the repo for a new task, the request spans multiple packages, or you need to map a change to the owning subsystem before editing.

## Required Inputs

- The task statement
- Any known file paths or package names

## Steps

1. Read `README.md`, `Makefile`, and `config.example.yaml`.
2. Map the request to the main surfaces:
   - CLI and startup flow: `main.go`, `cmd/cli/`
   - Server/API: `internal/daemon/`, `internal/api/`
   - Config/bootstrap: `internal/config/`
   - Providers and RBAC: `internal/providers/<provider>/`
   - Workflow engine and DSL: `internal/workflows/`, `sdk/workflows/`, `examples/workflows/`
   - Public examples/docs: `examples/`, `docs/`
   - Heavy integration coverage: nested `test/` module
3. Check `.github/workflows/test-and-build.yml` so your verification plan matches CI instead of a guessed workflow.
4. Decide whether the task touches a high-risk surface: auth, sessions, provider authorization/revocation, Temporal wiring, config/env loading, updater/service install, or public API/CLI behavior.
5. Write down the smallest set of packages, docs, and tests that should move together before you edit anything.

## Completion Criteria

- You can name the likely owning package(s).
- You can name the expected docs/examples/config files, if any.
- You can hand off to the `verify-change`, `provider-or-workflow-change`, `public-surface-change`, or `risky-access-change-review` skill as appropriate.

