# Benchmark And Demo

The benchmark goal is to show that Claude Code with `coremem` avoids mistakes caused by missing project or user memory.

## Seed Data

Run:

```sh
go build -o bin/coremem ./cmd/coremem
./bin/coremem migrate
./bin/coremem demo
```

`coremem demo` seeds the current repo with:

1. `core_constraint`, repo: **No in-memory queues**  
   Do not use in-memory queues for job processing because workers run across multiple pods. Use persistent job state instead.

2. `core_constraint`, repo: **Idempotent handlers**  
   Job handlers must be idempotent because retries can duplicate execution.

3. `core_negative`, repo: **No new dependencies**  
   Do not add external dependencies unless explicitly approved.

4. `core_preference`, user `arjun`: **Arjun Go style**  
   Prefer explicit error handling, small functions, simple interfaces, and table-driven tests. Avoid clever abstractions.

5. `core_preference`, user `meena`: **Meena Go style**  
   Prefer composable helpers, reusable validation functions, and concise service-layer abstractions.

## Sanity Checks

```sh
./bin/coremem search queue --user-id arjun --repo-path .
```

Expected: includes `No in-memory queues`.

```sh
./bin/coremem context \
  --prompt "Implement async job processing" \
  --user-id arjun \
  --repo-path .
```

Expected context includes:

- `No in-memory queues`
- `Idempotent handlers`
- `No new dependencies`
- `Arjun Go style`

## Job Processing Scenario

Demo repo scenario: a simple Go job processing service.

Baseline prompt:

```text
Implement async job submission and processing.
```

Memory-enabled prompt:

```text
Use coremem_get_context for this repo and then implement async job submission and processing.
```

Expected difference:

- Without memory: Claude may choose an in-memory queue or background goroutine-only design.
- With memory: Claude should prefer a persistent job table/state machine, mention idempotency, and avoid new dependencies.

Suggested scoring:

| Check | Baseline | With coremem |
| --- | --- | --- |
| Avoids in-memory queues | 0/1 | 1/1 |
| Uses persistent job state | 0/1 | 1/1 |
| Addresses idempotency | 0/1 | 1/1 |
| Avoids new dependencies | 0/1 | 1/1 |

## Style Preference Scenario

Run the same prompt twice with different users.

Prompt:

```text
Add CSV import support with validation and tests.
```

Arjun:

```text
Use coremem_get_context with user_id arjun for this repo, then add CSV import support with validation and tests.
```

Expected: explicit error handling, small functions, simple interfaces, and table-driven tests.

Meena:

```text
Use coremem_get_context with user_id meena for this repo, then add CSV import support with validation and tests.
```

Expected: composable helpers, reusable validation functions, and concise service-layer abstractions.

## Notes

This POC intentionally avoids embeddings and graph algorithms. Retrieval is a simple ranked SQLite query plus active one-hop neighbor inclusion. The database is the memory source of truth; Markdown is only used for generated documentation and display.
