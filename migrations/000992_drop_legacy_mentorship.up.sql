-- Retire the pre-§102 legacy mentorship model (programs + program-scoped
-- matches). §102 connections live in service_bookings on mentorship-kind
-- shadow listings; legacy data is intentionally not migrated.
DROP TABLE IF EXISTS public.mentorship_matches;
DROP TABLE IF EXISTS public.mentorship_programs;
