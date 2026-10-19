// Ref: japaconnect-engineering/sections/79-community-story-wall.md
import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGetEnvelope, apiList } from '$lib/server/api';
import { attachAuthorUsernames } from '$lib/server/users';
import type { BlogSeries, GeoCountry, Story } from '$lib/types';
import type { GuideListItem } from '$lib/types/content';

/**
 * Community Story Wall at /stories — SSR list of relocation stories.
 * Fetches GET /stories?sort=top&page_size=20, passing through the sort
 * (top|latest) and filter query params (origin, dest, visa, profession).
 * ?tab=featured consumes the generic featured junction
 * (GET /content?type=story&featured=1 — §14) and ?tab=series lists series
 * containing stories (GET /series?content_type=story).
 * On error, degrades to an empty list with loadError: true.
 */
const PAGE_SIZE = 20;

export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['story.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Community Stories are not available' });
	}

	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	const sort = event.url.searchParams.get('sort') ?? 'top';
	const tab = event.url.searchParams.get('tab') ?? '';
	const origin = event.url.searchParams.get('origin') ?? '';
	const dest = event.url.searchParams.get('dest') ?? '';
	const visa = event.url.searchParams.get('visa') ?? '';
	const profession = event.url.searchParams.get('profession') ?? '';

	const base = {
		stories: [] as Story[],
		featured: [] as GuideListItem[],
		series: [] as BlogSeries[],
		total: undefined as number | undefined,
		authed,
		pageSize: PAGE_SIZE,
		loadError: false,
		sort: sort === 'latest' ? 'latest' : 'top',
		tab,
		filters: { origin, dest, visa, profession },
		countries: [] as GeoCountry[]
	};

	if (tab === 'featured') {
		try {
			const res = await apiGetEnvelope<GuideListItem[]>(
				ctx,
				`/content?type=story&featured=1&page_size=${PAGE_SIZE}`
			);
			const featured = res.data ?? [];
			await attachAuthorUsernames(ctx, featured);
			return { ...base, featured };
		} catch {
			return { ...base, loadError: true };
		}
	}

	if (tab === 'series') {
		const { items } = await apiList<BlogSeries>(
			ctx,
			`/series?content_type=story&page_size=${PAGE_SIZE}`
		).catch(() => ({ items: [] as BlogSeries[], meta: null }));
		return { ...base, series: items };
	}

	const params = new URLSearchParams();
	params.set('sort', sort === 'latest' ? 'latest' : 'top');
	params.set('page_size', String(PAGE_SIZE));
	if (origin) params.set('origin', origin);
	if (dest) params.set('dest', dest);
	if (visa) params.set('visa', visa);
	if (profession) params.set('profession', profession);

	// Countries and the story list are independent — one wave.
	const [countries, storiesRes] = await Promise.all([
		apiList<GeoCountry>(ctx, '/geo/countries')
			.then((r) => r.items)
			.catch(() => [] as GeoCountry[]),
		apiList<Story>(ctx, `/stories?${params.toString()}`)
			.then((r) => ({ ok: true as const, items: r.items, total: r.meta?.total }))
			.catch(() => ({ ok: false as const, items: [] as Story[], total: undefined as number | undefined }))
	]);

	if (storiesRes.ok) {
		await attachAuthorUsernames(ctx, storiesRes.items);
		return { ...base, stories: storiesRes.items, total: storiesRes.total, countries };
	}
	return { ...base, loadError: true, countries };
};
