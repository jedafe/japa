-- Add many-to-many junction table for space hierarchy
-- Custom spaces can belong to multiple primary spaces (country/state/city)

CREATE TABLE space_parent_spaces (
    space_id UUID NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    parent_space_id UUID NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (space_id, parent_space_id),
    CHECK (space_id != parent_space_id)
);

CREATE INDEX idx_space_parent_spaces_parent ON space_parent_spaces(parent_space_id);

-- Migrate existing single-parent data to junction table
INSERT INTO space_parent_spaces (space_id, parent_space_id, created_at)
SELECT id, parent_id, created_at
FROM spaces
WHERE parent_id IS NOT NULL
ON CONFLICT DO NOTHING;

-- Keep parent_id column for backward compatibility during transition
-- (can be dropped in a future migration after all code is updated)
