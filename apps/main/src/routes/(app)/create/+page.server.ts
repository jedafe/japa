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
	ComparisonType,
	ContentTag,
	MediaAsset
} from '$lib/types';

/** Load everything the composer needs; every fetch degrades independently. */
export const load: PageServerLoad = async (event) => {
	// /create is auth-gated — a guest has no spaces, drafts, or series to compose with.
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}

	const ctx = apiContext(event);
	const draftId = event.url.searchParams.get('draft');

	const [typesRes, flagsRes, spacesRes, groupsRes, seriesRes, orgsRes, compTypesRes, dimTypesRes] =
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
			apiList<ComparisonType>(ctx, '/comparisons/types?page_size=50'),
			// Dimension types (Section 30) — for the dimension type selector
			// in the ComparisonBuilder (Rule 16 — no hardcoded enums).
			apiList<{ value: string; label: string }>(ctx, '/comparisons/dimension-types')
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
	const dimensionTypes = dimTypesRes.status === 'fulfilled' ? dimTypesRes.value.items : [];

	let draft: ContentRecord | null = null;
	let comparisonDetail: ComparisonDetail | null = null;
	let draftTags: string[] = [];
	let coverMedia: MediaAsset | null = null;
	let draftSeriesId: string | null = null;
	if (draftId) {
		try {
			// Ownership-scoped: GET /me/content/{id} returns the caller's own
			// content by ID (any status — published, scheduled, or draft),
			// tenant + author scoped. The public GET /content/{slug} resolves
			// by slug and is not ownership-scoped, so it cannot safely serve
			// the editor restore. The status='draft'-scoped /me/drafts/{id}
			// only covers drafts; /me/content/{id} covers the rest so the
			// Edit button on published content detail pages works too.
			// For non-owners (super-admin or `content.edit` RBAC holder), the
			// backend falls back to a tenant-scoped fetch — see GetMyContent.
			draft = await apiGet<ContentRecord>(ctx, `/me/content/${draftId}`);
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
			// Restore the tag chips — GET /content/{id}/tags returns the tags
			// joined via content_tags. The composer consumes tag slugs (not
			// names) because attachTag/detachTag send the value as a slug to
			// POST /content/{id}/tags { tag_slugs: [slug] } and
			// DELETE /content/{id}/tags/{slug}. Loading names here would
			// break detach — the backend slug resolver cannot match a
			// display name like "Driving License" to the stored slug
			// "driving-license". Deduplicate by slug — the join can return
			// the same tag multiple times if a tag was attached more than
			// once (data bug in older content). Duplicate values also crash
			// Svelte's keyed each block in TagInput.
			try {
				const tagsRes = await apiList<ContentTag>(ctx, `/content/${draft.id}/tags`);
				const seen = new Set<string>();
				draftTags = tagsRes.items
					.map((t) => t.slug)
					.filter((slug): slug is string => {
						if (!slug) return false;
						const key = slug.toLowerCase();
						if (seen.has(key)) return false;
						seen.add(key);
						return true;
					});
			} catch {
				draftTags = [];
			}
			// Fetch the cover image (featured_image_id) so the CoverUploader
			// can show the preview. GET /media/{id} returns the MediaAsset
			// with its URL. Falls back to null if the media was deleted.
			if (draft?.featured_image_id) {
				try {
					coverMedia = await apiGet<MediaAsset>(ctx, `/media/${draft.featured_image_id}`);
				} catch {
					coverMedia = null;
				}
			}
			// Fetch the series_id for this content so the series selector
			// restores when editing. GET /content/{id}/series returns the
			// series_id or null if the content is not part of any series.
			try {
				const seriesRes = await apiGet<{ series_id: string | null }>(ctx, `/content/${draft.id}/series`);
				draftSeriesId = seriesRes.series_id ?? null;
			} catch {
				draftSeriesId = null;
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
		draftTags,
		comparisonDetail,
		comparisonTypes,
		dimensionTypes,
		coverMedia,
		draftSeriesId,
		loadError: typesRes.status === 'rejected'
	};
};
