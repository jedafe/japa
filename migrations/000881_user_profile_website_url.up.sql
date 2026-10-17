-- §50 PATCH /me/profile accepts website_url; the column was never created.
ALTER TABLE user_profiles ADD COLUMN website_url text;
