# Connecting AI Agents to JapaConnect

Japa.to exposes a remote MCP server (`/api/v1/mcp`, Streamable HTTP + SSE
fallback) and a scoped public REST API. This page covers every supported agent
surface. See `.jeos/sections/119-mcp-server.md` (bridge internals) and
`.jeos/sections/120-mcp-agent-distribution.md` (OAuth AS, aliases, GPT Actions)
for the specs.

Production endpoints:

- MCP: `https://api.japa.to/api/v1/mcp`
- OAuth discovery: `https://api.japa.to/.well-known/oauth-protected-resource`
- GPT Actions schema: `https://api.japa.to/docs/gpt-actions.json`

Two credential paths exist:

| Path | Used by | Auth |
|---|---|---|
| OAuth 2.1 (PKCE + DCR) | claude.ai, ChatGPT connectors, VS Code MCP | `Authorization: Bearer jcpat_*` |
| API key (`X-API-Key`) | Local/CLI agents, GPT Actions | `X-API-Key: jc_key_*` |

Both end in the same read-only tool gate. `agent.use` scope is always required.

Exception: an API key that also carries the **`agent.write`** scope bypasses
the read-only gate and receives the key owner's full RBAC-filtered catalog —
the same surface a JWT session gets, writes included. See
[Write-capable keys](#write-capable-keys-agentwrite) below. `agent.write` is
admin-only: it can only be granted via the control-panel API-keys flow (or by
a caller holding `api.keys.manage`), can never be self-minted, and is never
grantable through OAuth — `jcpat_*` tokens stay read-only.

## Local / CLI agents (API key)

Generate a key at `/dashboard/developer` (or admin API-keys flow) with the
`agent.use` scope plus the read scopes you want (`api.read.content` is
recommended minimum for useful results).

### Claude Code / Devin CLI (`.mcp.json`)

```json
{
  "mcpServers": {
    "japaconnect": {
      "type": "http",
      "url": "https://api.japa.to/api/v1/mcp",
      "headers": { "X-API-Key": "${JC_MCP_API_KEY}" }
    }
  }
}
```

### Cursor (`.cursor/mcp.json`)

```json
{
  "mcpServers": {
    "japaconnect": {
      "url": "https://api.japa.to/api/v1/mcp",
      "headers": { "X-API-Key": "${JC_MCP_API_KEY}" }
    }
  }
}
```

### OpenCode (`opencode.jsonc`)

```jsonc
{
  "mcp": {
    "japaconnect": {
      "type": "remote",
      "url": "https://api.japa.to/api/v1/mcp",
      "headers": { "X-API-Key": "{env:JC_MCP_API_KEY}" }
    }
  }
}
```

### Windsurf / Cline / generic MCP hosts

Any host that supports remote Streamable HTTP servers with custom headers can
attach the same URL + `X-API-Key` pair.

### stdio-only hosts (`cmd/mcpbridge`)

Hosts that only support local stdio servers use `cmd/mcpbridge` — a
stdlib-only Go binary that relays newline-delimited JSON-RPC between the
host's stdio transport and the remote endpoint:

```bash
go build -o mcpbridge ./cmd/mcpbridge
```

```json
{
  "mcpServers": {
    "japaconnect": {
      "type": "stdio",
      "command": "/path/to/mcpbridge",
      "env": {
        "JC_MCP_URL": "https://api.japa.to/api/v1/mcp",
        "JC_MCP_API_KEY": "jc_key_..."
      }
    }
  }
}
```

`JC_MCP_URL` defaults to the production endpoint; `JC_MCP_TOKEN` (a
`jcpat_*` OAuth access token or JWT) can replace `JC_MCP_API_KEY` for
Bearer auth. Diagnostics go to stderr; stdout carries only the protocol.

## Write-capable keys (`agent.write`)

For content management through an agent — the catalog's write tools
(`content.create`, `content.update`, `content.publish`, docs/wiki/spaces
writers, the tool-type record CRUD ops, etc.) — mint a key with both
`agent.use` and `agent.write`. The key then acts with its owner's full
RBAC-filtered catalog instead of the read-only gate: every tool the owning
user could reach via a JWT session becomes available, and every boundary that
applies to sessions still applies — RBAC `required_permission` filters,
`is_blocked`/hardcoded blocklist, `ai_exempt` privacy, HITL (`agent.skip_hitl`
still bypasses for admin-group/super-admin owners), and premium `minTier`
gates.

Mint via the admin/control path — self-serve keys are rejected with
`scope_requires_admin` if they include `agent.write`:

```bash
curl -X POST https://api.japa.to/api/v1/control/api-keys \
  -H "Authorization: Bearer <admin-jwt>" \
  -H 'Content-Type: application/json' \
  -d '{"name":"content-agent","scopes":["agent.use","agent.write"]}'
```

(That route requires the caller's `api.keys.manage` permission; the
control-panel API-keys UI exposes the same option.)
Keep the full key out of tracked files — it is shown once at creation and is
bearer-equivalent to the owner's session. Use `${JC_MCP_API_KEY}` env
references in MCP configs, never the literal.

A plain `agent.use` + `api.read.*` key remains read-only — `agent.write` is
the single switch. OAuth `jcpat_*` tokens can never carry it.

## Web agents (OAuth)

Web products cannot accept a static `X-API-Key`; they run the OAuth 2.1 flow
against JapaConnect's authorization server. No configuration of secrets is
needed — clients register themselves via RFC 7591 DCR.

### claude.ai (custom connector)

1. Settings → Connectors → Add custom connector.
2. Paste `https://api.japa.to/api/v1/mcp`.
3. Claude discovers `/.well-known/oauth-protected-resource` →
   `oauth-authorization-server` → `oauth/register` automatically, then opens
   the JapaConnect consent page (`/oauth/authorize`).
4. Sign in (existing `jc_token` session) and approve the scope grant.

### ChatGPT connector (developer mode / deep research)

1. Enable developer mode in ChatGPT.
2. Add connector with server URL `https://api.japa.to/api/v1/mcp`.
3. ChatGPT performs the same OAuth discovery + DCR + PKCE flow and requires
   the `search` and `fetch` tools — JapaConnect exposes them as aliases over
   `content.search` and `content.read`.

### VS Code MCP / other OAuth-capable hosts

Point the host at the MCP URL; it will discover OAuth via the
`WWW-Authenticate` challenge on unauthenticated requests or the well-known
documents directly.

## ChatGPT Custom GPT Action (API key)

1. Create a Custom GPT → Actions → paste or import
   `https://api.japa.to/docs/gpt-actions.json` (OpenAPI 3.0.3,
   read-only subset of `/api/v1/public/*`).
2. Authentication → API Key → header name `X-API-Key` → paste a generated key.
3. Save. The Action calls the same scoped public REST API as any developer key.

## Discovery documents

| URL | Purpose |
|---|---|
| `GET /.well-known/oauth-protected-resource` | RFC 9728 — resource URL + AS list |
| `GET /.well-known/oauth-protected-resource/api/v1/mcp` | Path-scoped variant |
| `GET /.well-known/oauth-authorization-server` | RFC 8414 — endpoints, S256, DCR |
| `POST /oauth/register` | RFC 7591 Dynamic Client Registration |
| `GET/POST /oauth/authorize` | Consent page + code issue |
| `POST /oauth/token` | `authorization_code` + `refresh_token` grants |
| `POST /oauth/revoke` | RFC 7009 revocation |

## Token model

- Access tokens `jcpat_*`, refresh tokens `jcprt_*`, auth codes `jcac_*`,
  client IDs `jcmc_*` — all hashed at rest.
- Access tokens inherit the read-only API-key gate; `api.write.*` and
  `agent.write` scopes are never granted to OAuth clients.
- Refresh tokens rotate on every use. A second presentation within
  `mcp.oauth_reuse_grace_seconds` (default 30s) is treated as a lost-response
  retry (`invalid_grant`, tokens stay live); an older replay revokes the
  token family.
- Clients can self-revoke via `/oauth/revoke`. Users manage their grants at
  **Settings → Connected apps** (`/settings/connected-apps`), backed by
  `GET/DELETE /api/v1/me/oauth/grants*` — revoke drops the whole client/user
  token family (`jc:oauth_famrev` tombstone included).

## Deployment checklist

- `JC_MCP_PUBLIC_URL=https://api.japa.to` — issuer + consent base.
- `JC_SITE_URL=https://japa.to` — login redirect target.
- `jc_token` cookie domain must cover the API host (`JC_COOKIE_DOMAIN` —
  e.g. `.japa.to`) so the consent page sees the session.
- `PRIVATE_API_PUBLIC_ORIGIN=https://api.japa.to` on the main-site
  deployment **only when** `PRIVATE_API_BASE_URL` points at an internal
  address that differs from the public issuer — otherwise the post-login
  redirect allowlist rejects the consent URL.
- `JC_ENCRYPTION_KEY` must be set (signs the consent token; falls back to
  `JC_JWT_SECRET`).

## Registry manifest

`server.json` at the repo root is the official MCP registry manifest
(`com.japaconnect/mcp`). Publish with the `mcp-publisher` CLI after the
production domain is live.
