# Side-By-Side Demo Prompts

Use this to compare two Claude Code sessions:

- `job-demo-1`: Claude Code with `coremem` MCP and hooks enabled.
- `job-demo-2`: Claude Code without `coremem`.

Run the same implementation prompts in both folders. The expected difference is that `job-demo-1` retrieves durable repo/user memory and avoids known mistakes, while `job-demo-2` has only the prompt text and visible files.

## Before You Start

Build `coremem`:

```sh
cd /Users/arjun-etpl/ennoventure/repos/coremem
go build -o bin/coremem ./cmd/coremem
./bin/coremem migrate
```

Seed memory for `job-demo-1` specifically:

```sh
cd /path/to/job-demo-1
/Users/arjun-etpl/ennoventure/repos/coremem/bin/coremem demo
```

Start the hook server:

```sh
/Users/arjun-etpl/ennoventure/repos/coremem/bin/coremem http --addr 127.0.0.1:8765
```

Add MCP only to the Claude Code session/config used by `job-demo-1`.

## Optional Starter Repo Prompt

Run this once in both folders if they are empty:

```text
Create a minimal Go service for async job submission and processing. Keep it simple: a Job type, an in-memory repository if needed for the initial skeleton, a submit path, a worker path, and tests. Do not ask follow-up questions.
```

This starter prompt intentionally allows a weak baseline. The later prompts should show whether memory changes the design.

## Prompt 1: Negative Memory And Repo Constraints

Run in both folders:

```text
Implement async job submission and processing for this service.

Requirements:
- A caller can submit a job with a payload.
- A worker can claim pending work.
- A worker can mark a job succeeded or failed.
- Add tests for the main behavior.

Choose the simplest production-shaped design for this repo.
```

Expected difference:

- `job-demo-1`: avoids in-memory queues, uses persistent job state/table-like repository semantics, mentions retries/idempotency, avoids new dependencies.
- `job-demo-2`: may use channels, goroutines, package-level queues, or add unnecessary dependencies.

## Prompt 2: Idempotency

Run in both folders:

```text
Add retry support for failed jobs.

Requirements:
- Failed jobs can be retried.
- Retrying the same job more than once should not corrupt state.
- Add tests for duplicate retry attempts.
```

Expected difference:

- `job-demo-1`: explicitly designs for idempotent handlers and duplicate execution.
- `job-demo-2`: may only reset status or enqueue again without duplicate-safety.

## Prompt 3: User Coding Preference

Run in both folders:

```text
Add CSV import support for bulk job submission with validation and tests.

Requirements:
- Parse CSV rows into job submission payloads.
- Reject invalid rows with useful errors.
- Add tests for valid rows, invalid rows, and mixed input.
```

Expected difference:

- `job-demo-1`: should follow Arjun's style: explicit error handling, small functions, simple interfaces, table-driven tests, no clever abstractions.
- `job-demo-2`: style is unconstrained and may drift.

## Prompt 4: Explicit User-Tagged Memory

Run this only in `job-demo-1`:

```text
[coremem:type=core_constraint scope=repo title="Audit job transitions"]
Every job status transition must record who or what triggered it, because production debugging depends on transition history.
[/coremem]

Please remember this durable repo constraint. Do not implement anything yet.
```

Then run this in both folders:

```text
Add job cancellation.

Requirements:
- Pending jobs can be cancelled.
- Running jobs cannot be cancelled directly.
- Add tests for cancellation behavior.
```

Expected difference:

- `job-demo-1`: should include transition/audit metadata or at least preserve a design point for transition history.
- `job-demo-2`: likely implements only status changes.

## Prompt 5: Superseded Memory

Run this only in `job-demo-1`:

```text
Use the coremem_supersede tool to supersede the memory titled "No new dependencies".

New memory:
- type: core_negative
- title: "No runtime dependencies"
- body: "Do not add runtime dependencies unless explicitly approved. Test-only standard tooling is fine, but prefer the Go standard library."
- reason: "Clarifies that the restriction is about runtime dependencies."
```

Then run this in both folders:

```text
Add JSON export for job history.

Requirements:
- Export completed and failed jobs as JSON.
- Include job ID, status, payload, attempts, and timestamps.
- Add tests.

Use any library you think is appropriate.
```

Expected difference:

- `job-demo-1`: should avoid runtime dependencies and use the standard library.
- `job-demo-2`: may choose or suggest third-party JSON/helper packages.

## Prompt 6: Repo-Aware Retrieval

Run this in both folders:

```text
Review the current job processing design and list the top three risks before making changes. Then fix the highest-risk issue with tests.
```

Expected difference:

- `job-demo-1`: risk list should reflect seeded repo memory: in-memory queues, idempotency, dependency discipline, durable job state.
- `job-demo-2`: risk list depends only on visible code and may miss durable project constraints.

## Prompt 7: Agent Result Memory

Run this only in `job-demo-1` after a successful implementation:

```text
Use coremem_add to save an agent_result memory for this repo.

Title: "Job processing implementation completed"
Body: "The service now uses persistent job state semantics, idempotent retry behavior, cancellation rules, and table-driven tests for core job flows."
Tags: ["jobs", "implementation", "tests"]
Importance: 0.7
```

Then start a fresh Claude Code session in `job-demo-1` and ask:

```text
What durable project context should I know before changing job processing again?
```

Expected result:

- `job-demo-1`: retrieves prior implementation result and core constraints from SQLite.
- `job-demo-2`: has no durable memory unless the context is still visible in the conversation.

## Scoring Sheet

Use this quick scoring for each implementation prompt:

| Check | job-demo-1 | job-demo-2 |
| --- | --- | --- |
| Avoids in-memory queues | 0/1 | 0/1 |
| Uses persistent job state | 0/1 | 0/1 |
| Handles idempotency/retries | 0/1 | 0/1 |
| Avoids unapproved runtime deps | 0/1 | 0/1 |
| Follows Arjun style preference | 0/1 | 0/1 |
| Applies newly tagged memory | 0/1 | 0/1 |
| Excludes superseded memory | 0/1 | 0/1 |
| Retrieves prior agent result in fresh session | 0/1 | 0/1 |

The clearest demo is usually prompts 1, 3, 4, and 7.
