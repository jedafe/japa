import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

interface OnboardingStatus {
	current_step: number;
	completed: boolean;
	step_data: Record<string, unknown>;
	tour_dismissed: boolean;
	created_at: string;
	updated_at: string;
}

export const load: PageServerLoad = async ({ locals, cookies }) => {
	if (!locals.user) {
		throw redirect(303, '/login');
	}

	const token = cookies.get('jc_token');
	const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {};

	const result = await apiFetch<OnboardingStatus>('/onboarding/status', { headers });

	// If onboarding is already completed, redirect to home.
	if (result.ok && result.data?.completed) {
		throw redirect(303, '/');
	}

	return {
		onboarding: result.ok && result.data
			? result.data
			: { current_step: 1, completed: false, step_data: {}, tour_dismissed: false }
	};
};
