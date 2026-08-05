import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import { availableTypes } from '$lib/components/create/contentTypeRegistry';
import type {
	ContentTypeMeta,
	MeSpace,
	CustomFieldGroup,
	BlogSeries,
	ContentRecord,
	ComposerOrganization,
	ComparisonDetail,
	ComparisonType
} from '$lib/types';

/** Load everything the composer needs; every fetch degrades independently. */
export const load: PageServerLoad = async (event) => {
	// /create is auth-gated — a guest has no spaces, drafts, or series to compose with.
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}

	const ctx = apiContext(event);
	const draftId = event.url.searchParams.get('draft');

	const [typesRes, flagsRes, spacesRes, groupsRes, seriesRes, orgsRes, compTypesRes] =
		await Promise.allSettled([
			apiList<ContentTypeMeta>(ctx, '/content-types'),
			apiGet<Record<string, boolean>>(ctx, '/feature-flags'),
			apiList<MeSpace>(ctx, '/me/spaces'),
			apiList<CustomFieldGroup>(ctx, '/custom-field-groups'),
			apiList<BlogSeries>(ctx, '/blog/series'),
			// Post-as-org (ADR-062): the caller's org memberships, used by the
			// "post on behalf of" selector. Empty list = personal posts only.
			apiList<ComposerOrganization>(ctx, '/me/organizations'),
			// Comparison types (Section 30) — for the type selector in the
			// comparison editor. Empty list = "General (no type)" only.
			apiList<ComparisonType>(ctx, '/comparisons/types?page_size=50')
		]);

	const rawTypes = typesRes.status === 'fulfilled' ? typesRes.value.items : [];
	const flags = flagsRes.status === 'fulfilled' ? flagsRes.value : {};
	const spaces = spacesRes.status === 'fulfilled' ? spacesRes.value.items : [];
	const fieldGroups =
		groupsRes.status === 'fulfilled'
			? groupsRes.value.items.filter(
					(g) =>
						g.entity_type === 'content' &&
						// Exclude guide category-specific field groups — those are loaded
						// on demand by CreateShell when a guide category is selected,
						// via GET /custom-field-groups?entity_class=content_type:guide:{cat}
						!(g.slug?.startsWith('guide_') && g.slug?.endsWith('_fields'))
				)
			: [];
	const series = seriesRes.status === 'fulfilled' ? seriesRes.value.items : [];
	const organizations = orgsRes.status === 'fulfilled' ? orgsRes.value.items : [];
	const comparisonTypes = compTypesRes.status === 'fulfilled' ? compTypesRes.value.items : [];

	let draft: ContentRecord | null = null;
	let comparisonDetail: ComparisonDetail | null = null;
	if (draftId) {
		try {
			// Ownership-scoped: GET /me/drafts/{id} returns the caller's own
			// draft only (tenant + author + status='draft'); 404 otherwise.
			// The public GET /content/{slug} resolves by slug and is not
			// ownership-scoped, so it cannot safely serve the editor restore.
			draft = await apiGet<ContentRecord>(ctx, `/me/drafts/${draftId}`);
			// When the draft is a comparison, also fetch its items/dimensions
			// so the ComparisonBuilder can render them (Section 30). The
			// /compare/new wizard creates a draft and redirects here.
			if (draft?.content_type === 'comparison') {
				try {
					comparisonDetail = await apiGet<ComparisonDetail>(ctx, `/comparisons/${draft.id}`);
				} catch {
					comparisonDetail = null;
				}
			}
		} catch {
			draft = null;
		}
	}

	return {
		types: availableTypes(rawTypes, flags),
		spaces,
		fieldGroups,
		series,
		organizations,
		draft,
		comparisonDetail,
		comparisonTypes,
		loadError: typesRes.status === 'rejected'
	};
};
