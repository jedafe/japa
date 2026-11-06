BEGIN;
DROP TRIGGER IF EXISTS trg_sync_product_review_stats ON contents;
DROP FUNCTION IF EXISTS sync_product_review_stats();
COMMIT;
