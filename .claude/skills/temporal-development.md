> Generated from:
> `.agent-shared/skills/temporal-development.md`
> Edit the shared source and run `scripts/sync-agent-skills.sh`.

# Temporal Development

Use this skill when changing Temporal Workflow Definitions, Activities, local activities, workers,
message handlers, retries, versioning, or replay-sensitive verification in this repo's Go code.

## Required Inputs

- Changed workflow, activity, worker, or config paths
- Whether the change touches Workflow Definitions, Activities, local activities, Signals, Queries,
  Updates, `Continue-As-New`, Search Attributes, or worker registration
- Whether the change may replay on live executions or only affect new runs
- Whether `workflowcheck`, representative histories, and the relevant Temporal test environment are
  available

## Steps

1. Identify the owning workflow surface and registration path before editing:
   - `internal/workflows/`
   - `internal/config/workflows.go`
   - `internal/config/temporal*.go`
   - `sdk/workflows/`
   - the affected task queue, worker registration, and Activity registration
2. Keep Workflow Definitions deterministic:
   - For the same Event History, workflow code must emit the same Commands in the same order when
     it replays.
   - Do not branch on wall-clock time, randomness, process-local state, env var reads, network
     results, or unordered map iteration in workflow code.
   - In Go workflow code, use replay-safe Temporal APIs such as `workflow.Go`,
     `workflow.Channel`, `workflow.Selector`, `workflow.Now`, `workflow.Sleep`, and
     `workflow.GetLogger` instead of native goroutines, channels, `select`, `time.Now`,
     `time.Sleep`, or standard blocking primitives.
3. Keep side effects out of workflow code:
   - Do not perform direct network, filesystem, database, subprocess, or provider SDK I/O in
     Workflow Definitions.
   - Put external I/O, mutable side effects, and fallible business logic in Activities.
   - Do not hide side effects behind helper functions called from workflow code.
4. Design Activities for retries:
   - Make Activities idempotent when they can retry. Use idempotency keys or duplicate detection
     when the external system supports them.
   - If an Activity cannot be retried safely, make that explicit in retry policy or error
     handling.
   - Split multi-step side effects at retry-safe boundaries so retries do not repeat completed
     external effects blindly.
   - For long-running work, heartbeat and persist enough progress or checkpoint detail to resume or
     fail cleanly.
5. Use local activities sparingly and with the same retry-safety discipline:
   - Keep them short, low-latency, and free of long blocking calls.
   - Treat them as worker-local helpers, not a place to hide slow or durable external
     orchestration.
   - In Go, prefer stable named functions over anonymous local-activity functions when that keeps
     registration, histories, and diagnostics clearer.
6. Use `SideEffect` and `MutableSideEffect` only for replay-safe value capture:
   - The callback should return a value and must not mutate Workflow state.
   - Do not put business logic, blocking work, or external I/O there.
   - Prefer Activities when the value comes from outside the workflow process.
7. Keep message handlers safe:
   - Query handlers are read-only and must not mutate state or run async work.
   - Signal and Update handlers should stay lightweight, coordinate with main-workflow state, and
     avoid starting work the main workflow cannot account for safely.
   - Do not call `Continue-As-New` from Signal or Update handlers.
   - Before completion or `Continue-As-New`, finish critical in-flight handlers and drain or
     account for pending Signals. Use `workflow.AllHandlersFinished` when handler completion
     is part of the workflow's safety boundary.
   - Prefer serializable request and response structs for handler inputs so interfaces can evolve
     additively.
8. Manage history and long-running executions deliberately:
   - Watch Event History growth for loops, polling patterns, and message-heavy workflows.
   - Use `Continue-As-New` before history becomes operationally risky.
   - Carry forward enough workflow state in the next run input to resume safely.
9. Treat workflow code changes as compatibility-sensitive:
   - Reordering, adding, or removing command-producing calls for live executions needs Worker
     Versioning or patching.
   - This includes timers, Activities, local activities, Child Workflows, external signals,
     `SideEffect` or `MutableSideEffect`, Search Attribute or Memo upserts, and
     `Continue-As-New`.
   - Prefer Worker Versioning for incompatible changes. Use patching when you must bridge old and
     new histories in code.
   - Plan rollout around running executions, not just fresh starts.
10. Verify Temporal changes with replay in mind:
   - Add or update the nearest unit or integration tests for the changed workflow or activity
     surface.
   - For Go Workflow Definition or message-handler changes, run `workflowcheck` when available
     through repo-local or ephemeral tooling.
   - Run replay tests against representative histories when practical, especially before shipping
     changes that may replay on existing executions.
   - If verification or rollout safety depends on external Temporal state you could not inspect,
     say so plainly.

## Anti-Patterns To Reject

- direct I/O or side effects in Workflow Definitions
- native Go goroutines, channels, `select`, `time.Now`, `time.Sleep`, or unordered map iteration
  in workflow code
- workflow branches on process-local or wall-clock state that is not recorded in history
- mutating Workflow state inside `SideEffect` or `MutableSideEffect`
- non-idempotent Activities that may grant, revoke, provision, charge, or notify twice on retry
- long or unreliable work hidden in local activities
- `Continue-As-New` or completion while important Signal or Update handler work is still in flight
- incompatible command-sequence changes without Worker Versioning or patching
- shipping workflow-definition changes based only on happy-path unit tests when replay validation
  is feasible

## Completion Criteria

- Workflow code remains deterministic and side-effect free.
- Activity boundaries and retry policy are explicit and safe for the external systems involved.
- Message-handler, `Continue-As-New`, and versioning behavior are reviewed when applicable.
- Replay-oriented verification and rollout risks are documented, run, or explicitly deferred.

