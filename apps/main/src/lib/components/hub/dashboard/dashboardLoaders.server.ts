import { apiGet, apiList, type ApiContext } from '$lib/server/api';
import { buildContentQuery } from './contentQuery';
import type {
	MeAnalytics,
	FollowerGrowthItem,
	RecommendationsResponse,
	WalletSummary,
	LearnEnrollment,
	Certificate,
	UserDashboard
} from '$lib/types';

/**
 * Server-only loader registry for the dashboard family. Mirrors the settings
 * two-registry split — server fetch code stays out of the browser bundle.
 *
 * Ref: japaconnect-engineering/sections/23-main-site-frontend.md (Dashboard hub)
 */
export type DashboardLoaderParams = Record<string, string | undefined>;

type AnalyticsPeriod = '7d' | '30d' | '90d' | 'custom';

const VALID_PERIODS: AnalyticsPeriod[] = ['7d', '30d', '90d', 'custom'];

function nowISO(): string {
	return new Date().toISOString();
}

function daysAgoISO(days: number): string {
	return new Date(Date.now() - days * 24 * 60 * 60 * 1000).toISOString();
}

function periodBounds(
	period: AnalyticsPeriod,
	start?: string,
	end?: string
): { from: string; to: string } {
	if (period === 'custom' && start && end) {
		return { from: start, to: end };
	}
	const days = period === '7d' ? 7 : period === '90d' ? 90 : 30;
	return { from: daysAgoISO(days), to: nowISO() };
}

function previousPeriodBounds(
	period: AnalyticsPeriod,
	start?: string,
	end?: string
): { from: string; to: string } {
	if (period === 'custom' && start && end) {
		const startMs = new Date(start).getTime();
		const endMs = new Date(end).getTime();
		const duration = endMs - startMs;
		const prevFrom = new Date(startMs - duration).toISOString();
		return { from: prevFrom, to: start };
	}
	const days = period === '7d' ? 7 : period === '90d' ? 90 : 30;
	const now = Date.now();
	return {
		from: new Date(now - days * 2 * 24 * 60 * 60 * 1000).toISOString(),
		to: new Date(now - days * 24 * 60 * 60 * 1000).toISOString()
	};
}

function periodDays(period: AnalyticsPeriod, start?: string, end?: string): number {
	if (period === 'custom' && start && end) {
		const days = Math.ceil(
			(new Date(end).getTime() - new Date(start).getTime()) / (24 * 60 * 60 * 1000)
		);
		return Math.max(1, Math.min(365, days));
	}
	return period === '7d' ? 7 : period === '90d' ? 90 : 30;
}

function analyticsQuery(period: AnalyticsPeriod, start?: string, end?: string): string {
	const { from, to } = periodBounds(period, start, end);
	if (period === 'custom') {
		return `/me/analytics?period=custom&start=${encodeURIComponent(from)}&end=${encodeURIComponent(to)}`;
	}
	return `/me/analytics?period=${period}`;
}

function previousAnalyticsQuery(period: AnalyticsPeriod, start?: string, end?: string): string {
	const { from, to } = previousPeriodBounds(period, start, end);
	return `/me/analytics?period=custom&start=${encodeURIComponent(from)}&end=${encodeURIComponent(to)}`;
}

function dashboardQuery(period: AnalyticsPeriod, start?: string, end?: string): string {
	// GET /analytics/dashboard supports the same period presets as /me/analytics
	// plus explicit from/to timestamps for custom ranges.
	if (period === 'custom' && start && end) {
		return `/analytics/dashboard?from=${encodeURIComponent(start)}&to=${encodeURIComponent(end)}`;
	}
	return `/analytics/dashboard?period=${period}`;
}

function computeDeltas(current: MeAnalytics | null, previous: MeAnalytics | null) {
	const c = current ?? ({} as Partial<MeAnalytics>);
	const p = previous ?? ({} as Partial<MeAnalytics>);
	const cCreated = c.content_created ?? { published_count: 0 };
	const pCreated = p.content_created ?? { published_count: 0 };
	return {
		impressions: (c.total_views ?? 0) - (p.total_views ?? 0),
		reactions: (c.total_reactions ?? 0) - (p.total_reactions ?? 0),
		comments: (c.total_comments ?? 0) - (p.total_comments ?? 0),
		followers_gained: (c.total_followers_gained ?? 0) - (p.total_followers_gained ?? 0),
		posts: (cCreated.published_count ?? 0) - (pCreated.published_count ?? 0),
		bookmarks: (c.total_bookmarks ?? 0) - (p.total_bookmarks ?? 0),
		points: (c.total_points_earned ?? 0) - (p.total_points_earned ?? 0)
	};
}

