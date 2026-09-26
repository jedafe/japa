-- Ref: .jeos/sections/08-forum-qa-solved.md (Tags Schema — slug uniqueness amendment)
-- Merge case-variant duplicate tags ('Career' vs 'career') into the canonical
-- lowercase-slug row, then enforce case-insensitive slug uniqueness.
--
-- The composer get-or-create path (GetOrCreateTagSlugs) historically inserted
-- user-typed strings verbatim as slugs, so a post tagged "Career" created a
-- second tag while 'career' already existed. Aggregated tag lists then emitted
-- the same name twice, which crashed keyed {#each} blocks on the frontend.
--
-- Canonical choice per LOWER(slug) group: the row whose slug is already
-- lowercase wins; ties break on oldest created_at. Runs as one DO block so the
-- scratch tables never enter the sqlc schema catalog.

DO $$
BEGIN
	CREATE TEMP TABLE _tag_canon AS
	SELECT DISTINCT ON (LOWER(slug)) id, slug
	FROM tags
	ORDER BY LOWER(slug), (slug = LOWER(slug)) DESC, created_at, id;

	CREATE TEMP TABLE _tag_dups AS
	SELECT t.id AS dup_id, t.slug AS dup_slug, c.id AS canon_id, c.slug AS canon_slug
	FROM tags t
	JOIN _tag_canon c ON LOWER(t.slug) = LOWER(c.slug)
	WHERE t.id <> c.id;

	-- Redirect map for the retired variant slugs (Section 128 slug_history).
	INSERT INTO slug_history (entity_type, entity_id, old_slug, new_slug)
	SELECT 'tag', canon_id, dup_slug, canon_slug FROM _tag_dups;

	-- content_tags: PK (content_id, tag_id) — drop rows that would collide
	-- after re-pointing (content tagged with both variants), then re-point.
	DELETE FROM content_tags ct
	USING _tag_dups d
	WHERE ct.tag_id = d.dup_id
	  AND EXISTS (
	    SELECT 1 FROM content_tags x
	    WHERE x.content_id = ct.content_id AND x.tag_id = d.canon_id
	  );

	UPDATE content_tags ct SET tag_id = d.canon_id FROM _tag_dups d WHERE ct.tag_id = d.dup_id;

	-- tag_moderators: UNIQUE (tag_id, user_id) — same collision-safe merge.
	DELETE FROM tag_moderators tm
	USING _tag_dups d
	WHERE tm.tag_id = d.dup_id
	  AND EXISTS (
	    SELECT 1 FROM tag_moderators x
	    WHERE x.tag_id = d.canon_id AND x.user_id = tm.user_id
	  );

	UPDATE tag_moderators tm SET tag_id = d.canon_id FROM _tag_dups d WHERE tm.tag_id = d.dup_id;

	-- Polymorphic follows (entity_type='tag'): UNIQUE (follower_id, entity_type, entity_id).
	DELETE FROM follows f
	USING _tag_dups d
	WHERE f.entity_type = 'tag' AND f.entity_id = d.dup_id
	  AND EXISTS (
	    SELECT 1 FROM follows x
	    WHERE x.entity_type = 'tag' AND x.entity_id = d.canon_id AND x.follower_id = f.follower_id
	  );

	UPDATE follows f SET entity_id = d.canon_id FROM _tag_dups d
	WHERE f.entity_type = 'tag' AND f.entity_id = d.dup_id;

	-- Polymorphic bookmarks (entity_type='tag'): UNIQUE (user_id, entity_type, entity_id).
	DELETE FROM bookmarks b
	USING _tag_dups d
	WHERE b.entity_type = 'tag' AND b.entity_id = d.dup_id
	  AND EXISTS (
	    SELECT 1 FROM bookmarks x
	    WHERE x.entity_type = 'tag' AND x.entity_id = d.canon_id AND x.user_id = b.user_id
	  );

	UPDATE bookmarks b SET entity_id = d.canon_id FROM _tag_dups d
	WHERE b.entity_type = 'tag' AND b.entity_id = d.dup_id;

	-- Polymorphic reactions (entity_type='tag'):
	-- PK (entity_type, entity_id, user_id, reaction_key).
	DELETE FROM reactions r
	USING _tag_dups d
	WHERE r.entity_type = 'tag' AND r.entity_id = d.dup_id
	  AND EXISTS (
	    SELECT 1 FROM reactions x
	    WHERE x.entity_type = 'tag' AND x.entity_id = d.canon_id
	      AND x.user_id = r.user_id AND x.reaction_key = r.reaction_key
	  );

	UPDATE reactions r SET entity_id = d.canon_id FROM _tag_dups d
	WHERE r.entity_type = 'tag' AND r.entity_id = d.dup_id;

	-- Retire the variant rows.
	DELETE FROM tags t USING _tag_dups d WHERE t.id = d.dup_id;

	-- Normalize any remaining non-lowercase slugs (rows that had no lowercase
	-- sibling but still carried upper-case characters).
	UPDATE tags SET slug = LOWER(slug), updated_at = now() WHERE slug <> LOWER(slug);

	-- Recompute denormalized counters on canonical rows that gained relations.
	UPDATE tags t SET
	  content_count = (SELECT COUNT(*) FROM content_tags ct WHERE ct.tag_id = t.id),
	  follower_count = (SELECT COUNT(*) FROM follows f WHERE f.entity_type = 'tag' AND f.entity_id = t.id)
	WHERE EXISTS (SELECT 1 FROM _tag_dups d WHERE d.canon_id = t.id);

	DROP TABLE _tag_dups;
	DROP TABLE _tag_canon;
END $$;

-- Enforce case-insensitive slug uniqueness going forward.
CREATE UNIQUE INDEX tags_slug_lower_key ON tags (LOWER(slug));
