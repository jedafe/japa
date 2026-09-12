-- name: GetOnboardingState :one
SELECT id, user_id, current_step, completed, step_data, tour_dismissed, created_at, updated_at
FROM onboarding_state
WHERE user_id = @user_id;

-- name: UpsertOnboardingState :exec
INSERT INTO onboarding_state (user_id, current_step, completed, step_data, tour_dismissed) VALUES (@user_id, @current_step, @completed, @step_data, @tour_dismissed)

ON CONFLICT (user_id) DO UPDATE SET
 current_step = EXCLUDED.current_step,
 completed = EXCLUDED.completed,
 step_data = EXCLUDED.step_data,
 tour_dismissed = EXCLUDED.tour_dismissed,
 updated_at = now();

-- name: CompleteOnboarding :exec
UPDATE onboarding_state
SET completed = true, updated_at = now()
WHERE user_id = @user_id;

-- name: DismissTour :exec
UPDATE onboarding_state
SET tour_dismissed = true, updated_at = now()
WHERE user_id = @user_id;

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
