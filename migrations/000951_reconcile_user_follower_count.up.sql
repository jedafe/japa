-- BLK-003: reconcile users.follower_count with the follows table.
-- The §41 block path deletes follow rows directly (DeleteFollowsBetweenUsers)
-- and historically never decremented the denormalized counter, leaving
-- phantom followers. This re-derives every user's count from source rows;
-- SyncFollowerCountAfterBlock now keeps it correct on each new block.

-- Set exact counts for users who have at least one follower.
UPDATE users u
SET follower_count = sub.cnt
FROM (
  SELECT entity_id, COUNT(*)::int AS cnt
  FROM follows
  WHERE entity_type = 'user'
  GROUP BY entity_id
) sub
WHERE u.id = sub.entity_id
  AND u.follower_count <> sub.cnt;

-- Users whose follows were fully removed but whose count stayed non-zero.
UPDATE users u
SET follower_count = 0
WHERE u.follower_count <> 0
  AND NOT EXISTS (
    SELECT 1 FROM follows f
    WHERE f.entity_type = 'user' AND f.entity_id = u.id
  );
