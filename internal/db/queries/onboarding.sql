-- name: GetOnboardingState :one
SELECT id, tenant_id, user_id, current_step, completed, step_data, tour_dismissed, created_at, updated_at
FROM onboarding_state
WHERE tenant_id = @tenant_id AND user_id = @user_id;

-- name: UpsertOnboardingState :exec
INSERT INTO onboarding_state (tenant_id, user_id, current_step, completed, step_data, tour_dismissed)
VALUES (@tenant_id, @user_id, @current_step, @completed, @step_data, @tour_dismissed)
ON CONFLICT (tenant_id, user_id) DO UPDATE SET
    current_step = EXCLUDED.current_step,
    completed = EXCLUDED.completed,
    step_data = EXCLUDED.step_data,
    tour_dismissed = EXCLUDED.tour_dismissed,
    updated_at = now();

-- name: CompleteOnboarding :exec
UPDATE onboarding_state
SET completed = true, updated_at = now()
WHERE tenant_id = @tenant_id AND user_id = @user_id;

-- name: DismissTour :exec
UPDATE onboarding_state
SET tour_dismissed = true, updated_at = now()
WHERE tenant_id = @tenant_id AND user_id = @user_id;

-- name: ListOnboardingTemplates :many
SELECT id, tenant_id, name, description, steps, is_active, is_system, created_at, updated_at
FROM onboarding_templates
WHERE tenant_id = @tenant_id
ORDER BY created_at DESC;

-- name: GetOnboardingTemplate :one
SELECT id, tenant_id, name, description, steps, is_active, is_system, created_at, updated_at
FROM onboarding_templates
WHERE id = @id AND tenant_id = @tenant_id;

-- name: CreateOnboardingTemplate :one
INSERT INTO onboarding_templates (tenant_id, name, description, steps, is_active, is_system)
VALUES (@tenant_id, @name, @description, @steps, @is_active, @is_system)
RETURNING id, tenant_id, name, description, steps, is_active, is_system, created_at, updated_at;

-- name: UpdateOnboardingTemplate :exec
UPDATE onboarding_templates
SET name = @name, description = @description, steps = @steps, is_active = @is_active, updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id;

-- name: DeleteOnboardingTemplate :exec
DELETE FROM onboarding_templates
WHERE id = @id AND tenant_id = @tenant_id;
