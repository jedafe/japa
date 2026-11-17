-- Roll back 001197_attestation_launch_data.

DELETE FROM public.attestation_pathways
WHERE status = 'published'
  AND NOT ai_generated
  AND community_last_verified IS NOT NULL
  AND (document_type, issuing_space_id, destination_space_id) IN (
      SELECT x.document_type, iss.id, ds.id
      FROM (
          VALUES
              ('birth_certificate', 'NG', 'AE'),
              ('degree',            'NG', 'GB'),
              ('degree',            'NG', 'US'),
              ('marriage',          'NG', 'CA'),
              ('police_clearance',  'NG', 'AE'),
              ('degree',            'GH', 'DE'),
              ('birth_certificate', 'KE', 'GB'),
              ('degree',            'ZA', 'GB'),
              ('police_clearance',  'ZA', 'NL'),
              ('degree',            'IN', 'DE'),
              ('birth_certificate', 'JM', 'GB'),
              ('marriage',          'CO', 'ES')
      ) AS x(document_type, issuing_cc, dest_cc)
      JOIN public.spaces iss
          ON iss.level = 'country' AND iss.geo_code = x.issuing_cc
      JOIN public.spaces ds
          ON ds.level = 'country' AND ds.geo_code = x.dest_cc
  );

UPDATE public.system_settings
SET value = '[]'::jsonb
WHERE key = 'attestation_guide.hague_members';

ALTER TABLE public.attestation_pathways
    DROP CONSTRAINT IF EXISTS attestation_pathways_document_type_check;

ALTER TABLE public.attestation_pathways
    DROP CONSTRAINT IF EXISTS attestation_pathways_status_check;

ALTER TABLE public.attestation_community_updates
    DROP CONSTRAINT IF EXISTS attestation_community_updates_status_check;
