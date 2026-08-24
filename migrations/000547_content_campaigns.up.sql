-- 000547_content_campaigns.up.sql
-- Section 104 (AI Content Campaigns) — campaign and campaign item tables.
--
-- Ref: .jeos/sections/104-content-campaigns.md
--      docs/superpowers/specs/2026-08-24-content-campaigns-design.md

BEGIN;

CREATE TABLE IF NOT EXISTS content_campaigns (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id       uuid NOT NULL REFERENCES tenants(id),
  name            text NOT NULL,
  prompt          text NOT NULL,
  status          text NOT NULL DEFAULT 'planning'
                    CHECK (status IN ('planning','planned','generating','completed','failed','cancelled')),
  content_type    text,
  requested_count int NOT NULL
                    CHECK (requested_count > 0 AND requested_count <= 100),
  persona_pool    uuid[],
  auto_provision_personas boolean NOT NULL DEFAULT true,
  auto_assign     boolean NOT NULL DEFAULT true,
  target_space_id uuid REFERENCES spaces(id),
  target_tags     text[],
  plan_json       jsonb,
  created_by      uuid NOT NULL REFERENCES users(id),
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_content_campaigns_tenant_status
  ON content_campaigns (tenant_id, status, created_at DESC);

CREATE TABLE IF NOT EXISTS content_campaign_items (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id       uuid NOT NULL REFERENCES tenants(id),
  campaign_id     uuid NOT NULL REFERENCES content_campaigns(id) ON DELETE CASCADE,
  sort_order      int NOT NULL DEFAULT 0,
  persona_id      uuid NOT NULL REFERENCES ai_personas(id),
  content_type    text NOT NULL,
  topic_prompt    text NOT NULL,
  title           text,
  target_space_id uuid REFERENCES spaces(id),
  target_tags     text[],
  status          text NOT NULL DEFAULT 'planned'
                    CHECK (status IN ('planned','queued','generating','draft_ready','in_review','approved','published','rejected','failed')),
  studio_job_id   uuid REFERENCES studio_jobs(id),
  content_id      uuid REFERENCES contents(id),
  review_id       uuid REFERENCES content_reviews(id),
  error_message   text,
  created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_content_campaign_items_campaign
  ON content_campaign_items (campaign_id, status, sort_order);

CREATE UNIQUE INDEX IF NOT EXISTS idx_content_campaign_items_unique_persona
  ON content_campaign_items (campaign_id, persona_id)
  WHERE status NOT IN ('failed','cancelled');

COMMIT;
