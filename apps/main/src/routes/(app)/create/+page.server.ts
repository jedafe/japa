import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import { createAvailableTypes, CREATE_ALLOWED_TYPES } from '$lib/components/create/contentTypeRegistry';
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
	ContentSpace,
	MediaAsset,
	Visibility,
	JCEComparison
} from '$lib/types';

/** Load everything the composer needs; every fetch degrades independently. */
export const load: PageServerLoad = async (event) => {
	// /create is auth-gated — a guest has no spaces, drafts, or series to compose with.
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}

	const ctx = apiContext(event);
	const draftId = event.url.searchParams.get('draft');

	const [
		typesRes,
		flagsRes,
		spacesRes,
		groupsRes,
		seriesRes,
		orgsRes,
		compTypesRes,
		dimTypesRes,
		visibilityRes
	] = await Promise.allSettled([
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
		apiList<ComparisonType>(ctx, '/jce/comparison-types?page_size=50'),
		// Dimension types (Section 30) — for the dimension type selector
		// in the ComparisonBuilder (Rule 16 — no hardcoded enums).
		apiList<{ value: string; label: string }>(ctx, '/jce/comparisons/dimension-types'),
		// Visibility options (Section 14) — canonical visibility values for
		// the composer settings panel (Rule 16 — no hardcoded option lists).
		apiGet<{ value: Visibility; label: string }[]>(ctx, '/content/visibility-options')
	]);

	const rawTypes = typesRes.status === 'fulfilled' ? typesRes.value.items : [];
	const flags = flagsRes.status === 'fulfilled' ? flagsRes.value : {};
	const spaces = spacesRes.status === 'fulfilled' ? spacesRes.value.items : [];

	// Blog posts, comparisons, guides, and stories are authored through
	// /create (stories use the standard composer; the backend syncs
	// story_meta and routes publishes into §79 moderation). Pages are
	// created in the admin pagebuilder; discussions are created in the forum
	// composer (ComposeModal -> DiscussionComposer or /forum/[category]/new).
	// Invalid ?type values are ignored and fall back to the first allowed type.
	const requestedType = event.url.searchParams.get('type') ?? '';
	const initialType = CREATE_ALLOWED_TYPES.some((t) => t === requestedType) ? requestedType : null;

	// ?space is a space slug. Resolve it to the user's joined space id so the
	// composer can pre-select the space. Unknown / non-joined slugs are ignored.
	const requestedSpaceSlug = event.url.searchParams.get('space') ?? '';
	const initialSpaceId = requestedSpaceSlug
		? (spaces.find((s) => s.slug === requestedSpaceSlug)?.id ?? null)
		: null;
	// Only load field groups that match the content types the /create composer
	// actually supports. Job listing and forum fields have their own creation
	// surfaces and should not show up here.
	const allowedFieldGroupSlugs = new Set<string>(CREATE_ALLOWED_TYPES.map((t) => `${t}_fields`));
	const fieldGroups =
		groupsRes.status === 'fulfilled'
			? groupsRes.value.items.filter(
					(g) =>
						g.entity_type === 'content' &&
						allowedFieldGroupSlugs.has(g.slug ?? '') &&
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
	const visibilityOptions =
		visibilityRes.status === 'fulfilled' ? visibilityRes.value : [];

	let draft: ContentRecord | null = null;
	let comparisonDetail: ComparisonDetail | null = null;
	let draftTags: string[] = [];
	let coverMedia: MediaAsset | null = null;
	let draftSeriesId: string | null = null;
	let draftSpaces: ContentSpace[] = [];
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
			// from the JCE endpoint so the ComparisonBuilder can render them.
			if (draft?.content_type === 'comparison') {
				try {
					const jce = await apiGet<JCEComparison>(ctx, `/jce/comparisons/${draft.id}`);
					comparisonDetail = {
						id: jce.content.id,
						tenant_id: jce.content.tenant_id,
						author_id: jce.content.author_id,
						title: jce.content.title,
						slug: jce.content.slug,
						body: jce.content.body,
						visibility: jce.content.visibility,
						status: jce.content.status,
						space_id: jce.content.space_id,
						items: jce.items,
						dimensions: jce.dimensions
					};
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
			// Fetch the content's assigned spaces so the multi-space selector
			// can restore primary + additional spaces when editing.
			// GET /content/{id}/spaces returns { data: [{ space_id, is_primary }] }.
			try {
				const spacesRes = await apiGet<{ data: ContentSpace[] }>(ctx, `/content/${draft.id}/spaces`);
				draftSpaces = spacesRes.data ?? [];
			} catch {
				draftSpaces = [];
			}
		} catch {
			draft = null;
		}
	}

	return {
		types: createAvailableTypes(rawTypes, flags),
		spaces,
		fieldGroups,
		series,
		organizations,
		draft,
		draftTags,
		comparisonDetail,
		comparisonTypes,
		dimensionTypes,
		visibilityOptions,
		coverMedia,
		draftSeriesId,
		draftSpaces,
		initialType,
		initialSpaceId,
		loadError: typesRes.status === 'rejected'
	};
};
