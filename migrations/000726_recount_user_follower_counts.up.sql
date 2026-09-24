-- 000726_recount_user_follower_counts.up.sql
-- Reconcile users.follower_count with the follows table. The pre-fix
-- toggle-on-DELETE bug (POST/DELETE /users/{id}/follow both toggled state)
-- could create/delete follow rows on repeated calls, leaving the denormalized
-- counter out of sync with actual follow rows.

UPDATE users u
SET follower_count = COALESCE(c.n, 0)
FROM (
    SELECT entity_id, COUNT(*) AS n
    FROM follows
    WHERE entity_type = 'user'
    GROUP BY entity_id
) c
WHERE c.entity_id = u.id
  AND u.follower_count <> c.n;

UPDATE users u
SET follower_count = 0
WHERE u.follower_count <> 0
  AND NOT EXISTS (
    SELECT 1 FROM follows f
    WHERE f.entity_type = 'user' AND f.entity_id = u.id
  );
