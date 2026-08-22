import { apiGet, apiList, type ApiContext } from '$lib/server/api';
import { contentBase, contentQueryString } from './contentQuery';
import type { RecommendationsResponse, LearnEnrollment, Certificate } from '$lib/types';

/**
 * Server-only loader registry for the dashboard family. Mirrors the settings
 * two-registry split — server fetch code stays out of the browser bundle.
 *
 * Ref: japaconnect-engineering/sections/23-main-site-frontend.md (Dashboard hub)
 */
export type DashboardLoaderParams = Record<string, string | undefined>;

export const DASHBOARD_LOADERS: Record<string, (ctx: ApiContext, params?: DashboardLoaderParams) => Promise<unknown>> = {
	overview: async (ctx) => ({
		analytics: await apiGet<unknown>(ctx, '/me/analytics').catch(() => null),
		recent: (await apiList<unknown>(ctx, '/me/content?page_size=5')).items,
		// Phase 18.01 — wallet + unread summary cards. Follower/following
		// counts come from /me/analytics (total_followers/total_following),
		// not a separate /me/followers/summary endpoint.
		wallet: await apiGet<unknown>(ctx, '/me/wallet').catch(() => null),
		unread: await apiGet<unknown>(ctx, '/me/notifications/unread-count').catch(() => null),
		// ADR-064a — geo-aware tool/feature/content/space recommendations.
		// Best-effort: degrades to null when the endpoint is unavailable.
		recommendations: await apiGet<RecommendationsResponse>(ctx, '/recommendations?limit=6').catch(() => null)
	}),
	analytics: async (ctx) => ({
		analytics: await apiGet<unknown>(ctx, '/me/analytics').catch(() => null),
		// Phase 18.01a — per-content breakdown for the top-posts BarChart and
		// daily follower growth for the follower-growth LineChart. Each
		// degrades gracefully so a missing endpoint never blanks the page.
		// Ref: sections/40-platform-analytics-reports.md, ADR-059
		contentAnalytics: (await apiList<unknown>(ctx, '/me/analytics/content?page_size=10').catch(() => ({ items: [] }))).items,
		followerGrowth: (await apiList<unknown>(ctx, '/me/analytics/followers?days=30').catch(() => ({ items: [] }))).items
	}),
	content: async (ctx, params) => {
		// Fetch page_size+1 so the client can tell whether a "Load more"
		// page exists without a separate count query (the API convention
		// returns { data: [...] } with no total). Ref: §50, api-response-format.
		const pageSize = 20;
		const status = params?.status;
		const qs = contentQueryString({ status, type: params?.type, q: params?.q, pageSize: pageSize + 1 });
		const contentRes = await (status === 'draft'
			? apiList<unknown>(ctx, `/me/drafts?${qs}`)
			: status === 'scheduled'
				? apiList<unknown>(ctx, `/me/scheduled?${qs}`)
				: apiList<unknown>(ctx, `/me/content?${qs}`));
		const batch = contentRes.items;
		const hasMore = batch.length > pageSize;
		const items = hasMore ? batch.slice(0, pageSize) : batch;
		return {
			items,
			// Rule 16 — content type options come from the API, not hardcoded.
			contentTypes: (await apiList<unknown>(ctx, '/content-types').catch(() => ({ items: [] }))).items,
			pageSize,
			hasMore
		};
	},
	relationships: async (ctx) => ({
		following: (await apiList<unknown>(ctx, '/me/following').catch(() => ({ items: [] }))).items,
		followers: (await apiList<unknown>(ctx, '/me/followers').catch(() => ({ items: [] }))).items,
		spaces: (await apiList<unknown>(ctx, '/me/spaces').catch(() => ({ items: [] }))).items,
		tags: (await apiList<unknown>(ctx, '/me/followed-tags').catch(() => ({ items: [] }))).items,
		blocked: (await apiList<unknown>(ctx, '/me/blocked-users').catch(() => ({ items: [] }))).items,
		// FE-2: muted users + spaces (Section 52). Best-effort — degrades to
		// empty list on failure.
		mutedUsers: (await apiList<unknown>(ctx, '/me/muted-users').catch(() => ({ items: [] }))).items,
		mutedSpaces: (await apiList<unknown>(ctx, '/me/muted-spaces').catch(() => ({ items: [] }))).items
	}),
	learning: async (ctx) => (await apiList<LearnEnrollment>(ctx, '/me/enrollments').catch(() => ({ items: [] }))).items,
	certificates: async (ctx) => (await apiList<Certificate>(ctx, '/me/certificates').catch(() => ({ items: [] }))).items,
	'knowledge-base': async (ctx) =>
		(
			await apiList<unknown>(ctx, '/me/bookmarks?entity_type=doc_page').catch(() => ({
				items: []
			}))
		).items
};
