# Verify Change

Use this skill after making a change, or earlier when you need to choose the right verification scope for the affected subsystem.

## Required Inputs

- Changed paths or packages
- Whether Docker, Chromium, Ruby/Bundler, `swag`, and `flatc` are available
- Whether the change is internal-only or user-facing

## Steps

1. Run `gofmt -w` on every changed Go file.
2. Run root-module tests for code in the main module. Start with the narrowest useful package tests; use `go test ./...` when the change crosses packages or you need broader confidence.
3. If the change affects behavior covered by the nested `test/` module, run the matching suite:
   - Functional provider behavior: `cd test && go test -v -timeout 10m ./functional/...`
   - Services/Temporal wiring: `cd test && go test -v -timeout 5m ./integration/services/...`
   - Workflow behavior: `cd test && go test -v -timeout 15m ./integration/workflows/...`
   - Frontend/browser flow: `make build-linux-amd64` then `cd test && go test -v -timeout 30m ./integration/frontend/...`
4. If the change affects docs-site content or navigation, run `cd docs && bundle exec jekyll build`.
5. If the change affects API annotations or generated Swagger assets, run `make swagger`.
6. If the change affects FlatBuffers schemas or IAM dataset generation, run `make generate-data`.
7. Record every command you ran and call out anything you intentionally skipped.

## Completion Criteria

- Changed Go files are formatted.
- Relevant tests/builds have been run, or skipped with a concrete reason.
- Any required generated artifacts have been refreshed.
