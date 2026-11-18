-- §97 / ADR-105 — Attestation Guide launch data.
-- 1) Domain constraints on document_type / status (audit BUG-008: both were
--    unconstrained text, so typos and UI-produced values could diverge).
-- 2) Seeds attestation_guide.hague_members with the Hague Apostille
--    Convention member list resolved to country space IDs (audit BUG-007:
--    the setting shipped as [] so the tool could never derive
--    apostille_required).
-- 3) Seeds a curated launch set of attestation pathways for high-traffic
--    diaspora corridors so the tool is not empty at launch.

-- ---------------------------------------------------------------------------
-- 1) Domain constraints (NOT VALID → VALIDATE keeps long table scans off the
--    write path on populated installs).
-- ---------------------------------------------------------------------------
ALTER TABLE public.attestation_pathways
    ADD CONSTRAINT attestation_pathways_document_type_check
    CHECK (document_type IN (
        'birth_certificate', 'degree', 'marriage',
        'police_clearance', 'professional_licence', 'other'
    )) NOT VALID;

ALTER TABLE public.attestation_pathways
    ADD CONSTRAINT attestation_pathways_status_check
    CHECK (status IN ('draft', 'ai_draft', 'pending', 'published', 'rejected')) NOT VALID;

ALTER TABLE public.attestation_community_updates
    ADD CONSTRAINT attestation_community_updates_status_check
    CHECK (status IN ('pending', 'approved', 'rejected')) NOT VALID;

ALTER TABLE public.attestation_pathways
    VALIDATE CONSTRAINT attestation_pathways_document_type_check;

ALTER TABLE public.attestation_pathways
    VALIDATE CONSTRAINT attestation_pathways_status_check;

ALTER TABLE public.attestation_community_updates
    VALIDATE CONSTRAINT attestation_community_updates_status_check;

-- ---------------------------------------------------------------------------
-- 2) Hague Apostille Convention members (ISO-2 geo_codes resolved to country
--    space IDs). Admins can amend via Control → Settings → attestation_guide.
-- ---------------------------------------------------------------------------
UPDATE public.system_settings
SET value = (
    SELECT COALESCE(jsonb_agg(id::text), '[]'::jsonb)
    FROM public.spaces
    WHERE level = 'country'
      AND deleted_at IS NULL
      AND geo_code = ANY (ARRAY[
        'AD','AG','AL','AM','AR','AT','AU','AZ','BA','BB','BD','BE','BG','BH',
        'BI','BN','BO','BR','BS','BW','BY','BZ','CA','CH','CK','CL','CN','CO',
        'CR','CV','CY','CZ','DE','DK','DM','DO','DZ','EC','EE','ES','FI','FJ',
        'FR','GB','GD','GE','GR','GT','GY','HK','HN','HR','HU','ID','IE','IL',
        'IN','IS','IT','JM','JP','KG','KN','KR','KZ','LC','LI','LR','LS','LT',
        'LU','LV','MA','MC','MD','ME','MH','MK','MN','MO','MT','MU','MW','MX',
        'NA','NI','NL','NO','NU','NZ','OM','PA','PE','PH','PK','PL','PT','PW',
        'PY','RO','RS','RU','RW','SA','SC','SE','SG','SI','SK','SM','SN','SR',
        'ST','SV','SZ','TH','TJ','TO','TT','TN','TR','UA','US','UY','UZ','VC',
        'VE','VU','WS','XK','ZA','ZM'
      ])
)
WHERE key = 'attestation_guide.hague_members';

