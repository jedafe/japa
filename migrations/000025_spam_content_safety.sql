-- Section 41: Spam Prevention & Content Safety

-- IP Blocking
CREATE TABLE ip_blocks (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    cidr         inet NOT NULL,
    block_type   text NOT NULL DEFAULT 'full',  -- 'full'|'registration'|'posting'|'api'
    reason       text,
    blocked_by   uuid REFERENCES users(id),
    expires_at   timestamptz,  -- NULL = permanent
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_ip_blocks_cidr ON ip_blocks USING gist (cidr inet_ops);
CREATE INDEX idx_ip_blocks_tenant ON ip_blocks(tenant_id);

-- User-to-User Blocking
CREATE TABLE user_blocks (
    blocker_id  uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    blocked_id  uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (blocker_id, blocked_id)
);
CREATE INDEX idx_user_blocks_blocked ON user_blocks (blocked_id);

-- Censored Words
CREATE TABLE censored_words (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    pattern     text NOT NULL,
    action      text NOT NULL DEFAULT 'block',  -- 'block'|'replace'|'flag'
    replacement text,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_censored_words_tenant ON censored_words(tenant_id);

-- Duplicate Content Detection
-- content_hash column and unique index already exist from migration 000010
-- (contents table already exists from earlier sections)
ALTER TABLE contents ADD COLUMN IF NOT EXISTS content_hash text;
-- Index idx_contents_tenant_content_hash already created in migration 000010;
-- no duplicate index needed here.
