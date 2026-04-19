# Risky Access Change Review

Use this skill when reviewing or implementing changes that affect security-sensitive paths in this repo.

## Required Inputs

- The diff or change summary
- Impacted files or packages
- Any known threat model, incident context, or regression concern

## Steps

1. Check whether the change touches any high-risk surface:
   - auth/login/session state
   - provider grant/revoke behavior
   - role/workflow resolution
   - Temporal workers, retries, or activity boundaries
   - updater/service install flows
   - config/env loading, secrets, or audit/logging
2. Review both the happy path and the cleanup/failure path. A change is incomplete if it grants access cleanly but leaves revocation, retry, or partial-failure behavior unclear.
3. Check for least-privilege regressions, broadened defaults, or missing validation around role/resource selection.
4. Confirm audit/logging still captures the action without leaking credentials, tokens, private keys, or sensitive provider responses.
5. Verify docs/examples/tests do not introduce secrets or normalize unsafe defaults.
6. Use `.github/PULL_REQUEST_TEMPLATE.md` as a concrete checklist for security impact, testing, provider/workflow/role changes, and documentation follow-through.
7. Run the smallest verification set that exercises both success and failure semantics, or call out what could not be validated.

## Completion Criteria

- Security impact is explicitly stated.
- Grant/revoke or enable/disable behavior has been reviewed as a pair.
- Remaining risks, skipped checks, or external dependencies are called out plainly.
