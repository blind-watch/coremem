# Claude Code Local Setup

This setup runs `coremem` locally with SQLite and exposes both MCP stdio tools and local HTTP hook endpoints.

## 1. Build

```sh
go build -o bin/coremem ./cmd/coremem
```

## 2. Initialize

```sh
./bin/coremem migrate
./bin/coremem demo
```

The default database path is `~/.coremem/coremem.db`. To override it:

```sh
export COREMEM_DB_PATH=/absolute/path/to/coremem.db
```

## 3. Add MCP Server To Claude Code

```sh
claude mcp add --transport stdio --scope user coremem -- /absolute/path/to/bin/coremem mcp
```

Then ask Claude Code to call:

```text
Use coremem_get_context for this repo and user_id arjun before implementing.
```

## 4. Start HTTP Hook Server

```sh
./bin/coremem http --addr 127.0.0.1:8765
```

Keep this process running while Claude Code is active.

## 5. Example Hooks Config

Create or update `.claude/settings.local.json`:

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

Use your own `user_id` in the hook URLs. Claude Code hook schemas can vary by installed version, so keep the endpoint URLs and adjust the surrounding hook config if your installed version differs.

## Hosted Hook URL

After adding authentication, TLS, tenant isolation, and a managed database, the hook URLs can point at a hosted service:

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

Do not expose the current local POC directly to the internet. See `docs/cloud-native-hosting.md`.

## Manual Hook Smoke Test

```sh
curl -sS -X POST 'http://127.0.0.1:8765/hooks/user-prompt-submit?plain=1' \
  -H 'Content-Type: application/json' \
  --data '{"prompt":"Implement async job processing","cwd":"'$(pwd)'","user_id":"arjun","session_id":"local-smoke"}'
```

Expected output includes a `<coremem_context>` block after `./bin/coremem demo`.
