-- Rollback: Affiliate Enhancements
DROP TABLE IF EXISTS affiliate_content_commissions;
DROP TABLE IF EXISTS content_affiliate_scan_results;
ALTER TABLE affiliate_providers DROP COLUMN IF EXISTS author_commission_pct;
