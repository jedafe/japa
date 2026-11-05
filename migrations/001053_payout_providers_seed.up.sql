-- SEC-W-NGN: seed the payout_providers registry so method creation and
-- provider listing are DB-driven (is_active / supported_currencies)
-- instead of the hardcoded AllowedWithdrawalProviders list.
INSERT INTO payout_providers (type, name, is_active, is_default, supported_currencies) VALUES
 ('paystack',      'Paystack (NGN bank)',  true, true,  ARRAY['NGN']),
 ('nowpayments',   'NOWPayments (crypto)', true, false, ARRAY['BTC','ETH','USDT','USDC','LTC','TRX','XLM','SOL','MATIC','BNB','DOGE']),
 ('circle',        'Circle (USDC)',        true, false, ARRAY['USDC']),
 ('bank_transfer', 'Bank transfer (manual)', true, false, '{}'),
 ('paypal',        'PayPal',               false, false, '{}'),
 ('flutterwave',   'Flutterwave',          false, false, '{}'),
 ('stripe',        'Stripe',               false, false, '{}'),
 ('manual',        'Manual payout',        true, false, '{}'),
 ('manual_crypto', 'Manual crypto payout', true, false, '{}')
ON CONFLICT (type) DO NOTHING;
