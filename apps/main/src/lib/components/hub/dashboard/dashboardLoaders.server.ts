import { apiGet, apiList, type ApiContext } from '$lib/server/api';
import { buildContentQuery } from './contentQuery';

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
		unread: await apiGet<unknown>(ctx, '/me/notifications/unread-count').catch(() => null)
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
	content: async (ctx, params) => ({
		items: (await apiList<unknown>(ctx, buildContentQuery({ status: params?.status, type: params?.type }))).items,
		// Rule 16 — content type options come from the API, not hardcoded.
		contentTypes: (await apiList<unknown>(ctx, '/content-types').catch(() => ({ items: [] }))).items
	}),
	relationships: async (ctx) => ({
		following: (await apiList<unknown>(ctx, '/me/following').catch(() => ({ items: [] }))).items,
		spaces: (await apiList<unknown>(ctx, '/me/spaces').catch(() => ({ items: [] }))).items,
		tags: (await apiList<unknown>(ctx, '/me/followed-tags').catch(() => ({ items: [] }))).items,
		blocked: (await apiList<unknown>(ctx, '/me/blocked-users').catch(() => ({ items: [] }))).items
	}),
	learning: async (ctx) => (await apiList<unknown>(ctx, '/me/enrollments')).items,
	certificates: async (ctx) => (await apiList<unknown>(ctx, '/me/certificates')).items,
	'knowledge-base': async (ctx) => (await apiList<unknown>(ctx, '/me/bookmarks')).items
};
