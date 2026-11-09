ALTER TABLE public.form_steps
  ADD COLUMN config jsonb NOT NULL DEFAULT '{}'::jsonb;
