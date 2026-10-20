-- 000909_wiki_entity_content_seed.down.sql
-- Reverses 000909: removes the seeded cfv values, visa routes, and
-- embassies rows. Values are scoped by entity ids of the routes this
-- migration created; pre-existing values are untouched.

BEGIN;

DELETE FROM custom_field_values cfv
WHERE cfv.entity_type = 'visa_route'
  AND cfv.entity_id IN (
      SELECT id FROM visa_routes WHERE slug IN (
          'uk-skilled-worker-visa','uk-student-visa','uk-family-visa','uk-visitor-visa',
          'australia-student-visa','australia-work-visa','australia-pr-pathway',
          'canada-pr-pathway','germany-work-visa','germany-business-visa',
          'portugal-d7-visa','portugal-golden-visa','spain-digital-nomad-visa',
          'uae-business-visa','usa-b1-business-visa','usa-tourist-visa','ireland-family-visa',
          'canada-tourist-visa','chile-family-visa'
      )
  );

DELETE FROM embassies
WHERE slug IN (
    'us-embassy-abuja','uk-high-commission-abuja','canada-high-commission-abuja',
    'german-embassy-abuja','us-embassy-accra','uk-high-commission-accra',
    'us-embassy-nairobi','nigeria-high-commission-london'
);

DELETE FROM visa_routes
WHERE slug IN (
    'uk-skilled-worker-visa','uk-student-visa','uk-family-visa','uk-visitor-visa',
    'australia-student-visa','australia-work-visa','australia-pr-pathway',
    'canada-pr-pathway','germany-work-visa','germany-business-visa',
    'portugal-d7-visa','portugal-golden-visa','spain-digital-nomad-visa',
    'uae-business-visa','usa-b1-business-visa','usa-tourist-visa','ireland-family-visa'
);

COMMIT;
