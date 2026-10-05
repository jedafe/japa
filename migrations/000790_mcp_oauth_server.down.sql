-- 000790 down: drop the §120 OAuth authorization-server tables.
BEGIN;
DROP TABLE IF EXISTS public.oauth_tokens;
DROP TABLE IF EXISTS public.oauth_authorization_codes;
DROP TABLE IF EXISTS public.oauth_clients;
COMMIT;
