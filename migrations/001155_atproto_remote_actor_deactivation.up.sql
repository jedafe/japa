-- Ref: japaconnect-engineering/sections/127-atproto-native.md (A5 — Jetstream inbound)
-- Remote-actor lifecycle tracking: Jetstream `account` events carry
-- active:false when an upstream PDS deactivates or deletes a repo. We mark
-- the actor row so reads can distinguish a deactivated handle's historical
-- attribution; full tombstone propagation (unwinding their comments/likes)
-- is A6 scope.

ALTER TABLE atproto_remote_actors
    ADD COLUMN deactivated_at timestamptz;
