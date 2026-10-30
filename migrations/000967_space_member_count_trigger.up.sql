-- Maintained member_count: keep spaces.member_count in sync with approved
-- space_members rows via trigger — O(1) per write, atomic with the write
-- transaction, and covers every write path (joins, approvals, leaves,
-- admin adds, GDPR erasure, bulk ops) with no handler cooperation needed.
-- Supersedes the IncrementSpaceCount recount (BUG-023): the trigger makes
-- drift impossible instead of recomputing it after the fact.
-- Pattern mirrors trg_update_active_mentee_count (000824).
-- Ref: japaconnect-engineering/sections/05-geo-first-space-system.md
CREATE OR REPLACE FUNCTION trg_update_space_member_count() RETURNS TRIGGER AS $$
BEGIN
  IF TG_OP = 'INSERT' AND NEW.status = 'approved' THEN
    UPDATE spaces SET member_count = member_count + 1 WHERE id = NEW.space_id;
  ELSIF TG_OP = 'DELETE' AND OLD.status = 'approved' THEN
    UPDATE spaces SET member_count = GREATEST(0, member_count - 1) WHERE id = OLD.space_id;
  ELSIF TG_OP = 'UPDATE' THEN
    IF OLD.space_id <> NEW.space_id THEN
      -- Row moved between spaces: decrement the old parent, increment the new.
      IF OLD.status = 'approved' THEN
        UPDATE spaces SET member_count = GREATEST(0, member_count - 1) WHERE id = OLD.space_id;
      END IF;
      IF NEW.status = 'approved' THEN
        UPDATE spaces SET member_count = member_count + 1 WHERE id = NEW.space_id;
      END IF;
    ELSIF OLD.status <> 'approved' AND NEW.status = 'approved' THEN
      UPDATE spaces SET member_count = member_count + 1 WHERE id = NEW.space_id;
    ELSIF OLD.status = 'approved' AND NEW.status <> 'approved' THEN
      UPDATE spaces SET member_count = GREATEST(0, member_count - 1) WHERE id = NEW.space_id;
    END IF;
  END IF;
  RETURN NULL;
END $$ LANGUAGE plpgsql;

CREATE TRIGGER trg_space_member_count
AFTER INSERT OR DELETE OR UPDATE OF status, space_id ON space_members
FOR EACH ROW EXECUTE FUNCTION trg_update_space_member_count();

-- One-time recount to heal drift accumulated before the trigger existed.
UPDATE spaces s SET member_count = (
  SELECT COUNT(*) FROM space_members sm
  WHERE sm.space_id = s.id AND sm.status = 'approved'
);
