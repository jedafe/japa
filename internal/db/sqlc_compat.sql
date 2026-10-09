-- =====================================================================
-- sqlc catalog compatibility stub — NOT a migration, NOT a query file.
--
-- Migration 000824 dropped mentorship_connections and Task 6 of
-- 2026-10-09-services-mentorship-unification removed every query and
-- call site in internal/mentorship that used it (connection state now
-- lives in service_bookings behind the mentor's shadow listing). The
-- table is still referenced by the dead anonymization/export queries in
-- internal/db/queries/gdpr.sql — Task 7 owns cleaning those.
--
-- sqlc builds its catalog by replaying migrations/, so without this stub
-- `make sqlc-gen` fails on the dangling gdpr.sql references and the
-- gen.MentorshipConnection model disappears, breaking internal/gdpr
-- callers that Task 7 has not rewired yet.
--
-- The definition below is the baseline 000735 shape verbatim (the same
-- one 000824.down.sql recreates), so the generated model is identical to
-- the pre-drop model and the remaining gdpr callers keep compiling.
--
-- REMOVE this file (and the schema entry in sqlc.yaml) in Task 7 together
-- with the mentorship_connections queries in gdpr.sql — until then any
-- new query written against this table would compile but fail at
-- runtime.
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
