> Generated from:
> `.agent-shared/skills/verify-change.md`
> Edit the shared source and run `scripts/sync-agent-skills.sh`.

# Verify Change

Use this skill after making a change, or earlier when you need to choose the right verification
scope for the affected subsystem.

## Required Inputs

- Changed paths or packages
- Whether Docker, Chromium, Ruby/Bundler, `swag`, and `flatc` are available
- Whether the change is internal-only or user-facing

## Steps

1. Prefer TDD-style verification when practical: add or update a failing test first, confirm it
   fails on the pre-change behavior, then implement the fix and rerun the targeted tests to confirm
   they pass.
2. Run `gofmt -w` on every changed Go file.
3. Run root-module tests for code in the main module. Start with the narrowest useful package
   tests; use `go test ./...` when the change crosses packages or you need broader confidence.
4. If the change affects behavior covered by the nested `test/` module, run the matching suite:
   - Functional provider behavior: `cd test && go test -v -timeout 10m ./functional/...`
   - Services/Temporal wiring: `cd test && go test -v -timeout 5m ./integration/services/...`
   - Workflow behavior: `cd test && go test -v -timeout 15m ./integration/workflows/...`
   - Frontend/browser flow: `make build-linux-amd64` then `cd test && go test -v -timeout 30m
     ./integration/frontend/...`
5. If the change touches Go Workflow Definitions or message handlers, use
   `temporal-development` to scope determinism checks, including replay testing and
   `workflowcheck` when practical.
6. If the change affects docs-site content or navigation, run `cd docs && bundle exec jekyll
   build`.
7. If the change affects API annotations or generated Swagger assets, run `make swagger`.
8. If the change affects FlatBuffers schemas or IAM dataset generation, run `make generate-data`.
9. Record every command you ran and call out anything you intentionally skipped.

## Completion Criteria

- Changed Go files are formatted.
- Relevant tests/builds have been run, or skipped with a concrete reason.
- Temporal determinism checks from `temporal-development` were included for Workflow-definition
  changes, or explicitly deferred.
- Any required generated artifacts have been refreshed.