-- ---------------------------------------------------------------------------
-- 3) Launch dataset — curated pathways for high-traffic corridors.
--    apostille_required is computed with the same both-sides Hague rule the
--    service uses at create/approve time.
--    pathway_steps shape: [{step, agency, what_to_bring, fee_estimate,
--    turnaround_days, url, note}] — the keys the UI renders.
-- ---------------------------------------------------------------------------
INSERT INTO public.attestation_pathways (
    document_type, issuing_space_id, destination_space_id,
    apostille_required, pathway_steps, community_last_verified,
    status, is_active
)
SELECT
    x.document_type,
    iss.id,
    ds.id,
    (iss.geo_code = ANY (hague.codes)) AND (ds.geo_code = ANY (hague.codes)),
    x.steps::jsonb,
    now()::date,
    'published',
    true
FROM (
    VALUES
        -- Nigeria → UAE (consular legalization; neither is Hague member)
        ('birth_certificate', 'NG', 'AE', '[
          {"step":"Notarize the document","agency":"Notary Public (Nigeria)","what_to_bring":"Original birth certificate + valid ID","fee_estimate":"₦5,000–₦15,000","turnaround_days":1,"note":"Must be a Supreme Court-notarized document for MFA acceptance"},
          {"step":"Federal Ministry of Foreign Affairs attestation","agency":"MFA Abuja — Legal Services Division","what_to_bring":"Notarized document + passport data page","fee_estimate":"₦3,000–₦10,000","turnaround_days":3,"note":"Standard service; express costs more"},
          {"step":"UAE Embassy legalization","agency":"UAE Embassy, Abuja","what_to_bring":"MFA-attested document + application form","fee_estimate":"≈₦45,000","turnaround_days":7,"url":"https://www.mofa.gov.ae"},
          {"step":"MOFA attestation on arrival (if required)","agency":"UAE Ministry of Foreign Affairs","what_to_bring":"Embassy-legalized document","fee_estimate":"AED 150–200","turnaround_days":2}
        ]'),
        -- Nigeria → UK (Nigeria is not a Hague member — MFA attestation route)
        ('degree', 'NG', 'GB', '[
          {"step":"Notarize the certificate","agency":"Notary Public (Nigeria)","what_to_bring":"Original degree certificate + ID","fee_estimate":"₦5,000–₦20,000","turnaround_days":1},
          {"step":"Federal Ministry of Foreign Affairs attestation","agency":"MFA Abuja","what_to_bring":"Notarized certificate","fee_estimate":"₦3,000–₦10,000","turnaround_days":3},
          {"step":"Submit to UK institution / UK ENIC if required","agency":"Receiving university, employer, or UK ENIC","what_to_bring":"MFA-attested certificate","fee_estimate":"£49.50 (ENIC, if needed)","turnaround_days":15,"url":"https://www.enic.org.uk","note":"UK is a Hague member but Nigeria is not — apostille is not available on Nigerian documents"}
        ]'),
        -- Nigeria → USA
        ('degree', 'NG', 'US', '[
          {"step":"Notarize the certificate","agency":"Notary Public (Nigeria)","what_to_bring":"Original degree certificate + ID","fee_estimate":"₦5,000–₦20,000","turnaround_days":1},
          {"step":"Federal Ministry of Foreign Affairs attestation","agency":"MFA Abuja","what_to_bring":"Notarized certificate","fee_estimate":"₦3,000–₦10,000","turnaround_days":3},
          {"step":"Credential evaluation (if required)","agency":"WES / ECE / receiving institution","what_to_bring":"MFA-attested certificate + transcripts","fee_estimate":"$100–$250","turnaround_days":20,"url":"https://www.wes.org","note":"Most US employers and schools accept MFA-attested documents directly or via evaluation — the US does not require embassy legalization for Nigerian documents"}
        ]'),
        -- Nigeria → Canada
        ('marriage', 'NG', 'CA', '[
          {"step":"Notarize the marriage certificate","agency":"Notary Public (Nigeria)","what_to_bring":"Original certificate + IDs of both spouses","fee_estimate":"₦5,000–₦15,000","turnaround_days":1},
          {"step":"Federal Ministry of Foreign Affairs attestation","agency":"MFA Abuja","what_to_bring":"Notarized certificate","fee_estimate":"₦3,000–₦10,000","turnaround_days":3},
          {"step":"Include in IRCC application","agency":"IRCC / receiving authority","what_to_bring":"MFA-attested certificate","fee_estimate":"—","turnaround_days":0,"note":"Canada is a Hague member but Nigeria is not — IRCC accepts MFA-attested Nigerian documents"}
        ]'),
        -- Nigeria police clearance → UAE
        ('police_clearance', 'NG', 'AE', '[
          {"step":"Obtain Police Clearance Certificate","agency":"Nigeria Police Force — CID, Force HQ Abuja or state commands","what_to_bring":"Passport data page + passport photos + fingerprints","fee_estimate":"₦5,000–₦40,000","turnaround_days":14},
          {"step":"Notarize the clearance","agency":"Notary Public (Nigeria)","what_to_bring":"Original PCC","fee_estimate":"₦3,000–₦10,000","turnaround_days":1},
          {"step":"Federal Ministry of Foreign Affairs attestation","agency":"MFA Abuja","what_to_bring":"Notarized PCC","fee_estimate":"₦3,000–₦10,000","turnaround_days":3},
          {"step":"UAE Embassy legalization","agency":"UAE Embassy, Abuja","what_to_bring":"MFA-attested PCC","fee_estimate":"≈₦45,000","turnaround_days":7,"url":"https://www.mofa.gov.ae"}
        ]'),
        -- Ghana → Germany (Ghana is not a Hague member)
        ('degree', 'GH', 'DE', '[
          {"step":"Notarize the certificate","agency":"Notary Public (Ghana)","what_to_bring":"Original certificate + Ghana Card/passport","fee_estimate":"GH₵100–300","turnaround_days":1},
          {"step":"Ministry of Foreign Affairs legalization","agency":"Ghana MFA — Legal & Consular Bureau, Accra","what_to_bring":"Notarized certificate","fee_estimate":"GH₵50–150","turnaround_days":5},
          {"step":"German Embassy verification (if required)","agency":"German Embassy, Accra","what_to_bring":"MFA-legalized certificate","fee_estimate":"€45","turnaround_days":30,"url":"https://accra.diplo.de","note":"German authorities may require formal document verification — check with the receiving institution first"}
        ]'),
        -- Kenya → UK (Kenya is not a Hague member)
        ('birth_certificate', 'KE', 'GB', '[
          {"step":"Obtain a certified copy from the Civil Registry","agency":"Civil Registration Services, Nairobi","what_to_bring":"Birth notification/original + ID","fee_estimate":"KSh 500–1,000","turnaround_days":7},
          {"step":"Notarize","agency":"Notary Public (Kenya)","what_to_bring":"Certified copy + ID","fee_estimate":"KSh 2,000–5,000","turnaround_days":1},
          {"step":"Ministry of Foreign Affairs attestation","agency":"Kenya MFA, Nairobi","what_to_bring":"Notarized document","fee_estimate":"KSh 1,000","turnaround_days":3,"note":"Kenya is not a Hague member — UK-bound documents use the MFA attestation route"}
        ]'),
        -- South Africa → UK (both Hague members — apostille route)
        ('degree', 'ZA', 'GB', '[
          {"step":"Obtain an apostille","agency":"DIRCO — Legalisation Section, Pretoria (or High Court for notarized docs)","what_to_bring":"Original degree certificate","fee_estimate":"Free (DIRCO)","turnaround_days":10,"url":"https://www.dirco.gov.za","note":"Both South Africa and the UK are Hague members — a single apostille suffices, no embassy step needed"}
        ]'),
        -- South Africa → Netherlands (both Hague members)
        ('police_clearance', 'ZA', 'NL', '[
          {"step":"Obtain Police Clearance Certificate","agency":"SAPS — Criminal Record Centre, Pretoria","what_to_bring":"Fingerprints + ID + application form","fee_estimate":"R160","turnaround_days":21,"url":"https://www.saps.gov.za"},
          {"step":"Obtain an apostille","agency":"DIRCO — Legalisation Section","what_to_bring":"Original PCC","fee_estimate":"Free","turnaround_days":10,"note":"Both countries are Hague members — apostille completes the legalization"}
        ]'),
        -- India → Germany (both Hague members)
        ('degree', 'IN', 'DE', '[
          {"step":"State-level attestation (HRD/Home Dept)","agency":"Issuing state HRD or Home Department","what_to_bring":"Original certificate","fee_estimate":"₹500–2,500","turnaround_days":10,"note":"Educational documents route via HRD; personal documents via Home Dept"},
          {"step":"MEA apostille","agency":"Ministry of External Affairs — via outsourced centers","what_to_bring":"State-attested certificate","fee_estimate":"₹50 + service fee","turnaround_days":3,"url":"https://www.mea.gov.in/apostille.htm","note":"Both India and Germany are Hague members"}
        ]'),
        -- Jamaica → UK (both Hague members)
        ('birth_certificate', 'JM', 'GB', '[
          {"step":"Obtain a certified copy","agency":"Registrar General''s Department (RGD), Jamaica","what_to_bring":"Application + ID","fee_estimate":"J$3,500–6,500","turnaround_days":7,"url":"https://www.rgd.gov.jm"},
          {"step":"Obtain an apostille","agency":"Ministry of Foreign Affairs & Foreign Trade, Kingston","what_to_bring":"Certified copy","fee_estimate":"J$3,000","turnaround_days":5,"note":"Both Jamaica and the UK are Hague members"}
        ]'),
        -- Colombia → Spain (both Hague members)
        ('marriage', 'CO', 'ES', '[
          {"step":"Obtain a certified registry copy","agency":"Registraduría Nacional del Estado Civil","what_to_bring":"Registry details + ID","fee_estimate":"COP 18,550","turnaround_days":5,"url":"https://www.registraduria.gov.co"},
          {"step":"Obtain an apostille","agency":"Ministerio de Relaciones Exteriores — online or in person","what_to_bring":"Certified copy","fee_estimate":"COP 35,100","turnaround_days":3,"url":"https://www.cancilleria.gov.co","note":"Both Colombia and Spain are Hague members"}
        ]')
) AS x(document_type, issuing_cc, dest_cc, steps)
JOIN public.spaces iss
    ON iss.level = 'country' AND iss.geo_code = x.issuing_cc AND iss.deleted_at IS NULL
