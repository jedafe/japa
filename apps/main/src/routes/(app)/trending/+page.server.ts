import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type {
	TrendingContent,
	TrendingTag,
	TrendingSpace,
	TrendingUser,
	TrendingWindow
} from '$lib/types';

/**
 * Trending hub at /trending — public, SSR, indexable. Four leaderboards
 * (content / tags / spaces / users) behind a tab bar, scoped to a time
 * window (24h / 7d / 30d). All four feeds are resolved server-side so the
 * active tab paints immediately on navigation; each degrades to an empty
 * list on failure so one broken feed never blanks the page.
 *
 * Ref: japaconnect-engineering/sections/53-trending-discovery.md
 *      ADR-058 (renders inside the app shell)
 */
const LIMIT = 25;
const WINDOWS: TrendingWindow[] = ['24h', '7d', '30d'];

function resolveWindow(raw: string | null): TrendingWindow {
	return (WINDOWS as string[]).includes(raw ?? '') ? (raw as TrendingWindow) : '24h';
}

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const window = resolveWindow(event.url.searchParams.get('window'));
	const tab = event.url.searchParams.get('tab') ?? 'content';
	const q = `?window=${window}&limit=${LIMIT}`;

	const [content, tags, spaces, users] = await Promise.all([
		apiList<TrendingContent>(ctx, `/trending/content${q}`).then((r) => r.items).catch(() => []),
		apiList<TrendingTag>(ctx, `/trending/tags${q}`).then((r) => r.items).catch(() => []),
		apiList<TrendingSpace>(ctx, `/trending/spaces${q}`).then((r) => r.items).catch(() => []),
		apiList<TrendingUser>(ctx, `/trending/users${q}`).then((r) => r.items).catch(() => [])
	]);

	return { content, tags, spaces, users, window, tab, limit: LIMIT };
};
