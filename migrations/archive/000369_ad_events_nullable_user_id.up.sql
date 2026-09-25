-- 000369: Make ad_events.user_id nullable for guest event ingestion.
-- Ref: japaconnect-engineering/sections/25-ad-placement.md
--
-- POST /ads/events is a public endpoint (catalog: [public, optional auth]).
-- Guests can submit impression/click events. The user_id column was NOT NULL,
-- which silently dropped all guest events (the service skipped events with
-- invalid user IDs). This ALTER allows NULL user_id for anonymous events.
ALTER TABLE ad_events ALTER COLUMN user_id DROP NOT NULL;

COMMENT ON COLUMN ad_events.user_id IS 'The user who triggered the event. NULL for anonymous/guest events (public /ads/events endpoint).';