JOIN public.spaces ds
    ON ds.level = 'country' AND ds.geo_code = x.dest_cc AND ds.deleted_at IS NULL
CROSS JOIN (
    SELECT ARRAY[
        'AD','AG','AL','AM','AR','AT','AU','AZ','BA','BB','BD','BE','BG','BH',
        'BI','BN','BO','BR','BS','BW','BY','BZ','CA','CH','CK','CL','CN','CO',
        'CR','CV','CY','CZ','DE','DK','DM','DO','DZ','EC','EE','ES','FI','FJ',
        'FR','GB','GD','GE','GR','GT','GY','HK','HN','HR','HU','ID','IE','IL',
        'IN','IS','IT','JM','JP','KG','KN','KR','KZ','LC','LI','LR','LS','LT',
        'LU','LV','MA','MC','MD','ME','MH','MK','MN','MO','MT','MU','MW','MX',
        'NA','NI','NL','NO','NU','NZ','OM','PA','PE','PH','PK','PL','PT','PW',
        'PY','RO','RS','RU','RW','SA','SC','SE','SG','SI','SK','SM','SN','SR',
        'ST','SV','SZ','TH','TJ','TO','TT','TN','TR','UA','US','UY','UZ','VC',
        'VE','VU','WS','XK','ZA','ZM'
    ] AS codes
) hague
ON CONFLICT DO NOTHING;
