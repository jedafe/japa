-- Backfill one stub editorial guide post per published attestation pathway
-- (guide_category='attestation', field_values mirrors the pathway incl.
-- pathway_id so /guide/{slug} cross-links back to the live tool, and the
-- pathway detail page resolves the guide via field_values->>'pathway_id').
-- Pattern follows 000535_seed_roadmap_guides: first user as author,
-- content_hash per Rule 12, slug NOT EXISTS guard for idempotency.
--
-- Ref: japaconnect-engineering/sections/97-attestation-guide.md

CREATE EXTENSION IF NOT EXISTS pgcrypto;

DO $$
DECLARE
    v_author_id uuid;
BEGIN
    SELECT id INTO v_author_id
    FROM users
    ORDER BY created_at
    LIMIT 1;

    IF v_author_id IS NULL THEN
        RAISE EXCEPTION 'No user found to author attestation guide stubs';
    END IF;

    -- Re-activate soft-deleted stubs so up/down cycles stay idempotent.
    UPDATE contents c
    SET deleted_at = NULL,
        status = 'published',
        updated_at = now()
    FROM attestation_pathways p
    JOIN spaces si ON si.id = p.issuing_space_id
    JOIN spaces sd ON sd.id = p.destination_space_id
    WHERE c.guide_category = 'attestation'
      AND c.deleted_at IS NOT NULL
      AND si.geo_code IS NOT NULL
      AND sd.geo_code IS NOT NULL
      AND c.slug = 'attestation-' || replace(p.document_type, '_', '-')
                 || '-' || lower(si.geo_code) || '-' || lower(sd.geo_code);

    INSERT INTO contents (
        author_id,
        content_type,
        title,
        slug,
        body,
        body_html,
        field_values,
        content_hash,
        status,
        visibility,
        read_time_minutes,
        published_at,
        created_at,
        updated_at,
        seo_indexable,
        guide_category,
        meta_title,
        meta_description,
        source_country,
        target_country
    )
    SELECT
        v_author_id,
        'guide',
        g.title,
        g.slug,
        g.body,
        g.body_html,
        g.field_values,
        encode(digest(lower(regexp_replace(g.body, '\s+', ' ', 'g'))::text, 'sha256'), 'hex'),
        'published',
        'public',
        3,
        now(),
        now(),
        now(),
        true,
        'attestation',
        g.title,
        g.meta_description,
        g.issuing_geo,
        g.dest_geo
    FROM (
        SELECT
            doc.label || ' attestation: ' || si.name || ' → ' || sd.name AS title,
            'attestation-' || replace(p.document_type, '_', '-')
                || '-' || lower(si.geo_code) || '-' || lower(sd.geo_code) AS slug,
            'How to get a ' || lower(doc.label) || ' issued in ' || si.name
                || ' attested for use in ' || sd.name || '. '
                || CASE WHEN p.apostille_required
                    THEN 'Both ' || si.name || ' and ' || sd.name
                        || ' are members of the Hague Apostille Convention, so the document is authenticated with an apostille from the competent authority in '
                        || si.name || '.'
                    ELSE 'This corridor uses consular legalisation rather than an apostille — the document passes through notarisation, foreign-affairs authentication in '
                        || si.name || ', and final legalisation for use in ' || sd.name || '.'
                END
                || ' The verified pathway steps are listed below. Fees and turnaround times are community-verified estimates — confirm current requirements with the issuing institution before you begin.' AS body,
            '<p>'
                || replace(replace(replace(
                    'How to get a ' || lower(doc.label) || ' issued in ' || si.name
                    || ' attested for use in ' || sd.name || '. '
                    || CASE WHEN p.apostille_required
                        THEN 'Both ' || si.name || ' and ' || sd.name
                            || ' are members of the Hague Apostille Convention, so the document is authenticated with an apostille from the competent authority in '
                            || si.name || '.'
                        ELSE 'This corridor uses consular legalisation rather than an apostille — the document passes through notarisation, foreign-affairs authentication in '
                            || si.name || ', and final legalisation for use in ' || sd.name || '.'
                    END
                    || ' The verified pathway steps are listed below. Fees and turnaround times are community-verified estimates — confirm current requirements with the issuing institution before you begin.',
                    '&', '&amp;'), '<', '&lt;'), '>', '&gt;')
                || '</p>' AS body_html,
            jsonb_build_object(
                'document_type', p.document_type,
                'issuing_country', si.geo_code,
                'destination_country', sd.geo_code,
                'apostille_required', p.apostille_required,
                'pathway_steps', COALESCE((
                    SELECT jsonb_agg(
                        CASE WHEN elem ? 'url'
                            THEN (elem - 'url') || jsonb_build_object('official_url', elem->>'url')
                            ELSE elem
                        END
                    )
                    FROM jsonb_array_elements(p.pathway_steps) AS elem
                ), '[]'::jsonb),
                'last_verified', to_char(p.community_last_verified, 'YYYY-MM-DD'),
                'pathway_id', p.id::text
            ) AS field_values,
            'Steps, fees, and turnaround for getting a ' || lower(doc.label)
                || ' issued in ' || si.name || ' attested for use in '
                || sd.name || '.' AS meta_description,
            si.geo_code AS issuing_geo,
            sd.geo_code AS dest_geo
        FROM attestation_pathways p
        JOIN spaces si ON si.id = p.issuing_space_id
        JOIN spaces sd ON sd.id = p.destination_space_id
        CROSS JOIN LATERAL (
            SELECT CASE p.document_type
                WHEN 'birth_certificate' THEN 'Birth certificate'
                WHEN 'degree' THEN 'Degree'
                WHEN 'academic_transcript' THEN 'Academic transcript'
                WHEN 'marriage' THEN 'Marriage certificate'
                WHEN 'death_certificate' THEN 'Death certificate'
                WHEN 'divorce_certificate' THEN 'Divorce certificate'
                WHEN 'single_status' THEN 'Single status certificate'
                WHEN 'adoption_papers' THEN 'Adoption papers'
                WHEN 'police_clearance' THEN 'Police clearance certificate'
                WHEN 'professional_licence' THEN 'Professional licence'
                WHEN 'power_of_attorney' THEN 'Power of attorney'
                WHEN 'driving_licence' THEN 'Driving licence'
                WHEN 'employment_letter' THEN 'Employment letter'
                WHEN 'company_registration' THEN 'Company registration'
                WHEN 'medical_certificate' THEN 'Medical certificate'
                ELSE initcap(replace(p.document_type, '_', ' '))
            END AS label
        ) doc
        WHERE p.status = 'published'
          AND p.is_active
          AND si.geo_code IS NOT NULL
          AND sd.geo_code IS NOT NULL
    ) g
    WHERE NOT EXISTS (
        SELECT 1 FROM contents c
        WHERE c.slug = g.slug AND c.deleted_at IS NULL
    );
END $$;
