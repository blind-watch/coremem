# coremem

`coremem` is a local-first MCP server for Claude Code that provides persistent, repo-aware, user-scoped core memory without using Markdown files as the source of truth.

The current POC stores explicit durable memories in SQLite. It exposes:

- a stdio MCP server for Claude Code tools
- an HTTP hook server for Claude Code lifecycle hooks
- a CLI for migration, demo seeding, manual memory writes, search, and context retrieval

Markdown files in this repository are documentation only. Durable memory lives in the database.

## Current Status

Implemented:

- SQLite schema and migration
- memory nodes, edges, events, sessions, users, repos, and workspaces
- simple ranked search without embeddings
- superseded memory handling
- explicit negative memory
- user-scoped preferences
- repo-aware context retrieval
- stdio MCP tools
- HTTP hook endpoints
- demo seed data
- tests for core behavior

Not implemented yet:

- authentication for public hosting
- remote MCP transport
- embeddings or pgvector search
- multi-user cloud control plane
- UI

## Repository Layout

```text
cmd/coremem/main.go          CLI entrypoint
internal/config/             DB path and environment config
internal/db/                 SQLite open/migration logic
internal/memory/             memory service, retrieval, supersession
internal/search/             simple ranking helpers
internal/parser/             [coremem:...] block parser
internal/mcpserver/          small stdio JSON-RPC MCP implementation
internal/hooks/              Claude Code HTTP hook server
migrations/                  embedded SQLite migrations
docs/                        setup, benchmark, design, hosting docs
```

## Build

```sh
go build -o bin/coremem ./cmd/coremem
```

## Initialize Local SQLite

Default DB path:

```text
~/.coremem/coremem.db
```

Override it when needed:

```sh
export COREMEM_DB_PATH=/absolute/path/to/coremem.db
```

Run migrations:

```sh
./bin/coremem migrate
```

Seed the current repo with demo memories:

```sh
./bin/coremem demo
```

Important: `demo` seeds memories for the directory you run it from. For a side-by-side Claude Code demo, run it from the memory-enabled demo repo:

```sh
cd /path/to/job-demo-1
/Users/arjun-etpl/ennoventure/repos/coremem/bin/coremem demo
```

## CLI Usage

Add a repo constraint:

```sh
./bin/coremem add \
  --type core_constraint \
  --scope repo \
  --title "No in-memory queues" \
  --body "Do not use in-memory queues for jobs because workers run in multiple pods." \
  --repo-path . \
  --user-id arjun
```

Search memories:

```sh
./bin/coremem search queue --repo-path . --user-id arjun
```

Print a context patch:

```sh
./bin/coremem context \
  --prompt "Implement async job processing" \
  --repo-path . \
  --user-id arjun
```

Expected demo context includes:

- `No in-memory queues`
- `Idempotent handlers`
- `No new dependencies`
- `Arjun Go style`

## Claude Code MCP Setup

Claude Code can connect to `coremem` as a local stdio MCP server.

Add the server:

```sh
claude mcp add --transport stdio --scope user coremem -- /absolute/path/to/bin/coremem mcp
```

Example from this repository:

```sh
claude mcp add --transport stdio --scope user coremem -- /Users/arjun-etpl/ennoventure/repos/coremem/bin/coremem mcp
```

Verify:

```sh
claude mcp list
```

Inside Claude Code, run:

```text
/mcp
```

The server should appear as connected and expose tools.

### MCP Tools

`coremem` exposes:

- `coremem_add`
- `coremem_search`
- `coremem_get_context`
- `coremem_supersede`
- `coremem_link`
- `coremem_recent`

### Claude Code Smoke Tests

Ask Claude Code in the target repo:

```text
Use the coremem_search MCP tool with query "queue", repo_path as the current repo path, user_id "arjun", and limit 5. Return only the tool result.
```

Expected: a result titled `No in-memory queues`.

Then:

```text
Use the coremem_get_context MCP tool with prompt "Implement async job processing", repo_path as the current repo path, user_id "arjun", and limit 8. Return only the context patch.
```

Expected: a `<coremem_context>` block with repo constraints, negative memories, and the current user's preferences.

## Claude Code Hook Setup

The MCP server lets Claude explicitly call tools. Hooks let Claude Code automatically send lifecycle events to `coremem`, so context can be retrieved before a turn or restored at session start.

Start the local HTTP hook server:

```sh
./bin/coremem http --addr 127.0.0.1:8765
```

Local endpoints:

- `POST /hooks/user-prompt-submit`
- `POST /hooks/stop`
- `POST /hooks/session-start`
- `POST /hooks/pre-compact`

For local testing, `?plain=1` returns only the context patch as text:

```sh
curl -sS -X POST 'http://127.0.0.1:8765/hooks/user-prompt-submit?plain=1&user_id=arjun' \
  -H 'Content-Type: application/json' \
  --data "{\"prompt\":\"Implement async job processing\",\"cwd\":\"$(pwd)\",\"session_id\":\"smoke\"}"
```

### Local `.claude/settings.local.json`

Use this in the repo where Claude Code runs:

