import { apiGet, apiList, type ApiContext } from '$lib/server/api';
import { buildContentQuery } from './contentQuery';

/**
 * Server-only loader registry for the dashboard family. Mirrors the settings
 * two-registry split — server fetch code stays out of the browser bundle.
 *
 * Ref: japaconnect-engineering/sections/23-main-site-frontend.md (Dashboard hub)
 */
export const DASHBOARD_LOADERS: Record<string, (ctx: ApiContext) => Promise<unknown>> = {
	overview: async (ctx) => ({
		analytics: await apiGet<unknown>(ctx, '/me/analytics').catch(() => null),
		recent: (await apiList<unknown>(ctx, '/me/content?page_size=5')).items
	}),
	analytics: async (ctx) => ({
		analytics: await apiGet<unknown>(ctx, '/me/analytics').catch(() => null)
	}),
	content: async (ctx) => (await apiList<unknown>(ctx, buildContentQuery({}))).items,
	relationships: async (ctx) => ({
		following: (await apiList<unknown>(ctx, '/me/following')).items,
		spaces: (await apiList<unknown>(ctx, '/me/spaces')).items,
		tags: (await apiList<unknown>(ctx, '/me/followed-tags')).items
	}),
	learning: async (ctx) => (await apiList<unknown>(ctx, '/me/enrollments')).items,
	certificates: async (ctx) => (await apiList<unknown>(ctx, '/me/certificates')).items,
	'knowledge-base': async (ctx) => (await apiList<unknown>(ctx, '/me/bookmarks')).items
};
