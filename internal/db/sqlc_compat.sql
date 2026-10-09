-- =====================================================================
-- sqlc catalog compatibility stub — NOT a migration, NOT a query file.
--
-- Migration 000824 dropped mentorship_connections, but the legacy
-- connection queries in internal/db/queries/mentorship.sql and gdpr.sql
-- still reference it (they are dead at runtime; the mentorship compat
-- rewrite in tasks 6/7 of 2026-10-09-services-mentorship-unification
-- removes them). sqlc builds its catalog by replaying migrations/, so
-- without this stub `make sqlc-gen` fails on the dangling references and
-- the gen.MentorshipConnection model disappears, breaking
-- internal/mentorship + internal/gdpr + internal/server callers that the
-- compat tasks have not rewired yet.
--
-- The definition below is the baseline 000735 shape verbatim (the same
-- one 000824.down.sql recreates), so the generated model is identical to
-- the pre-drop model and every existing caller keeps compiling.
--
-- REMOVE this file (and the schema entry in sqlc.yaml) together with the
-- mentorship_connections queries in Task 6/7 — until then any new query
-- written against this table would compile but fail at runtime.
-- =====================================================================

CREATE TABLE IF NOT EXISTS public.mentorship_connections (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    mentor_id uuid NOT NULL,
    mentee_id uuid NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    goals text,
    destination_space_id uuid,
    responded_at timestamp with time zone,
    completed_at timestamp with time zone,
    cancelled_at timestamp with time zone,
    cancel_reason text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);
