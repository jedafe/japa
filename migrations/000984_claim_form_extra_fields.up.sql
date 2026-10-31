-- 000984_claim_form_extra_fields.up.sql
-- More evidence on the claim-ownership form: contact channels on the
-- connection step, and a document upload + official listing on the
-- evidence step (the wizard renders media fields with upload UX).
INSERT INTO custom_fields (id, group_id, label, key, field_type, placeholder, is_required, is_system, show_in_frontend, position) VALUES
  ('00000000-0000-0000-0000-00000000d050', '00000000-0000-0000-0000-00000000d042',
   'Work email on the organization domain', 'contact_email', 'email',
   'you@organization.com', false, true, true, 2),
  ('00000000-0000-0000-0000-00000000d051', '00000000-0000-0000-0000-00000000d042',
   'Contact phone', 'contact_phone', 'text',
   '+234 …', false, true, true, 3),
  ('00000000-0000-0000-0000-00000000d052', '00000000-0000-0000-0000-00000000d043',
   'Proof documents', 'proof_documents', 'media',
   'Registration certificate, utility bill, or authorization letter', false, true, true, 2),
  ('00000000-0000-0000-0000-00000000d053', '00000000-0000-0000-0000-00000000d043',
   'Official registry or website listing', 'org_website', 'url',
   'https://registry.example.com/org/…', false, true, true, 4)
ON CONFLICT (id) DO NOTHING;
