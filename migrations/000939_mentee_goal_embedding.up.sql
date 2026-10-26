-- §102 match scoring: mentee-side embedding vector for bio cosine
-- similarity (0.6 keyword + 0.4 pgvector). Populated asynchronously by
-- mentor:generate_embedding from the mentee's latest request goals.
ALTER TABLE user_profiles
    ADD COLUMN IF NOT EXISTS mentee_goal_embedding public.vector(1536);
