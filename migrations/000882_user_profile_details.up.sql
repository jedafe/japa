-- §50 / user-facing profile customization: extended profile detail fields.
-- PII-sensitive (DOB, marital status, gender) — surfaced only on /me routes.
ALTER TABLE user_profiles ADD COLUMN education text;
ALTER TABLE user_profiles ADD COLUMN profession text;
ALTER TABLE user_profiles ADD COLUMN marital_status text;
ALTER TABLE user_profiles ADD COLUMN date_of_birth date;
ALTER TABLE user_profiles ADD COLUMN gender text;
