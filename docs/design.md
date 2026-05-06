# coremem Design

## Goal

`coremem` gives Claude Code durable, explicit, repo-aware memory for coding work. It is intentionally not a generic transcript store.

The server should help Claude remember:

- user-tagged core decisions
- durable repo constraints
- negative memories such as "do not do X"
- user coding preferences
- superseded decisions
- prior agent results

## Non-Goals

The POC does not implement:

- a UI
- Neo4j or graph algorithms
- Markdown memory files as source of truth
- cloud auth
- embeddings
- automatic transcript summarization
- arbitrary shell execution from memory

## Data Model

The database is the memory source of truth.

Primary tables:

- `workspaces`
- `repos`
- `users`
- `sessions`
- `memory_nodes`
- `memory_edges`
- `memory_events`

`memory_nodes` are explicit durable memories. They are not raw chat messages.

Memory types:

- `core_decision`
- `core_constraint`
- `core_negative`
- `core_preference`
- `derived_note`
- `agent_result`

Memory statuses:

- `active`
- `superseded`
- `archived`

Memory scopes:

- `global`
- `workspace`
- `repo`
- `user`
- `session`

Authority levels:

- `user_tagged`
- `agent_tagged`
- `system_observed`

## Local Components

```text
Claude Code
  | MCP stdio tools
  v
coremem mcp
  |
  v
memory service -> SQLite

Claude Code hooks
  | HTTP POST
  v
coremem http
  |
  v
memory service -> SQLite
```

## MCP Path

`coremem mcp` starts a stdio JSON-RPC MCP server.

Claude Code can call:

- `coremem_add`
- `coremem_search`
- `coremem_get_context`
- `coremem_supersede`
- `coremem_link`
- `coremem_recent`

The MCP implementation is isolated in `internal/mcpserver`. Stdio logs must go to stderr only because stdout is reserved for JSON-RPC messages.

## Hook Path

`coremem http --addr 127.0.0.1:8765` starts a local HTTP server.

Hook endpoints:

- `/hooks/user-prompt-submit`
- `/hooks/stop`
- `/hooks/session-start`
- `/hooks/pre-compact`

Hook behavior:

- stores the raw event in `memory_events`
- extracts prompt, cwd, session ID, and user ID when available
- retrieves compact repo/user context on user prompt submit
- retrieves recent/high-importance memory on session start for startup, resume, and compact
- reminds Claude to save durable memories before compaction
- parses explicit `[coremem:...]` blocks from prompt or stop payloads

## Retrieval Design

`GetRelevantContext(workspaceID, repoPath, userID, prompt, filePaths, limit)`:

1. Resolves or creates the repo by root path.
2. Searches active memories matching prompt terms and file paths.
3. Prefers same repo.
4. Includes user-scoped preferences for the current user.
5. Includes relevant negative memories.
6. Excludes superseded memories by default.
7. Includes strongest active one-hop neighbors.
8. Returns a generated context patch.

The context patch is Markdown-like XML text generated from the DB. It is not read from a Markdown memory file.

## Search Design

The POC intentionally avoids embeddings.

Search uses case-insensitive `LIKE` over:

- title
- body
- tags
- entities
- file paths

Ranking boosts:

- exact query match
- term match
- repo match
- user match
- active status
- `core_*` type
- importance
- recency

Future search can add embeddings without changing the high-level service contract.

## Supersession Design

When a memory is superseded:

1. A new active memory is created.
2. The old memory status becomes `superseded`.
3. The old memory's `superseded_by` field points at the new memory.
4. A `memory_edges` row connects old to new with relation `superseded_by`.

Retrieval excludes superseded memories by default, but can include compact supersession notes.

## Explicit Tag Parser

Supported format:

```text
[coremem:type=core_constraint scope=repo title="No in-memory queues"]
Do not use in-memory queues for jobs because workers run in multiple pods.
[/coremem]
```

Rules:

- `type` is required
- `title` is required
- `scope` defaults to `repo`
- body is required
- invalid type/scope values are rejected

Authority:

- parsed from user prompt: `user_tagged`
- parsed from assistant stop payload: `agent_tagged`

## Security Model

Local POC:

- listens on `127.0.0.1` by default
- writes only to SQLite
- does not execute memory contents
- does not log secrets intentionally

Hosted future:

- all requests authenticated
- all data tenant-scoped
- database row-level isolation or equivalent application enforcement
- request size limits
- rate limits
- encrypted transport
- audit logs

## Extension Points

The likely next extensions are:

- Postgres storage implementation
- pgvector embeddings
- remote MCP transport
- auth middleware
- tenant/workspace provisioning
- memory review/admin commands
- import/export tooling
