import { env } from '$env/dynamic/private';
import type { PageServerLoad } from './$types';

const API_BASE = env.PRIVATE_API_BASE_URL ?? 'http://localhost:8080';

export const load: PageServerLoad = async ({ locals }) => {
	const token = (locals as any)?.token;
	const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {};

	try {
		const [overviewRes, securityRes, usersRes, contentRes, spacesRes, revenueRes, searchRes] = await Promise.allSettled([
			fetch(`${API_BASE}/api/v1/admin/analytics/overview`, { headers }),
			fetch(`${API_BASE}/api/v1/admin/analytics/security`, { headers }),
			fetch(`${API_BASE}/api/v1/admin/analytics/users`, { headers }),
			fetch(`${API_BASE}/api/v1/admin/analytics/content`, { headers }),
			fetch(`${API_BASE}/api/v1/admin/analytics/spaces`, { headers }),
			fetch(`${API_BASE}/api/v1/admin/analytics/revenue`, { headers }),
			fetch(`${API_BASE}/api/v1/admin/analytics/search`, { headers })
		]);

		const parseResponse = async (result: PromiseSettledResult<Response>) => {
			if (result.status === 'fulfilled' && result.value.ok) {
				const { data } = await result.value.json();
				return data ?? null;
			}
			return null;
		};

		const overview = await parseResponse(overviewRes);
		const security = await parseResponse(securityRes);
		const userAnalytics = await parseResponse(usersRes);
		const contentAnalytics = await parseResponse(contentRes);
		const spaceAnalytics = await parseResponse(spacesRes);
		const revenueAnalytics = await parseResponse(revenueRes);
		const searchAnalytics = await parseResponse(searchRes);

		return {
			overview,
			security,
			userAnalytics,
			contentAnalytics,
			spaceAnalytics,
			revenueAnalytics,
			searchAnalytics
		};
	} catch {
		return {
			overview: null,
			security: null,
			userAnalytics: null,
			contentAnalytics: null,
			spaceAnalytics: null,
			revenueAnalytics: null,
			searchAnalytics: null,
			error: 'Failed to load analytics'
		};
	}
};