export const DASHBOARD_LOADERS: Record<string, (ctx: ApiContext, params?: DashboardLoaderParams) => Promise<unknown>> = {
	overview: async (ctx) => ({
		analytics: await apiGet<MeAnalytics>(ctx, '/me/analytics').catch(() => null),
		recent: (await apiList<unknown>(ctx, '/me/content?page_size=5').catch(() => ({ items: [] }))).items,
		// Phase 18.01 — wallet + unread summary cards. Follower/following
		// counts come from /me/analytics (total_followers/total_following),
		// not a separate /me/followers/summary endpoint.
		wallet: await apiGet<WalletSummary>(ctx, '/me/wallet').catch(() => null),
		unread: await apiGet<unknown>(ctx, '/me/notifications/unread-count').catch(() => null),
		// ADR-064a — geo-aware tool/feature/content/space recommendations.
		// Best-effort: degrades to null when the endpoint is unavailable.
		recommendations: await apiGet<RecommendationsResponse>(ctx, '/recommendations?limit=6').catch(() => null)
	}),
	analytics: async (ctx, params) => {
		const rawPeriod = params?.period ?? '30d';
		const period = (VALID_PERIODS.includes(rawPeriod as AnalyticsPeriod)
			? rawPeriod
			: '30d') as AnalyticsPeriod;
		const start = params?.start;
		const end = params?.end;

		// Fetch current and previous period analytics, the Section 40 user
		// dashboard, and follower growth in parallel so the page is not blocked
		// on sequential API round trips. Missing previous data is treated as
		// zero, not a failure.
		const days = periodDays(period, start, end);
		const [analytics, previous, userDashboard, followerGrowth] = await Promise.all([
			apiGet<MeAnalytics>(ctx, analyticsQuery(period, start, end)).catch(() => null),
			apiGet<MeAnalytics>(ctx, previousAnalyticsQuery(period, start, end)).catch(() => null),
			apiGet<UserDashboard>(ctx, dashboardQuery(period, start, end)).catch(() => null),
			apiList<FollowerGrowthItem>(ctx, `/me/analytics/followers?days=${days}`).catch(
				() => ({ items: [] })
			)
		]);

		return {
			period,
			start,
			end,
			analytics,
			previous,
			deltas: computeDeltas(analytics, previous),
			followerGrowth: followerGrowth.items,
			userDashboard
		};
	},
	content: async (ctx, params) => {
		// Fetch page_size+1 so the client can tell whether a "Load more"
		// page exists without a separate count query (the API convention
		// returns { data: [...] } with no total). Ref: §50, api-response-format.
		const pageSize = 20;
		const path = buildContentQuery({
			status: params?.status,
			type: params?.type,
			q: params?.q,
			sort: params?.sort,
			pageSize: pageSize + 1
		});
		let batch: unknown[] = [];
		try {
			const contentRes = await apiList<unknown>(ctx, path);
			batch = contentRes.items;
		} catch {
			// Degrade to an empty list rather than 500ing the whole dashboard
			// when one list endpoint is unhealthy.
		}
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
	relationships: async (ctx, params = {}) => {
		const page = Math.max(1, parseInt(params.page ?? '1', 10) || 1);
		const pageSize = 25;
		const fetchSize = pageSize + 1;
		const offset = (page - 1) * pageSize;
		const type = params.type ?? '';
		const q = (params.q ?? '').trim();
		const spaceType = params.space_type ?? '';
		const level = params.level ?? '';

		const listParams = new URLSearchParams();
		listParams.set('page_size', String(fetchSize));
		listParams.set('page_offset', String(offset));
		if (type) listParams.set('type', type);

		// Spaces and muted spaces support server-side search, space type and level filters.
		const spaceParams = new URLSearchParams();
		spaceParams.set('page_size', String(fetchSize));
		spaceParams.set('page_offset', String(offset));
		if (q) spaceParams.set('q', q);
		if (spaceType) spaceParams.set('space_type', spaceType);
		if (level) spaceParams.set('level', level);
		// member_only forces the membership listing even for super admins, whose
		// /me/spaces response otherwise returns every space in the tenant.
		const memberSpaceParams = new URLSearchParams(spaceParams);
		memberSpaceParams.set('member_only', '1');

		const list = (path: string, search = listParams) =>
			apiList<unknown>(ctx, `${path}?${search.toString()}`).catch(() => ({ items: [] }));

		const analytics = await apiGet<MeAnalytics>(ctx, '/me/analytics').catch(() => null);

		const [
			followingRes,
			followersRes,
			spacesRes,
			tagsRes,
			blockedRes,
			mutedUsersRes,
			mutedSpacesRes
		] = await Promise.all([
			list('/me/following'),
			list('/me/followers'),
			list('/me/spaces', memberSpaceParams),
			list('/me/followed-tags'),
			list('/me/blocked-users'),
			list('/me/muted-users'),
			list('/me/muted-spaces', spaceParams)
		]);

		function paginated(items: unknown[]) {
			const hasMore = items.length > pageSize;
			return { items: items.slice(0, pageSize), hasMore };
		}

		const following = paginated(followingRes.items);
		const followers = paginated(followersRes.items);
		const spaces = paginated(spacesRes.items);
		const tags = paginated(tagsRes.items);
		const blocked = paginated(blockedRes.items);
		const mutedUsers = paginated(mutedUsersRes.items);
		const mutedSpaces = paginated(mutedSpacesRes.items);

		return {
			following: following.items,
			followers: followers.items,
			spaces: spaces.items,
			tags: tags.items,
			blocked: blocked.items,
			mutedUsers: mutedUsers.items,
			mutedSpaces: mutedSpaces.items,
			counts: {
				following: analytics?.total_following,
				followers: analytics?.total_followers
			},
			hasMore: {
				following: following.hasMore,
				followers: followers.hasMore,
				spaces: spaces.hasMore,
				tags: tags.hasMore,
				blocked: blocked.hasMore,
				'muted-users': mutedUsers.hasMore,
				'muted-spaces': mutedSpaces.hasMore
			},
			page,
			pageSize,
			q,
			type,
			spaceType,
			level
		};
	},
	learning: async (ctx) => (await apiList<LearnEnrollment>(ctx, '/me/enrollments').catch(() => ({ items: [] }))).items,
	certificates: async (ctx) => (await apiList<Certificate>(ctx, '/me/certificates').catch(() => ({ items: [] }))).items,
	'knowledge-base': async (ctx) =>
		(
			await apiList<unknown>(ctx, '/me/bookmarks?entity_type=doc_page').catch(() => ({
				items: []
			}))
		).items
};
