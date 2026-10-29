-- §35 broadcasts: claim lease to close the send-vs-sweep double-dispatch
-- race. A claimant stamps claimed_at before inserting notifications; the
-- claim self-expires after 10 minutes so a crashed worker's lease can be
-- re-taken instead of wedging the row forever.
ALTER TABLE admin_broadcasts
    ADD COLUMN claimed_at timestamptz;
