-- §97 / ADR-105 — Attestation Guide: Canada + Germany destination dataset.
--
-- Seeds community-verified pathways for the Canada and Germany destination
-- corridors across Nigeria, Ghana, and South Africa issuing countries —
-- every document type except 'other' (no canonical chain exists for it).
--
-- Coverage (28 rows):
--   NG → DE (5), NG → CA (4), GH → DE (4), GH → CA (5), ZA → DE (5), ZA → CA (5)
--   NG→CA marriage and GH→DE degree already exist from
--   001197_attestation_launch_data — not re-seeded here.
--
-- Route shapes mirror the real corridors:
--   * NG/GH are not Hague members → notary → home MFA attestation →
--     destination mission legalisation (DE) or direct submission to the
--     receiving Canadian authority (CA accepts foreign public docs without
--     consular legalisation for non-member origins — verify per institution).
--   * ZA is a Hague member, as are CA and DE → a single DIRCO/High Court
--     apostille completes the chain.
--
-- apostille_required is computed with the same both-sides Hague rule the
-- service uses at create/approve time — the code list is duplicated here so
-- the migration is self-contained (must stay in sync with 001197's array).
-- pathway_steps shape: [{step, agency, what_to_bring, fee_estimate,
-- turnaround_days, url, note}] — the keys the UI renders.

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
        -- ================= Nigeria → Germany (legalisation chain) =================
        ('birth_certificate', 'NG', 'DE', '[
          {"step":"Obtain an NPC Attestation of Birth (if no birth certificate exists)","agency":"National Population Commission — any NPC office or portal","what_to_bring":"Birth notification/declaration of age affidavit + ID + passport photos","fee_estimate":"₦5,000–₦25,000","turnaround_days":7,"url":"https://nationalpopulation.gov.ng"},
          {"step":"Notarize the document","agency":"Notary Public (Nigeria)","what_to_bring":"Original birth certificate/attestation + valid ID","fee_estimate":"₦5,000–₦15,000","turnaround_days":1},
          {"step":"Federal Ministry of Foreign Affairs attestation","agency":"MFA Abuja — Legal Services Division","what_to_bring":"Notarized document + passport data page","fee_estimate":"₦3,000–₦10,000","turnaround_days":3},
          {"step":"German mission legalisation","agency":"German Embassy Abuja or Consulate-General Lagos","what_to_bring":"MFA-attested document + appointment + application form","fee_estimate":"≈€30 per document (payable in naira)","turnaround_days":14,"url":"https://abuja.diplo.de","note":"Nigeria is not a Hague member — German authorities require consular legalisation, not an apostille. Book the embassy appointment early; slots are scarce."}
        ]'),
        ('degree', 'NG', 'DE', '[
          {"step":"Notarize the certificate","agency":"Notary Public (Nigeria)","what_to_bring":"Original degree certificate + ID","fee_estimate":"₦5,000–₦20,000","turnaround_days":1},
          {"step":"Federal Ministry of Foreign Affairs attestation","agency":"MFA Abuja","what_to_bring":"Notarized certificate","fee_estimate":"₦3,000–₦10,000","turnaround_days":3},
          {"step":"German mission legalisation","agency":"German Embassy Abuja or Consulate-General Lagos","what_to_bring":"MFA-attested certificate + appointment","fee_estimate":"≈€30 per document (payable in naira)","turnaround_days":14,"url":"https://abuja.diplo.de","note":"For university admission, Germany may separately require APS verification of academic credentials — check with the receiving institution."}
        ]'),
        ('marriage', 'NG', 'DE', '[
          {"step":"Obtain a certified marriage certificate","agency":"Marriage registry / local government or church registry that conducted the marriage","what_to_bring":"Marriage certificate + IDs of both spouses","fee_estimate":"₦5,000–₦20,000","turnaround_days":7},
          {"step":"Notarize","agency":"Notary Public (Nigeria)","what_to_bring":"Original certificate","fee_estimate":"₦3,000–₦10,000","turnaround_days":1},
          {"step":"Federal Ministry of Foreign Affairs attestation","agency":"MFA Abuja","what_to_bring":"Notarized certificate","fee_estimate":"₦3,000–₦10,000","turnaround_days":3},
          {"step":"German mission legalisation","agency":"German Embassy Abuja or Consulate-General Lagos","what_to_bring":"MFA-attested certificate + appointment","fee_estimate":"≈€30 per document (payable in naira)","turnaround_days":14,"url":"https://abuja.diplo.de"}
        ]'),
        ('police_clearance', 'NG', 'DE', '[
          {"step":"Obtain Police Clearance Certificate","agency":"Nigeria Police Force — CID, Force HQ Abuja or state commands","what_to_bring":"Passport data page + passport photos + fingerprints","fee_estimate":"₦5,000–₦40,000","turnaround_days":14,"url":"https://www.npf.gov.ng"},
          {"step":"Notarize the clearance","agency":"Notary Public (Nigeria)","what_to_bring":"Original PCC","fee_estimate":"₦3,000–₦10,000","turnaround_days":1},
          {"step":"Federal Ministry of Foreign Affairs attestation","agency":"MFA Abuja","what_to_bring":"Notarized PCC","fee_estimate":"₦3,000–₦10,000","turnaround_days":3},
          {"step":"German mission legalisation","agency":"German Embassy Abuja or Consulate-General Lagos","what_to_bring":"MFA-attested PCC + appointment","fee_estimate":"≈€30 per document (payable in naira)","turnaround_days":14,"url":"https://abuja.diplo.de"}
        ]'),
        ('professional_licence', 'NG', 'DE', '[
          {"step":"Obtain a verification/good-standing letter","agency":"Issuing council (MDCN, COREN, NMCN, PCN, etc.)","what_to_bring":"Licence number + membership ID + request letter","fee_estimate":"₦10,000–₦50,000","turnaround_days":14},
          {"step":"Notarize the licence and letter","agency":"Notary Public (Nigeria)","what_to_bring":"Original licence + council letter","fee_estimate":"₦5,000–₦15,000","turnaround_days":1},
          {"step":"Federal Ministry of Foreign Affairs attestation","agency":"MFA Abuja","what_to_bring":"Notarized documents","fee_estimate":"₦3,000–₦10,000","turnaround_days":3},
          {"step":"German mission legalisation","agency":"German Embassy Abuja or Consulate-General Lagos","what_to_bring":"MFA-attested documents + appointment","fee_estimate":"≈€30 per document (payable in naira)","turnaround_days":14,"url":"https://abuja.diplo.de","note":"German recognition (Anerkennung) of the licence itself is a separate process through the competent authority — legalisation only certifies the document."}
        ]'),
        -- ================= Nigeria → Canada (authentication route) =================
        ('birth_certificate', 'NG', 'CA', '[
          {"step":"Obtain an NPC Attestation of Birth (if no birth certificate exists)","agency":"National Population Commission — any NPC office or portal","what_to_bring":"Birth notification/declaration of age affidavit + ID + passport photos","fee_estimate":"₦5,000–₦25,000","turnaround_days":7,"url":"https://nationalpopulation.gov.ng"},
          {"step":"Notarize the document","agency":"Notary Public (Nigeria)","what_to_bring":"Original birth certificate/attestation + valid ID","fee_estimate":"₦5,000–₦15,000","turnaround_days":1},
          {"step":"Federal Ministry of Foreign Affairs attestation","agency":"MFA Abuja — Legal Services Division","what_to_bring":"Notarized document + passport data page","fee_estimate":"₦3,000–₦10,000","turnaround_days":3},
          {"step":"Submit to the receiving Canadian authority","agency":"IRCC / provincial registry / receiving institution","what_to_bring":"MFA-attested document","fee_estimate":"—","turnaround_days":0,"url":"https://www.canada.ca","note":"Canada accepts apostilles since January 2024 but Nigeria cannot issue them — IRCC accepts MFA-authenticated Nigerian documents. Confirm with the receiving institution."}
        ]'),
        ('degree', 'NG', 'CA', '[
          {"step":"Notarize the certificate","agency":"Notary Public (Nigeria)","what_to_bring":"Original degree certificate + ID","fee_estimate":"₦5,000–₦20,000","turnaround_days":1},
          {"step":"Federal Ministry of Foreign Affairs attestation","agency":"MFA Abuja","what_to_bring":"Notarized certificate","fee_estimate":"₦3,000–₦10,000","turnaround_days":3},
          {"step":"Educational Credential Assessment (for immigration)","agency":"WES Canada / IRCC-designated ECA body","what_to_bring":"MFA-attested certificate + sealed transcripts","fee_estimate":"CA$200–$260","turnaround_days":30,"url":"https://www.wes.org","note":"An ECA is required for Express Entry; employers may accept the attested certificate directly."}
        ]'),
        ('police_clearance', 'NG', 'CA', '[
          {"step":"Obtain Police Clearance Certificate","agency":"Nigeria Police Force — CID, Force HQ Abuja or state commands","what_to_bring":"Passport data page + passport photos + fingerprints","fee_estimate":"₦5,000–₦40,000","turnaround_days":14,"url":"https://www.npf.gov.ng"},
          {"step":"Notarize the clearance","agency":"Notary Public (Nigeria)","what_to_bring":"Original PCC","fee_estimate":"₦3,000–₦10,000","turnaround_days":1},
          {"step":"Federal Ministry of Foreign Affairs attestation","agency":"MFA Abuja","what_to_bring":"Notarized PCC","fee_estimate":"₦3,000–₦10,000","turnaround_days":3},
          {"step":"Include in IRCC application","agency":"IRCC / receiving authority","what_to_bring":"MFA-attested PCC","fee_estimate":"—","turnaround_days":0,"url":"https://www.canada.ca","note":"PCCs are time-sensitive for immigration — IRCC generally requires one issued within the last 6 months."}
        ]'),
        ('professional_licence', 'NG', 'CA', '[
          {"step":"Obtain a verification/good-standing letter","agency":"Issuing council (MDCN, COREN, NMCN, PCN, etc.)","what_to_bring":"Licence number + membership ID + request letter","fee_estimate":"₦10,000–₦50,000","turnaround_days":14},
          {"step":"Notarize the licence and letter","agency":"Notary Public (Nigeria)","what_to_bring":"Original licence + council letter","fee_estimate":"₦5,000–₦15,000","turnaround_days":1},
          {"step":"Federal Ministry of Foreign Affairs attestation","agency":"MFA Abuja","what_to_bring":"Notarized documents","fee_estimate":"₦3,000–₦10,000","turnaround_days":3},
          {"step":"Submit to the provincial regulator","agency":"Provincial licensing body (e.g. provincial nursing/engineering college)","what_to_bring":"MFA-attested documents","fee_estimate":"—","turnaround_days":0,"note":"Licensing itself is province-specific — the attestation only certifies the document for the regulator review."}
        ]'),
        -- ================= Ghana → Germany (legalisation chain) =================
        ('birth_certificate', 'GH', 'DE', '[
          {"step":"Obtain a certified birth certificate","agency":"Births and Deaths Registry, Ghana","what_to_bring":"Birth registration details + Ghana Card","fee_estimate":"GH₵50–150","turnaround_days":7},
          {"step":"Notarize","agency":"Notary Public (Ghana)","what_to_bring":"Original certificate + Ghana Card/passport","fee_estimate":"GH₵100–300","turnaround_days":1},
          {"step":"Ministry of Foreign Affairs legalization","agency":"Ghana MFA — Legal & Consular Bureau, Accra","what_to_bring":"Notarized certificate","fee_estimate":"GH₵50–150","turnaround_days":5,"url":"https://mfa.gov.gh"},
          {"step":"German Embassy legalisation","agency":"German Embassy, Accra","what_to_bring":"MFA-legalized certificate + appointment","fee_estimate":"≈€30 per document","turnaround_days":14,"url":"https://accra.diplo.de","note":"Ghana is not a Hague member — consular legalisation, not apostille."}
        ]'),
        ('marriage', 'GH', 'DE', '[
          {"step":"Obtain a certified marriage certificate","agency":"Registrar General''s Department / district marriage registry","what_to_bring":"Marriage certificate + IDs of both spouses","fee_estimate":"GH₵50–200","turnaround_days":7},
          {"step":"Notarize","agency":"Notary Public (Ghana)","what_to_bring":"Original certificate","fee_estimate":"GH₵100–300","turnaround_days":1},
          {"step":"Ministry of Foreign Affairs legalization","agency":"Ghana MFA — Legal & Consular Bureau, Accra","what_to_bring":"Notarized certificate","fee_estimate":"GH₵50–150","turnaround_days":5,"url":"https://mfa.gov.gh"},
          {"step":"German Embassy legalisation","agency":"German Embassy, Accra","what_to_bring":"MFA-legalized certificate + appointment","fee_estimate":"≈€30 per document","turnaround_days":14,"url":"https://accra.diplo.de"}
        ]'),
        ('police_clearance', 'GH', 'DE', '[
          {"step":"Obtain Police Clearance Certificate","agency":"Ghana Police Service — CID Headquarters, Accra","what_to_bring":"Passport/Ghana Card + fingerprints + photos","fee_estimate":"GH₵100–300","turnaround_days":10,"url":"https://police.gov.gh"},
          {"step":"Notarize","agency":"Notary Public (Ghana)","what_to_bring":"Original PCC","fee_estimate":"GH₵100–300","turnaround_days":1},
          {"step":"Ministry of Foreign Affairs legalization","agency":"Ghana MFA — Legal & Consular Bureau, Accra","what_to_bring":"Notarized PCC","fee_estimate":"GH₵50–150","turnaround_days":5,"url":"https://mfa.gov.gh"},
          {"step":"German Embassy legalisation","agency":"German Embassy, Accra","what_to_bring":"MFA-legalized PCC + appointment","fee_estimate":"≈€30 per document","turnaround_days":14,"url":"https://accra.diplo.de"}
        ]'),
        ('professional_licence', 'GH', 'DE', '[
          {"step":"Obtain a verification/good-standing letter","agency":"Issuing council (N&MC, GEDC, Pharmacy Council, etc.)","what_to_bring":"Licence number + PIN + request letter","fee_estimate":"GH₵200–800","turnaround_days":14},
          {"step":"Notarize the licence and letter","agency":"Notary Public (Ghana)","what_to_bring":"Original licence + council letter","fee_estimate":"GH₵100–300","turnaround_days":1},
          {"step":"Ministry of Foreign Affairs legalization","agency":"Ghana MFA — Legal & Consular Bureau, Accra","what_to_bring":"Notarized documents","fee_estimate":"GH₵50–150","turnaround_days":5,"url":"https://mfa.gov.gh"},
          {"step":"German Embassy legalisation","agency":"German Embassy, Accra","what_to_bring":"MFA-legalized documents + appointment","fee_estimate":"≈€30 per document","turnaround_days":14,"url":"https://accra.diplo.de","note":"German Anerkennung of the qualification is a separate process — legalisation only certifies the document."}
        ]'),
        -- ================= Ghana → Canada (authentication route) =================
        ('birth_certificate', 'GH', 'CA', '[
          {"step":"Obtain a certified birth certificate","agency":"Births and Deaths Registry, Ghana","what_to_bring":"Birth registration details + Ghana Card","fee_estimate":"GH₵50–150","turnaround_days":7},
          {"step":"Notarize","agency":"Notary Public (Ghana)","what_to_bring":"Original certificate + Ghana Card/passport","fee_estimate":"GH₵100–300","turnaround_days":1},
          {"step":"Ministry of Foreign Affairs legalization","agency":"Ghana MFA — Legal & Consular Bureau, Accra","what_to_bring":"Notarized certificate","fee_estimate":"GH₵50–150","turnaround_days":5,"url":"https://mfa.gov.gh"},
          {"step":"Submit to the receiving Canadian authority","agency":"IRCC / provincial registry / receiving institution","what_to_bring":"MFA-legalized certificate","fee_estimate":"—","turnaround_days":0,"url":"https://www.canada.ca","note":"Canada accepts apostilles since January 2024 but Ghana cannot issue them — Canadian authorities accept Ghana MFA-authenticated documents. Confirm with the receiving institution."}
        ]'),
        ('degree', 'GH', 'CA', '[
          {"step":"Notarize the certificate","agency":"Notary Public (Ghana)","what_to_bring":"Original degree certificate + Ghana Card/passport","fee_estimate":"GH₵100–300","turnaround_days":1},
          {"step":"Ministry of Foreign Affairs legalization","agency":"Ghana MFA — Legal & Consular Bureau, Accra","what_to_bring":"Notarized certificate","fee_estimate":"GH₵50–150","turnaround_days":5,"url":"https://mfa.gov.gh"},
          {"step":"Educational Credential Assessment (for immigration)","agency":"WES Canada / IRCC-designated ECA body","what_to_bring":"MFA-legalized certificate + sealed transcripts","fee_estimate":"CA$200–$260","turnaround_days":30,"url":"https://www.wes.org","note":"An ECA is required for Express Entry; employers may accept the attested certificate directly."}
        ]'),
        ('marriage', 'GH', 'CA', '[
          {"step":"Obtain a certified marriage certificate","agency":"Registrar General''s Department / district marriage registry","what_to_bring":"Marriage certificate + IDs of both spouses","fee_estimate":"GH₵50–200","turnaround_days":7},
          {"step":"Notarize","agency":"Notary Public (Ghana)","what_to_bring":"Original certificate","fee_estimate":"GH₵100–300","turnaround_days":1},
          {"step":"Ministry of Foreign Affairs legalization","agency":"Ghana MFA — Legal & Consular Bureau, Accra","what_to_bring":"Notarized certificate","fee_estimate":"GH₵50–150","turnaround_days":5,"url":"https://mfa.gov.gh"},
          {"step":"Include in IRCC application","agency":"IRCC / receiving authority","what_to_bring":"MFA-legalized certificate","fee_estimate":"—","turnaround_days":0,"url":"https://www.canada.ca"}
        ]'),
        ('police_clearance', 'GH', 'CA', '[
          {"step":"Obtain Police Clearance Certificate","agency":"Ghana Police Service — CID Headquarters, Accra","what_to_bring":"Passport/Ghana Card + fingerprints + photos","fee_estimate":"GH₵100–300","turnaround_days":10,"url":"https://police.gov.gh"},
          {"step":"Notarize","agency":"Notary Public (Ghana)","what_to_bring":"Original PCC","fee_estimate":"GH₵100–300","turnaround_days":1},
          {"step":"Ministry of Foreign Affairs legalization","agency":"Ghana MFA — Legal & Consular Bureau, Accra","what_to_bring":"Notarized PCC","fee_estimate":"GH₵50–150","turnaround_days":5,"url":"https://mfa.gov.gh"},
          {"step":"Include in IRCC application","agency":"IRCC / receiving authority","what_to_bring":"MFA-legalized PCC","fee_estimate":"—","turnaround_days":0,"url":"https://www.canada.ca","note":"PCCs are time-sensitive — IRCC generally requires one issued within the last 6 months."}
        ]'),
        ('professional_licence', 'GH', 'CA', '[
          {"step":"Obtain a verification/good-standing letter","agency":"Issuing council (N&MC, GEDC, Pharmacy Council, etc.)","what_to_bring":"Licence number + PIN + request letter","fee_estimate":"GH₵200–800","turnaround_days":14},
          {"step":"Notarize the licence and letter","agency":"Notary Public (Ghana)","what_to_bring":"Original licence + council letter","fee_estimate":"GH₵100–300","turnaround_days":1},
          {"step":"Ministry of Foreign Affairs legalization","agency":"Ghana MFA — Legal & Consular Bureau, Accra","what_to_bring":"Notarized documents","fee_estimate":"GH₵50–150","turnaround_days":5,"url":"https://mfa.gov.gh"},
          {"step":"Submit to the provincial regulator","agency":"Provincial licensing body","what_to_bring":"MFA-legalized documents","fee_estimate":"—","turnaround_days":0,"note":"Licensing is province-specific — the attestation only certifies the document for the regulator review."}
        ]'),
        -- ============ South Africa → Germany (apostille route) ============
        ('birth_certificate', 'ZA', 'DE', '[
          {"step":"Obtain an unabridged birth certificate","agency":"Department of Home Affairs — any office or eHomeAffairs","what_to_bring":"ID + birth registration details","fee_estimate":"R75","turnaround_days":45,"url":"https://www.dha.gov.za","note":"German authorities typically require the unabridged (vault) certificate, not the abridged computer printout — apply early, DHA backlogs are long."},
          {"step":"Obtain an apostille","agency":"DIRCO — Legalisation Section, Pretoria","what_to_bring":"Original unabridged certificate","fee_estimate":"Free","turnaround_days":10,"url":"https://www.dirco.gov.za","note":"Both South Africa and Germany are Hague members — the apostille completes legalization, no embassy step needed."}
        ]'),
        ('degree', 'ZA', 'DE', '[
          {"step":"Obtain a certified copy of the qualification","agency":"Issuing university (or SAQA for foreign-held credentials)","what_to_bring":"Original degree + ID","fee_estimate":"R0–R500","turnaround_days":7},
          {"step":"Obtain an apostille","agency":"DIRCO — Legalisation Section, Pretoria (or High Court registrar for notarized copies)","what_to_bring":"Original or notarized certificate","fee_estimate":"Free","turnaround_days":10,"url":"https://www.dirco.gov.za","note":"Both countries are Hague members. For university admission, Germany may separately require recognition via uni-assist/ZAB — check with the institution."}
        ]'),
        ('marriage', 'ZA', 'DE', '[
          {"step":"Obtain an unabridged marriage certificate","agency":"Department of Home Affairs","what_to_bring":"ID + marriage register details (BI-30 details help)","fee_estimate":"R75","turnaround_days":60,"url":"https://www.dha.gov.za","note":"Germany requires the unabridged certificate — DHA processing can take 8–12 weeks, apply early."},
          {"step":"Obtain an apostille","agency":"DIRCO — Legalisation Section, Pretoria","what_to_bring":"Original unabridged certificate","fee_estimate":"Free","turnaround_days":10,"url":"https://www.dirco.gov.za"}
        ]'),
        ('police_clearance', 'ZA', 'DE', '[
          {"step":"Obtain Police Clearance Certificate","agency":"SAPS — Criminal Record and Crime Scene Management, Pretoria","what_to_bring":"Fingerprints + ID + application form","fee_estimate":"R160","turnaround_days":21,"url":"https://www.saps.gov.za"},
          {"step":"Obtain an apostille","agency":"DIRCO — Legalisation Section, Pretoria","what_to_bring":"Original PCC","fee_estimate":"Free","turnaround_days":10,"url":"https://www.dirco.gov.za","note":"Both countries are Hague members — apostille completes the legalization."}
        ]'),
        ('professional_licence', 'ZA', 'DE', '[
          {"step":"Obtain a verification/good-standing letter","agency":"Issuing council (HPCSA, SANC, ECSA, SAQA-recognized bodies)","what_to_bring":"Registration number + ID + request letter","fee_estimate":"R200–R1,500","turnaround_days":14},
          {"step":"Notarize the licence and letter","agency":"Notary Public (South Africa)","what_to_bring":"Original licence + council letter","fee_estimate":"R500–R1,500","turnaround_days":2},
          {"step":"Obtain an apostille","agency":"High Court registrar (notarized docs) or DIRCO — Legalisation Section","what_to_bring":"Notarized/original documents","fee_estimate":"Free","turnaround_days":10,"url":"https://www.dirco.gov.za","note":"German Anerkennung of the qualification is a separate process — the apostille only certifies the document."}
        ]'),
        -- ============ South Africa → Canada (apostille route) ============
        ('birth_certificate', 'ZA', 'CA', '[
          {"step":"Obtain an unabridged birth certificate","agency":"Department of Home Affairs — any office or eHomeAffairs","what_to_bring":"ID + birth registration details","fee_estimate":"R75","turnaround_days":45,"url":"https://www.dha.gov.za","note":"Canadian authorities may ask for the unabridged certificate — apply early, DHA backlogs are long."},
          {"step":"Obtain an apostille","agency":"DIRCO — Legalisation Section, Pretoria","what_to_bring":"Original unabridged certificate","fee_estimate":"Free","turnaround_days":10,"url":"https://www.dirco.gov.za","note":"Both South Africa and Canada are Hague members (Canada since January 2024) — the apostille completes legalization."}
        ]'),
        ('degree', 'ZA', 'CA', '[
          {"step":"Obtain a certified copy of the qualification","agency":"Issuing university","what_to_bring":"Original degree + ID","fee_estimate":"R0–R500","turnaround_days":7},
          {"step":"Obtain an apostille","agency":"DIRCO — Legalisation Section, Pretoria (or High Court registrar for notarized copies)","what_to_bring":"Original or notarized certificate","fee_estimate":"Free","turnaround_days":10,"url":"https://www.dirco.gov.za"},
          {"step":"Educational Credential Assessment (for immigration)","agency":"WES Canada / IRCC-designated ECA body","what_to_bring":"Apostilled certificate + sealed transcripts","fee_estimate":"CA$200–$260","turnaround_days":30,"url":"https://www.wes.org","note":"An ECA is required for Express Entry — order it in parallel with the apostille to save time."}
        ]'),
        ('marriage', 'ZA', 'CA', '[
          {"step":"Obtain an unabridged marriage certificate","agency":"Department of Home Affairs","what_to_bring":"ID + marriage register details","fee_estimate":"R75","turnaround_days":60,"url":"https://www.dha.gov.za","note":"DHA processing can take 8–12 weeks — apply early."},
          {"step":"Obtain an apostille","agency":"DIRCO — Legalisation Section, Pretoria","what_to_bring":"Original unabridged certificate","fee_estimate":"Free","turnaround_days":10,"url":"https://www.dirco.gov.za"},
          {"step":"Include in IRCC application","agency":"IRCC / receiving authority","what_to_bring":"Apostilled certificate","fee_estimate":"—","turnaround_days":0,"url":"https://www.canada.ca"}
        ]'),
        ('police_clearance', 'ZA', 'CA', '[
          {"step":"Obtain Police Clearance Certificate","agency":"SAPS — Criminal Record and Crime Scene Management, Pretoria","what_to_bring":"Fingerprints + ID + application form","fee_estimate":"R160","turnaround_days":21,"url":"https://www.saps.gov.za"},
          {"step":"Obtain an apostille","agency":"DIRCO — Legalisation Section, Pretoria","what_to_bring":"Original PCC","fee_estimate":"Free","turnaround_days":10,"url":"https://www.dirco.gov.za","note":"Both countries are Hague members (Canada since January 2024). PCCs are time-sensitive — IRCC generally requires one issued within the last 6 months."}
        ]'),
        ('professional_licence', 'ZA', 'CA', '[
          {"step":"Obtain a verification/good-standing letter","agency":"Issuing council (HPCSA, SANC, ECSA, SAQA-recognized bodies)","what_to_bring":"Registration number + ID + request letter","fee_estimate":"R200–R1,500","turnaround_days":14},
          {"step":"Notarize the licence and letter","agency":"Notary Public (South Africa)","what_to_bring":"Original licence + council letter","fee_estimate":"R500–R1,500","turnaround_days":2},
          {"step":"Obtain an apostille","agency":"High Court registrar (notarized docs) or DIRCO — Legalisation Section","what_to_bring":"Notarized/original documents","fee_estimate":"Free","turnaround_days":10,"url":"https://www.dirco.gov.za"},
          {"step":"Submit to the provincial regulator","agency":"Provincial licensing body","what_to_bring":"Apostilled documents","fee_estimate":"—","turnaround_days":0,"note":"Licensing is province-specific — the apostille only certifies the document for the regulator review."}
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
