> Generated from:
> `.agent-shared/skills/risky-access-change-review.md`
> Edit the shared source and run `scripts/sync-agent-skills.sh`.

# Risky Access Change Review

Use this skill when reviewing or implementing changes that affect security-sensitive paths in this
repo.

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
2. Review both the happy path and the cleanup/failure path. A change is incomplete if it grants
   access cleanly but leaves revocation, retry, or partial-failure behavior unclear. For Temporal
   changes, also check replay safety, versioning or patching strategy, and whether worker crashes
   or retries can duplicate external effects; use `temporal-development` for the detailed review
   checklist.
3. Reject Temporal anti-patterns in security-sensitive paths, especially workflow-side I/O, unsafe
   retries around external effects, and incompatible workflow changes without versioning. Use
   `temporal-development` for the full workflow, activity, handler, and replay-safety guardrails.
4. Check for least-privilege regressions, broadened defaults, or missing validation around
   role/resource selection.
5. Confirm audit/logging still captures the action without leaking credentials, tokens, private
   keys, or sensitive provider responses.
6. Verify docs/examples/tests do not introduce secrets or normalize unsafe defaults.
7. Use `.github/PULL_REQUEST_TEMPLATE.md` as a concrete checklist for security impact, testing,
   provider, workflow, and role changes, and documentation follow-through.
8. Run the smallest verification set that exercises both success and failure semantics, or call out
   what could not be validated. For Go Workflow changes, prefer replay testing and `workflowcheck`
   per `temporal-development` instead of relying only on happy-path unit tests.

## Completion Criteria

- Security impact is explicitly stated.
- Grant/revoke or enable/disable behavior has been reviewed as a pair.
- Remaining risks, skipped checks, or external dependencies are called out plainly.

