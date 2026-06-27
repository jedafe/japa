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
