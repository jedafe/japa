-- API Key IP Allowlisting
-- Restricts API keys to specific IP addresses or CIDR ranges.

CREATE TABLE api_key_ip_allowlist (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    api_key_id UUID NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
    cidr INET NOT NULL,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(api_key_id, cidr)
);
CREATE INDEX idx_api_key_ip_allowlist_key ON api_key_ip_allowlist(api_key_id);
