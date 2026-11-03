INSERT INTO public.notification_types (key, label, description, enabled, default_in_app, default_email, default_push, category)
VALUES ('content.spam_flagged', 'Content Flagged', 'Your content was flagged by safety filters', true, true, false, false, 'content')
ON CONFLICT (key) DO NOTHING;
