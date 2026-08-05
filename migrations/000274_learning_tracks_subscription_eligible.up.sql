-- Add subscription_eligible flag to learning_tracks.
-- When true, the track can be attached to subscription plans and
-- accessed by subscribers (with pro_stream revenue splits for creators).
-- Combined with price_kobo, this gives four monetization models:
--   free:              price_kobo = 0/null, subscription_eligible = false
--   purchase only:     price_kobo > 0,       subscription_eligible = false
--   subscription only: price_kobo = 0/null, subscription_eligible = true
--   both:              price_kobo > 0,       subscription_eligible = true

ALTER TABLE learning_tracks ADD COLUMN subscription_eligible boolean NOT NULL DEFAULT false;
