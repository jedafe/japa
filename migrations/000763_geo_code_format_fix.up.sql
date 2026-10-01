-- Normalize the two baseline-seeded state spaces that carry the
-- country-prefixed ISO 3166-2 form ('GH-AA', 'KE-30') to the bare subdivision
-- code used by every other seeded state ('AF'/'AH'/'30'...). geo_code on
-- non-country spaces is display/filter metadata only — country-level lookups
-- constrain level='country' — but the equality filters in the space listings
-- treat it literally, so keeping the prefixed form makes these two states
-- unreachable via geo_code='AA'/'30' queries that work for their siblings.
UPDATE public.spaces SET geo_code = 'AA' WHERE slug = 'greater-accra' AND geo_code = 'GH-AA';
UPDATE public.spaces SET geo_code = '30' WHERE slug = 'nairobi-county' AND geo_code = 'KE-30';
