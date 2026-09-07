import { fail, redirect } from '@sveltejs/kit';
import type { PageServerLoad, Actions } from './$types';
import { apiContext, apiGet, apiList, apiPost, apiPut, ApiError } from '$lib/server/api';
import type {
	OrgMeta,
	PublicOrganization,
	JCEComparison,
	ComparisonCreated,
	ComparisonItem,
	ComparisonDimension
} from '$lib/types';

/**
 * Organizations index at /org — public, SSR, indexable. Lists every
 * tenant organization with member/follower counts and verification state.
 * Each card links to the org profile at /org/[slug]. Search filters by
 * name or slug. org_type and verified filters are optional.
 *
 * Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md
 *      ADR-058 (renders inside the app shell)
 */
const PAGE_SIZE = 24;

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const authed = !!event.locals.user;
	const q = (event.url.searchParams.get('q') ?? '').trim();
	const orgType = (event.url.searchParams.get('org_type') ?? '').trim();
	const verified = (event.url.searchParams.get('verified') ?? '').trim();
	// 1-based ?page= param, matching the /compare pagination convention.
	const pageParam = parseInt(event.url.searchParams.get('page') ?? '1', 10);
	const page = isNaN(pageParam) || pageParam < 1 ? 1 : pageParam;
	const offset = (page - 1) * PAGE_SIZE;

	let orgs: PublicOrganization[] = [];
	let total = 0;
	let totalPages = 0;
	let loadError = false;
	let forbidden = false;

	// BUG-022: Fetch org type options from the API (Rule 16).
	let orgMeta: OrgMeta | null = null;
	try {
		orgMeta = await apiGet<OrgMeta>(ctx, '/organizations/meta');
	} catch {
		// Non-fatal — the page falls back to an empty filter.
	}

	try {
		const params = new URLSearchParams();
		params.set('page_size', String(PAGE_SIZE));
		params.set('page_offset', String(offset));
		if (q) params.set('q', q);
		if (orgType) params.set('org_type', orgType);
		if (verified) params.set('verified', verified);

		const result = await apiList<PublicOrganization>(ctx, `/organizations?${params.toString()}`);
		orgs = result.items;
		total = result.meta?.total ?? 0;
		totalPages = result.meta?.total_pages ?? 0;
	} catch (err) {
		if (err instanceof ApiError && err.status === 403) {
			forbidden = true;
		} else {
			loadError = true;
		}
	}

	return {
		orgs,
		q,
		orgType,
		verified,
		page,
		totalPages,
		total,
		pageSize: PAGE_SIZE,
		forbidden,
		loadError,
		orgMeta,
		authed
	};
};

/**
 * Build a private quick JCE comparison from 2-4 selected organizations.
 * The matrix is pre-seeded with public org fields (type, verified state,
 * follower/member counts, country, website, description) so the
 * /compare/{slug} preview is immediately useful.
 *
 * Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md
 *      japaconnect-engineering/sections/30-generic-comparison-system.md
 */
export const actions: Actions = {
	default: async (event) => {
		if (!event.locals.user) {
			return fail(401, { error: 'unauthenticated', message: 'Sign in to compare organizations' });
		}

		const form = await event.request.formData();
		const orgIds = form.getAll('org_id').filter((v): v is string => typeof v === 'string');
		const orgSlugs = form.getAll('org_slug').filter((v): v is string => typeof v === 'string');

		if (orgIds.length < 2 || orgIds.length > 4) {
			return fail(400, { error: 'invalid_selection', message: 'Select 2 to 4 organizations' });
		}
		if (orgSlugs.length !== orgIds.length) {
			return fail(400, { error: 'invalid_selection', message: 'Selection mismatch' });
		}

		const ctx = apiContext(event);

		// Fetch full details for each selected organization in parallel.
		const details = await Promise.all(
			orgSlugs.map(async (slug) => {
				try {
					return await apiGet<PublicOrganization>(ctx, `/organizations/${encodeURIComponent(slug)}`);
				} catch (e) {
					const message = e instanceof ApiError ? e.message : String(e);
					console.error('failed to fetch organization details:', message);
					return null;
				}
			})
		);

		const orgs = details.filter((o): o is PublicOrganization => !!o);
		if (orgs.length !== orgIds.length) {
			return fail(500, { error: 'fetch_failed', message: 'One or more selected organizations could not be loaded' });
		}

		const items = orgs.map((o, i) => ({
			title: o.name,
			description: o.tagline ?? o.description ?? '',
			position: i,
			item_ref: { type: 'organization', id: o.id }
		}));

		const dimensionLabels = ['Type', 'Verified', 'Followers', 'Members', 'Country', 'Website', 'Description'];

		let created: ComparisonCreated;
		try {
			created = await apiPost<ComparisonCreated>(ctx, '/jce/comparisons', {
				title: 'Organization comparison',
				depth: 'quick',
				body: '',
				items,
				dimensions: dimensionLabels
			});
		} catch (e) {
			const message = e instanceof ApiError ? e.message : 'Could not create comparison';
			return fail(500, { error: 'create_failed', message });
		}

		let jce: JCEComparison;
		try {
			jce = await apiGet<JCEComparison>(ctx, `/jce/comparisons/${created.id}`);
		} catch (e) {
			const message = e instanceof ApiError ? e.message : 'Could not load created comparison';
			return fail(500, { error: 'detail_failed', message });
		}

		const itemByPosition = new Map<number, ComparisonItem>();
		for (const [i, item] of jce.items.entries()) {
			itemByPosition.set(i, item);
		}
		const dimByLabel = new Map(jce.dimensions.map((d) => [d.label, d]));

		const fieldMap: Record<string, (o: PublicOrganization) => string> = {
			Type: (o) => o.org_type,
			Verified: (o) => (o.verified ? 'Verified' : 'Unverified'),
			Followers: (o) => new Intl.NumberFormat('en-US').format(o.follower_count),
			Members: (o) => new Intl.NumberFormat('en-US').format(o.member_count),
			Country: (o) => o.country_code ?? '—',
			Website: (o) => o.website_url ?? '—',
			Description: (o) => o.description ?? o.tagline ?? ''
		};

		for (const [label, fn] of Object.entries(fieldMap)) {
			const dim = dimByLabel.get(label);
			if (!dim) continue;

			const values = orgs
				.map((o, i) => {
					const item = itemByPosition.get(i);
					if (!item) return null;
					return { item_id: item.id, value: fn(o) };
				})
				.filter((v): v is { item_id: string; value: string } => v !== null);

			if (values.length === 0) continue;

			try {
				await apiPut<unknown>(ctx, `/jce/comparisons/dimensions/${dim.id}/values`, { values });
			} catch (e) {
				// Seed failures are non-fatal; the user can still see the
				// created comparison and fill values in the composer.
				const message = e instanceof ApiError ? e.message : String(e);
				console.error('failed to seed dimension values:', message);
			}
		}

		throw redirect(303, `/compare/${created.slug}`);
	}
};
