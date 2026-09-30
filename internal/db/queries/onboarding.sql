-- name: GetOnboardingState :one
SELECT id, user_id, current_step, completed, step_data, tour_dismissed, tour_dismissals, created_at, updated_at
FROM onboarding_state
WHERE user_id = @user_id;

-- name: UpsertOnboardingState :exec
INSERT INTO onboarding_state (user_id, current_step, completed, step_data, tour_dismissed, tour_dismissals) VALUES (@user_id, @current_step, @completed, @step_data, @tour_dismissed, @tour_dismissals)

ON CONFLICT (user_id) DO UPDATE SET
 current_step = EXCLUDED.current_step,
 completed = EXCLUDED.completed,
 step_data = EXCLUDED.step_data,
 tour_dismissed = EXCLUDED.tour_dismissed,
 tour_dismissals = EXCLUDED.tour_dismissals,
 updated_at = now();

-- name: CompleteOnboarding :exec
UPDATE onboarding_state
SET completed = true, updated_at = now()
WHERE user_id = @user_id;

-- name: DismissTour :exec
-- Upserts so tour dismissal also works on surfaces (docs, learn) where the
-- user may never have run the main onboarding wizard and therefore has no
-- onboarding_state row yet. tour_dismissals is merged per-surface; the
-- legacy tour_dismissed flag is only set for the "main" surface.
INSERT INTO onboarding_state (user_id, tour_dismissed, tour_dismissals) VALUES (@user_id, @tour_dismissed, @tour_dismissals)

ON CONFLICT (user_id) DO UPDATE SET
 tour_dismissed = onboarding_state.tour_dismissed OR EXCLUDED.tour_dismissed,
 tour_dismissals = onboarding_state.tour_dismissals || EXCLUDED.tour_dismissals,
 updated_at = now();

-- name: AnonymizeOnboardingState :exec
-- GDPR erasure (ADR-014): clear step_data PII (name, current_location,
-- destination) during the anonymization phase. The row is retained (with
-- completed=true) until hard-delete cascade; only the PII jsonb is scrubbed.
UPDATE onboarding_state
SET step_data = '{}'::jsonb, completed = true, updated_at = now()
WHERE user_id = @user_id;

-- name: ListOnboardingTemplates :many
SELECT id, name, description, steps, is_active, is_system, created_at, updated_at
FROM onboarding_templates

ORDER BY created_at DESC;

-- name: GetOnboardingTemplate :one
SELECT id, name, description, steps, is_active, is_system, created_at, updated_at
FROM onboarding_templates
WHERE id = @id;

-- name: CreateOnboardingTemplate :one
INSERT INTO onboarding_templates (name, description, steps, is_active, is_system) VALUES (@name, @description, @steps, @is_active, @is_system)

RETURNING id, name, description, steps, is_active, is_system, created_at, updated_at;

-- name: UpdateOnboardingTemplate :exec
UPDATE onboarding_templates
SET name = @name, description = @description, steps = @steps, is_active = @is_active, updated_at = now()
WHERE id = @id;

-- name: DeleteOnboardingTemplate :exec
DELETE FROM onboarding_templates
WHERE id = @id;

-- name: ListOnboardingReminderCandidates :many
-- Active, non-completed onboarding users inside the reminder window who
-- haven't hit the send cap or the minimum interval, and who haven't opted
-- out of onboarding_reminder in-app notifications (§35 preferences).
SELECT u.id
FROM users u
LEFT JOIN onboarding_state o ON o.user_id = u.id
WHERE u.status = 'active'
  AND COALESCE(o.completed, false) = false
  AND u.created_at > now() - make_interval(days => @window_days::int)
  AND COALESCE(o.reminder_count, 0) < @max_count::int
  AND (o.last_reminder_at IS NULL
       OR o.last_reminder_at < now() - make_interval(hours => @min_interval_hours::int))
  AND NOT EXISTS (
      SELECT 1 FROM notification_preferences np
      WHERE np.user_id = u.id
        AND np.notification_type = 'onboarding_reminder'
        AND np.in_app = false
  )
ORDER BY u.created_at DESC
LIMIT @batch_limit::int;

-- name: MarkOnboardingReminded :exec
-- Upserts so users with no onboarding_state row (never started the wizard)
-- are tracked too — same pattern as DismissTour.
INSERT INTO onboarding_state (user_id, reminder_count, last_reminder_at)
VALUES (@user_id, 1, now())
ON CONFLICT (user_id) DO UPDATE SET
    reminder_count = onboarding_state.reminder_count + 1,
    last_reminder_at = now(),
    updated_at = now();
