-- §50 former-username amendment: keeps /u/{old} resolving after a rename.
-- username is text (not citext) to mirror users.username; the PK makes the
-- upsert-release path in RecordUsernameRedirect atomic.
CREATE TABLE username_redirects (
    username   text PRIMARY KEY,
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_username_redirects_user_id ON username_redirects(user_id);
