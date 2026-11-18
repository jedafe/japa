-- Roll back 001198_attestation_ca_de_guides.
-- Removes only the 28 triples this migration seeds — the pre-existing
-- ('marriage','NG','CA') and ('degree','GH','DE') rows belong to 001197.

DELETE FROM public.attestation_pathways
WHERE status = 'published'
  AND NOT ai_generated
  AND community_last_verified IS NOT NULL
  AND (document_type, issuing_space_id, destination_space_id) IN (
      SELECT x.document_type, iss.id, ds.id
      FROM (
          VALUES
              ('birth_certificate', 'NG', 'DE'),
              ('degree',            'NG', 'DE'),
              ('marriage',          'NG', 'DE'),
              ('police_clearance',  'NG', 'DE'),
              ('professional_licence', 'NG', 'DE'),
              ('birth_certificate', 'NG', 'CA'),
              ('degree',            'NG', 'CA'),
              ('police_clearance',  'NG', 'CA'),
              ('professional_licence', 'NG', 'CA'),
              ('birth_certificate', 'GH', 'DE'),
              ('marriage',          'GH', 'DE'),
              ('police_clearance',  'GH', 'DE'),
              ('professional_licence', 'GH', 'DE'),
              ('birth_certificate', 'GH', 'CA'),
              ('degree',            'GH', 'CA'),
              ('marriage',          'GH', 'CA'),
              ('police_clearance',  'GH', 'CA'),
              ('professional_licence', 'GH', 'CA'),
              ('birth_certificate', 'ZA', 'DE'),
              ('degree',            'ZA', 'DE'),
              ('marriage',          'ZA', 'DE'),
              ('police_clearance',  'ZA', 'DE'),
              ('professional_licence', 'ZA', 'DE'),
              ('birth_certificate', 'ZA', 'CA'),
              ('degree',            'ZA', 'CA'),
              ('marriage',          'ZA', 'CA'),
              ('police_clearance',  'ZA', 'CA'),
              ('professional_licence', 'ZA', 'CA')
      ) AS x(document_type, issuing_cc, dest_cc)
      JOIN public.spaces iss
          ON iss.level = 'country' AND iss.geo_code = x.issuing_cc
      JOIN public.spaces ds
          ON ds.level = 'country' AND ds.geo_code = x.dest_cc
  );
