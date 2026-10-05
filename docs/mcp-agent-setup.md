# Connecting AI Agents to JapaConnect

JapaConnect exposes a remote MCP server (`/api/v1/mcp`, Streamable HTTP + SSE
fallback) and a scoped public REST API. This page covers every supported agent
surface. See `.jeos/sections/119-mcp-server.md` (bridge internals) and
`.jeos/sections/120-mcp-agent-distribution.md` (OAuth AS, aliases, GPT Actions)
for the specs.

Production endpoints:

- MCP: `https://api.japaconnect.com/api/v1/mcp`
- OAuth discovery: `https://api.japaconnect.com/.well-known/oauth-protected-resource`
- GPT Actions schema: `https://api.japaconnect.com/docs/gpt-actions.json`

Two credential paths exist:

| Path | Used by | Auth |
|---|---|---|
| OAuth 2.1 (PKCE + DCR) | claude.ai, ChatGPT connectors, VS Code MCP | `Authorization: Bearer jcpat_*` |
| API key (`X-API-Key`) | Local/CLI agents, GPT Actions | `X-API-Key: jc_key_*` |

Both end in the same read-only tool gate. `agent.use` scope is always required.

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
      "url": "https://api.japaconnect.com/api/v1/mcp",
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
      "url": "https://api.japaconnect.com/api/v1/mcp",
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
      "url": "https://api.japaconnect.com/api/v1/mcp",
      "headers": { "X-API-Key": "{env:JC_MCP_API_KEY}" }
    }
  }
}
```

### Windsurf / Cline / generic MCP hosts

Any host that supports remote Streamable HTTP servers with custom headers can
attach the same URL + `X-API-Key` pair. Hosts that only support stdio need a
stdio→HTTP bridge process (tracked in ADR-116 follow-ups).

## Web agents (OAuth)

Web products cannot accept a static `X-API-Key`; they run the OAuth 2.1 flow
against JapaConnect's authorization server. No configuration of secrets is
needed — clients register themselves via RFC 7591 DCR.

### claude.ai (custom connector)

1. Settings → Connectors → Add custom connector.
2. Paste `https://api.japaconnect.com/api/v1/mcp`.
3. Claude discovers `/.well-known/oauth-protected-resource` →
   `oauth-authorization-server` → `oauth/register` automatically, then opens
   the JapaConnect consent page (`/oauth/authorize`).
4. Sign in (existing `jc_token` session) and approve the scope grant.

### ChatGPT connector (developer mode / deep research)

1. Enable developer mode in ChatGPT.
2. Add connector with server URL `https://api.japaconnect.com/api/v1/mcp`.
3. ChatGPT performs the same OAuth discovery + DCR + PKCE flow and requires
   the `search` and `fetch` tools — JapaConnect exposes them as aliases over
   `content.search` and `content.read`.

### VS Code MCP / other OAuth-capable hosts

Point the host at the MCP URL; it will discover OAuth via the
`WWW-Authenticate` challenge on unauthenticated requests or the well-known
documents directly.

## ChatGPT Custom GPT Action (API key)

1. Create a Custom GPT → Actions → paste or import
   `https://api.japaconnect.com/docs/gpt-actions.json` (OpenAPI 3.0.3,
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
- Access tokens inherit the read-only API-key gate; `api.write.*` scopes are
  never granted to OAuth clients.
- Refresh tokens rotate on every use. A second presentation within
  `mcp.oauth_reuse_grace_seconds` (default 30s) is treated as a lost-response
  retry (`invalid_grant`, tokens stay live); an older replay revokes the
  token family.
- Clients can self-revoke via `/oauth/revoke`. A user-facing connected-apps
  UI in `/dashboard` is a planned follow-up (§120).

## Deployment checklist

- `JC_MCP_PUBLIC_URL=https://api.japaconnect.com` — issuer + consent base.
- `JC_SITE_URL=https://japaconnect.com` — login redirect target.
- `jc_token` cookie domain must cover the API host (`JC_COOKIE_DOMAIN` —
  e.g. `.japaconnect.com`) so the consent page sees the session.
- `PRIVATE_API_PUBLIC_ORIGIN=https://api.japaconnect.com` on the main-site
  deployment **only when** `PRIVATE_API_BASE_URL` points at an internal
  address that differs from the public issuer — otherwise the post-login
  redirect allowlist rejects the consent URL.
- `JC_ENCRYPTION_KEY` must be set (signs the consent token; falls back to
  `JC_JWT_SECRET`).

## Registry manifest

`server.json` at the repo root is the official MCP registry manifest
(`com.japaconnect/mcp`). Publish with the `mcp-publisher` CLI after the
production domain is live.
