-- 000949_tips_idempotency_unique — §125 harden POST /tips replay safety.
-- The service-level sender+key lookup races under concurrent requests; the
-- partial unique index makes the second settle's INSERT fail instead of
-- double-settling (translated to a replay response in CreateTip).
CREATE UNIQUE INDEX IF NOT EXISTS tips_idem_unique
    ON tips (sender_user_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;