```json
{
  "hooks": {
    "UserPromptSubmit": [
      {
        "hooks": [
          {
            "type": "http",
            "url": "http://127.0.0.1:8765/hooks/user-prompt-submit?user_id=arjun",
            "timeout": 5
          }
        ]
      }
    ],
    "Stop": [
      {
        "hooks": [
          {
            "type": "http",
            "url": "http://127.0.0.1:8765/hooks/stop?user_id=arjun",
            "timeout": 5
          }
        ]
      }
    ],
    "SessionStart": [
      {
        "matcher": "startup|resume|compact",
        "hooks": [
          {
            "type": "http",
            "url": "http://127.0.0.1:8765/hooks/session-start?user_id=arjun",
            "timeout": 5
          }
        ]
      }
    ],
    "PreCompact": [
      {
        "matcher": "manual|auto",
        "hooks": [
          {
            "type": "http",
            "url": "http://127.0.0.1:8765/hooks/pre-compact?user_id=arjun",
            "timeout": 5
          }
        ]
      }
    ]
  }
}
```

Keep `.claude/settings.local.json` uncommitted because it is user and machine specific.

### Hosted Hook URL Shape

Once `coremem` is deployed behind HTTPS with authentication, the same hook config can point to a hosted URL:

```json
{
  "hooks": {
    "UserPromptSubmit": [
      {
        "hooks": [
          {
            "type": "http",
            "url": "https://coremem.example.com/hooks/user-prompt-submit?user_id=arjun",
            "timeout": 5
          }
        ]
      }
    ],
    "Stop": [
      {
        "hooks": [
          {
            "type": "http",
            "url": "https://coremem.example.com/hooks/stop?user_id=arjun",
            "timeout": 5
          }
        ]
      }
    ],
    "SessionStart": [
      {
        "matcher": "startup|resume|compact",
        "hooks": [
          {
            "type": "http",
            "url": "https://coremem.example.com/hooks/session-start?user_id=arjun",
            "timeout": 5
          }
        ]
      }
    ],
    "PreCompact": [
      {
        "matcher": "manual|auto",
        "hooks": [
          {
            "type": "http",
            "url": "https://coremem.example.com/hooks/pre-compact?user_id=arjun",
            "timeout": 5
          }
        ]
      }
    ]
  }
}
```

Do not expose the current POC directly to the public internet. A hosted deployment needs TLS, auth, tenant isolation, request limits, and a managed database first. See [docs/cloud-native-hosting.md](docs/cloud-native-hosting.md).

## Memory Tag Format

Claude Code prompts or stop payloads can contain explicit durable-memory blocks:

```text
[coremem:type=core_constraint scope=repo title="No in-memory queues"]
Do not use in-memory queues for jobs because workers run in multiple pods.
[/coremem]
```

Examples:

```text
[coremem:type=core_negative scope=repo title="Do not use FSx hot path"]
Do not use FSx/OpenZFS for per-frame hot-path storage unless new latency numbers prove it is safe.
[/coremem]
```

```text
[coremem:type=core_preference scope=user title="User style preference"]
This user prefers explicit error handling, small functions, and table-driven tests.
[/coremem]
```

Parser rules:

- `type` is required
- `title` is required
- `scope` defaults to `repo`
- body is the text between start and end tags
- invalid types and scopes are rejected
- user prompt blocks are stored as `user_tagged`
- assistant stop blocks are stored as `agent_tagged`

## Retrieval Behavior

`coremem_get_context` and `coremem context` call:

```text
GetRelevantContext(workspaceID, repoPath, userID, prompt, filePaths, limit)
```

The POC retrieval strategy:

- resolves repo by root path
- searches active memories by case-insensitive `LIKE`
- matches title, body, tags, entities, and file paths
- boosts exact terms, same repo, same user, active status, core memory types, importance, and recency
- includes user-scoped preferences for the current user
- includes relevant negative memories
- excludes superseded memories by default
- includes the strongest active one-hop neighbors
- returns a compact generated context patch under roughly 1200 words

Context patch shape:

```text
<coremem_context>
Reason: retrieved from persistent DB memory for this repo/user/session.

Relevant constraints:
- ...

Relevant negative memories:
- ...

Relevant decisions:
- ...

Relevant user preferences:
- ...

Supersession notes:
- ...

Relevant files:
- ...
</coremem_context>
```

## Demo Flow

For the side-by-side demo:

- `job-demo-1`: Claude Code with `coremem` MCP and hooks enabled
- `job-demo-2`: Claude Code without `coremem`

Seed only `job-demo-1`:

```sh
cd /path/to/job-demo-1
/Users/arjun-etpl/ennoventure/repos/coremem/bin/coremem demo
```

Then run the same prompts in both folders. See [docs/demo-prompts.md](docs/demo-prompts.md).

## Tests

```sh
go test ./...
```

## Security Notes

- Memory contents are stored as data and never executed.
- The local POC writes only to the configured SQLite database.
- The HTTP server defaults to `127.0.0.1:8765`.
- Do not bind the POC to `0.0.0.0` on an untrusted network.
- Do not put secrets in memory bodies, hook URLs, or demo data.
- MCP stdio mode must never write logs to stdout because stdout is reserved for JSON-RPC.

## Docs

- [Claude Code local setup](docs/claude-code-local-setup.md)
- [Demo prompts](docs/demo-prompts.md)
- [Benchmark](docs/benchmark.md)
- [Design](docs/design.md)
- [Cloud-native hosting path](docs/cloud-native-hosting.md)
