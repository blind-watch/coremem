# Cloud-Native Hosting Path

This document describes how `coremem` can evolve from a local SQLite POC into a hosted, cloud-native service.

The current code is local-first. Do not expose it directly to the public internet without adding authentication, tenant isolation, managed storage, and operational controls.

## Target Architecture

```text
Claude Code
  | HTTPS hooks
  v
Cloud load balancer / ingress
  |
  v
coremem HTTP service
  |
  +--> Postgres + pgvector
  +--> object/log storage for audit exports
  +--> metrics/logging/tracing

Claude Code
  | remote MCP transport, future
  v
coremem MCP service
```

## Hosting Modes

### Mode 1: Local MCP Plus Hosted Hooks

This is the easiest transitional model.

- MCP remains local stdio.
- Claude Code HTTP hooks call `https://coremem.example.com/hooks/...`.
- Hosted service stores prompt/session events and returns context.
- Local CLI can still be used for manual debugging against local SQLite.

This mode needs auth and tenant isolation before production use.

### Mode 2: Hosted Hooks Plus Hosted MCP

In this model both hooks and MCP are remote.

- Hooks use HTTPS.
- MCP uses a remote transport supported by the target Claude Code version.
- Auth is required for both surfaces.
- Memory access is scoped by tenant, workspace, repo, and user.

The current POC does not implement hosted MCP transport.

## Hosted Hook Configuration

Once hosted safely, `.claude/settings.local.json` can point at a public HTTPS URL:

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

Recommended hosted URL shape:

```text
https://coremem.example.com/hooks/user-prompt-submit?user_id=arjun
https://coremem.example.com/hooks/stop?user_id=arjun
https://coremem.example.com/hooks/session-start?user_id=arjun
https://coremem.example.com/hooks/pre-compact?user_id=arjun
```

For production, prefer authenticated identity over query-string user IDs. Query-string `user_id` is acceptable for local demos and early private prototypes.

## Auth Requirements

Before public hosting, add:

- bearer token or signed hook secret verification
- per-user or per-workspace API keys
- rotation support
- rejected unauthenticated requests by default
- no secrets in logs

For hooks, a practical first step is:

```text
Authorization: Bearer <coremem-hook-token>
```

Claude Code hook config should inject this through supported HTTP hook headers when available, or through a local proxy that adds the header.

## Database Migration Path

Local POC:

```text
SQLite
```

Hosted target:

```text
Postgres
Postgres JSONB for tags/entities/file_paths
pgvector for embeddings
```

Suggested hosted schema changes:

- add `tenant_id` to every table
- replace JSON string columns with `JSONB`
- add `embedding vector(...)` to `memory_nodes`
- add indexes for `(tenant_id, repo_id, user_id, status, type)`
- add full-text search indexes for title/body
- add vector index after embedding quality is proven

## Service Decomposition

Keep the initial hosted service monolithic:

```text
coremem-api
  /hooks/*
  /mcp/*
  /healthz
  /readyz
```

Avoid splitting services until there is real load or team ownership pressure.

Possible later services:

- `coremem-api`: hooks, MCP, admin API
- `coremem-worker`: embedding generation, summarization, cleanup
- `coremem-migrator`: schema migration job

## Kubernetes Deployment Shape

Minimal production-shaped deployment:

- `Deployment/coremem-api`
- `Service/coremem-api`
- `Ingress` or Gateway API route
- `Secret` for DB URL and hook auth keys
- `ConfigMap` for non-secret config
- managed Postgres outside the cluster
- horizontal pod autoscaling after metrics exist

Required endpoints:

- `/healthz`: process is alive
- `/readyz`: DB reachable and migrations compatible

## Configuration

Likely environment variables:

```text
COREMEM_DB_URL=postgres://...
COREMEM_HOOK_AUTH_REQUIRED=true
COREMEM_HOOK_TOKENS=...
COREMEM_PUBLIC_BASE_URL=https://coremem.example.com
COREMEM_LOG_LEVEL=info
COREMEM_MAX_REQUEST_BYTES=1048576
```

The current POC uses:

```text
COREMEM_DB_PATH=/path/to/coremem.db
```

## Tenant And Repo Resolution

Hosted `repo_path` is not globally unique. Use a stable repo identity:

- tenant ID
- workspace ID
- git remote URL
- normalized repo root path as a fallback

For Claude Code hooks, cwd is useful locally. Hosted deployments should enrich repo identity with git remote data when available.

## Observability

Track:

- hook request count and latency
- MCP tool call count and latency
- DB query latency
- context retrieval result count
- empty retrieval rate
- auth failures
- request body size rejections
- migration version

Structured logs should include:

- request ID
- tenant ID
- workspace ID
- repo ID
- user ID
- endpoint/tool name

Do not log prompt bodies by default.

## Security Checklist

Before internet exposure:

- TLS only
- auth required
- per-tenant data isolation
- request body size limits
- rate limits
- no raw prompt logging by default
- no shell execution from memory
- no arbitrary file writes
- DB backups enabled
- migration rollback plan
- audit trail for memory changes

## Rollout Plan

1. Keep local SQLite and MCP stdio as the default developer path.
2. Add a Postgres storage implementation behind the existing service methods.
3. Add tenant IDs and auth middleware.
4. Deploy hosted hooks privately behind VPN or an allowlisted ingress.
5. Add metrics and health checks.
6. Add embedding generation behind a feature flag.
7. Add remote MCP only after the hook path is stable.

The design should stay boring until the retrieval quality or operational load proves a more complex architecture is necessary.
