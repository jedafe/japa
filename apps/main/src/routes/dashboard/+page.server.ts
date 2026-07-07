import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

interface TopContentItem {
	content_id: string;
	total_views: number;
	total_reactions: number;
	total_comments: number;
	total_unique_viewers: number;
}

interface EngagementItem {
	// flexible — backend shape may vary
	[key: string]: unknown;
}

interface ContentOverTimeItem {
	[key: string]: unknown;
}

interface UserDashboard {
	top_content: TopContentItem[];
	user_engagement: EngagementItem[];
	content_over_time: ContentOverTimeItem[];
}

export const load: PageServerLoad = async ({ cookies }) => {
	const token = cookies.get('jc_token');

	if (!token) {
		throw redirect(303, '/login');
	}

	const headers: Record<string, string> = { Authorization: `Bearer ${token}` };

	const dashboardResult = await apiFetch<UserDashboard>('/analytics/dashboard', { headers });

	return {
		dashboard: dashboardResult.ok && dashboardResult.data ? dashboardResult.data : null,
		isAuthenticated: true
	};
};
