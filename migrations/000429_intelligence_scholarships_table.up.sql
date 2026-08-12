-- 000429_intelligence_scholarships_table.up.sql
-- Scholarships domain projection table (reverses §73 per ADR-112).
-- Ref: docs/superpowers/specs/2026-08-12-japa-intelligence-system-design.md (Section 3)

BEGIN;

CREATE TABLE scholarships (
  id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  intel_record_id       uuid REFERENCES intel_records(id) ON DELETE CASCADE,
  guide_id              uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
  source_url            text NOT NULL,
  source_type           text NOT NULL,
  trust_level           smallint NOT NULL,
  title                 text NOT NULL,
  description           text,
  country               text NOT NULL,
  country_space_id      uuid,
  university            text,
  degree_level          text,
  fields_of_study       text[] NOT NULL DEFAULT '{}',
  eligible_countries    text[] NOT NULL DEFAULT '{}',
  funding               jsonb NOT NULL DEFAULT '{}',
  deadline              date,
  application_url       text,
  required_documents    text[] NOT NULL DEFAULT '{}',
  minimum_gpa           numeric(3,2),
  language_requirements jsonb NOT NULL DEFAULT '{}',
  confidence_overall    real NOT NULL,
  verification_status   text NOT NULL DEFAULT 'verified'
                          CHECK (verification_status IN ('verified','needs_review','conflict','stale')),
  freshness_score       int NOT NULL DEFAULT 100,
  override_source       text NOT NULL DEFAULT 'ingestion'
                          CHECK (override_source IN ('ingestion','editorial','mixed')),
  editorial_overrides   jsonb NOT NULL DEFAULT '{}',
  last_verified_at      timestamptz NOT NULL DEFAULT now(),
  created_at            timestamptz NOT NULL DEFAULT now(),
  updated_at            timestamptz NOT NULL DEFAULT now()
);

-- country_space_id FK added via ALTER (spaces already exists, no circular issue)
ALTER TABLE scholarships
  ADD CONSTRAINT scholarships_country_space_id_fkey
  FOREIGN KEY (country_space_id) REFERENCES spaces(id);

-- promoted_record_id FK added via ALTER (circular FK with scholarships, deferred from 428)
ALTER TABLE intel_records
  ADD CONSTRAINT intel_records_promoted_record_id_fkey
  FOREIGN KEY (promoted_record_id) REFERENCES scholarships(id) ON DELETE SET NULL;

CREATE UNIQUE INDEX idx_scholarships_intel      ON scholarships (intel_record_id) WHERE intel_record_id IS NOT NULL;
CREATE UNIQUE INDEX idx_scholarships_guide      ON scholarships (guide_id);
CREATE INDEX idx_scholarships_country_deadline  ON scholarships (country, deadline);
CREATE INDEX idx_scholarships_eligible          ON scholarships USING gin (eligible_countries);
CREATE INDEX idx_scholarships_fields            ON scholarships USING gin (fields_of_study);
CREATE INDEX idx_scholarships_status_fresh      ON scholarships (verification_status, freshness_score);
CREATE INDEX idx_scholarships_space             ON scholarships (country_space_id);

COMMIT;
