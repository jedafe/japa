-- 000827_discount_uses_counter.down.sql — drop the discount uses counter.
-- The discount_code_uses rows (audit trail) are left in place; only the
-- denormalized counter column is removed.

ALTER TABLE discount_codes DROP COLUMN IF EXISTS uses;
