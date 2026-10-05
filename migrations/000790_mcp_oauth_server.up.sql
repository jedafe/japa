-- 000790_mcp_oauth_server.up.sql
-- OAuth 2.1 authorization server for the §119 MCP bridge (§120, ADR-126).
-- Lets web agents (claude.ai custom connectors, ChatGPT connectors) obtain
-- per-user bearer tokens via dynamic client registration + authorization
-- code with PKCE S256 + rotating refresh tokens. Issued tokens map onto the
-- API-key read-only tool gate — no new authorization surface.
--
-- All bearer material (client secrets are never issued; auth codes, access
-- and refresh tokens) is stored as SHA-256 hashes, matching the api_keys
-- posture from §47.
--
-- Ref: .jeos/sections/120-mcp-agent-distribution.md
--      .jeos/adr/ADR-126-mcp-agent-distribution.md

BEGIN;

CREATE TABLE public.oauth_clients (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    client_id text NOT NULL,
    client_name text NOT NULL,
    redirect_uris text[] NOT NULL,
    grant_types text[] DEFAULT '{authorization_code,refresh_token}'::text[] NOT NULL,
    scopes text[] DEFAULT '{}'::text[] NOT NULL,
    token_endpoint_auth_method text DEFAULT 'none'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT oauth_clients_pkey PRIMARY KEY (id),
    CONSTRAINT oauth_clients_client_id_key UNIQUE (client_id)
);

CREATE TABLE public.oauth_authorization_codes (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    code_hash text NOT NULL,
    client_id text NOT NULL,
    user_id uuid NOT NULL,
    redirect_uri text NOT NULL,
    scope text NOT NULL,
    code_challenge text NOT NULL,
    code_challenge_method text DEFAULT 'S256'::text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    used_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT oauth_authorization_codes_pkey PRIMARY KEY (id),
    CONSTRAINT oauth_authorization_codes_code_hash_key UNIQUE (code_hash)
);

CREATE TABLE public.oauth_tokens (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    token_hash text NOT NULL,
    client_id text NOT NULL,
    user_id uuid NOT NULL,
    scope text NOT NULL,
    token_type text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    revoked_at timestamp with time zone,
    replaced_by uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT oauth_tokens_pkey PRIMARY KEY (id),
    CONSTRAINT oauth_tokens_token_hash_key UNIQUE (token_hash),
    CONSTRAINT oauth_tokens_token_type_check CHECK (token_type IN ('access', 'refresh'))
);

ALTER TABLE ONLY public.oauth_authorization_codes
    ADD CONSTRAINT oauth_authorization_codes_client_id_fkey FOREIGN KEY (client_id) REFERENCES public.oauth_clients(client_id) ON DELETE CASCADE;
ALTER TABLE ONLY public.oauth_authorization_codes
    ADD CONSTRAINT oauth_authorization_codes_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;
ALTER TABLE ONLY public.oauth_tokens
    ADD CONSTRAINT oauth_tokens_client_id_fkey FOREIGN KEY (client_id) REFERENCES public.oauth_clients(client_id) ON DELETE CASCADE;
ALTER TABLE ONLY public.oauth_tokens
    ADD CONSTRAINT oauth_tokens_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

CREATE INDEX idx_oauth_codes_client ON public.oauth_authorization_codes (client_id, created_at DESC);
CREATE INDEX idx_oauth_tokens_user ON public.oauth_tokens (user_id, token_type);
CREATE INDEX idx_oauth_tokens_client ON public.oauth_tokens (client_id, token_type);
CREATE INDEX idx_oauth_tokens_expiry ON public.oauth_tokens (expires_at) WHERE revoked_at IS NULL;

COMMIT;
