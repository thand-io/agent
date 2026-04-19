> Generated from `.agent-shared/skills/public-surface-change.md`. Edit the shared source and run `scripts/sync-agent-skills.sh`.

# Public Surface Change

Use this skill when a change alters user-facing CLI behavior, HTTP/API behavior, config shape, examples, docs, or release/build semantics.

## Required Inputs

- The user-facing surface that changed
- The implementation files that drove the change

## Steps

1. Identify the public surface:
   - CLI: `cmd/cli/`, `cmd/cli/README.md`, `README.md`, `docs/configuration/cli.md`
   - API/server: `internal/daemon/`, `internal/api/`, `docs/api/`, `docs/swagger.json`, `docs/swagger.yaml`, `docs/docs.go`
   - Config/examples: `config.example.yaml`, `examples/`, `docs/configuration/`
   - Release/build/docs tooling: `Makefile`, `Dockerfile*`, `.github/workflows/`, `RELEASE.md`, `docs/`
2. Update the code and the user-facing description in the same change whenever possible.
3. If API annotations changed, regenerate Swagger assets with `make swagger`.
4. If config keys, workflow examples, or provider examples changed, update `config.example.yaml`, the matching `examples/` file, and the relevant docs page together.
5. If docs-site content changed, run `cd docs && bundle exec jekyll build`.
6. If release or CI semantics changed, verify the impacted workflow file and `RELEASE.md` stay aligned.

## Completion Criteria

- Code and user-facing docs/examples/config describe the same behavior.
- Generated docs artifacts are refreshed when needed.
- Verification covers both the implementation and the public surface.

