-- Down: drops the uniqueness guard. The tag merge itself is irreversible —
-- retired variant rows cannot be reconstructed.
DROP INDEX IF EXISTS tags_slug_lower_key;
