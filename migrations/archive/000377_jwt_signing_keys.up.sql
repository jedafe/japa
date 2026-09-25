-- JWT signing keys table for zero-disruption key rotation.
-- Ref: japaconnect-engineering/sections/02-authentication-sessions.md
--
-- Multiple active signing secrets can coexist, identified by a kid (key ID)
-- header in the JWT. The validator checks kid against the active rows here.
-- Old secrets are retired (active=false, retired_at set) after one access
-- token TTL — no user gets logged out on secret rotation.
--
-- This table is NOT tenant-scoped: JWT signing keys are global to the
-- application instance. The secret is encrypted at rest via JC_ENCRYPTION_KEY
-- using the shared internal/crypto AES-256-GCM helpers.
CREATE TABLE jwt_signing_keys (
    kid        text PRIMARY KEY,
    secret     text NOT NULL,
    active     boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    retired_at timestamptz
);

CREATE INDEX idx_jwt_signing_keys_active ON jwt_signing_keys (active) WHERE active = true;
