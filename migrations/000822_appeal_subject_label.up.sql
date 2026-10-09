-- §22 appeals: snapshot the subject hook's human-readable label at submit
-- time so the linked §38 ticket subject, the appellant's /me/appeals list,
-- and the admin appeal queue show "Removed post — Visa Help" instead of a
-- raw entity UUID. Nullable — rows predating this migration keep NULL and
-- clients fall back to the entity_type translation.

ALTER TABLE moderation_appeals
    ADD COLUMN IF NOT EXISTS subject_label text;
