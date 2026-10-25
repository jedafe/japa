-- 000934_verification_request_evidence_fields.up.sql
-- Richer evidence on the §07 verification-request form: document uploads,
-- a live-photo (selfie) field, and social media handles — the scrutiny
-- reviewers need before granting a badge. The Form Builder seed is
-- admin-editable; this updates the shipped default set.

-- Evidence URL becomes optional now that documents upload directly.
UPDATE custom_fields
SET is_required = false,
    position = 8,
    label = 'Additional evidence link'
WHERE id = '00000000-0000-0000-0000-00000000d023';

UPDATE custom_fields SET position = 9  WHERE id = '00000000-0000-0000-0000-00000000d024';
UPDATE custom_fields SET position = 10 WHERE id = '00000000-0000-0000-0000-00000000d014';

INSERT INTO custom_fields (id, group_id, label, key, field_type, placeholder, is_required, is_system, show_in_frontend, position) VALUES
  -- Identity documents: media field accepts any file (PDF, image, scan).
  ('00000000-0000-0000-0000-00000000d015', '00000000-0000-0000-0000-00000000d001',
   'Identity / registration documents', 'id_document', 'media',
   'Passport, national ID, certificate of incorporation, or license', true, true, true, 1),
  -- Live photo: image-only upload — reviewer matches it against the document.
  ('00000000-0000-0000-0000-00000000d016', '00000000-0000-0000-0000-00000000d001',
   'Live photo (selfie holding your ID)', 'live_photo', 'image_single',
   'Clear photo of you holding the document above', true, true, true, 2),
  -- Social media handles (optional, aids reviewer triangulation).
  ('00000000-0000-0000-0000-00000000d017', '00000000-0000-0000-0000-00000000d001',
   'X (Twitter) handle or profile URL', 'x_handle', 'text',
   '@handle or https://x.com/handle', false, true, true, 3),
  ('00000000-0000-0000-0000-00000000d018', '00000000-0000-0000-0000-00000000d001',
   'Instagram handle or profile URL', 'instagram_handle', 'text',
   '@handle or https://instagram.com/handle', false, true, true, 4),
  ('00000000-0000-0000-0000-00000000d019', '00000000-0000-0000-0000-00000000d001',
   'LinkedIn profile URL', 'linkedin_url', 'url',
   'https://linkedin.com/in/… or /company/…', false, true, true, 5),
  ('00000000-0000-0000-0000-00000000d020', '00000000-0000-0000-0000-00000000d001',
   'Facebook page URL', 'facebook_url', 'url',
   'https://facebook.com/…', false, true, true, 6),
  ('00000000-0000-0000-0000-00000000d021', '00000000-0000-0000-0000-00000000d001',
   'YouTube channel URL', 'youtube_url', 'url',
   'https://youtube.com/@…', false, true, true, 7)
ON CONFLICT (id) DO NOTHING;
