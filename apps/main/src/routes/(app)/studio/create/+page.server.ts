import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import { createAvailableTypes, CREATE_ALLOWED_TYPES } from '$lib/components/create/contentTypeRegistry';
import type { MatrixItem } from '$lib/types/jce';
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
	JCEComparison,
	AssessmentForm,
	VideoSource
} from '$lib/types';

/** Load everything the composer needs; every fetch degrades independently. */
export const load: PageServerLoad = async (event) => {
	// /create is auth-gated — a guest has no spaces, drafts, or series to compose with.
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}

	// Shell data (feature flags and public site settings) is fetched by the
	// root layout and reused here so the composer can gate the §106 video
	// content type on the public system setting video.enabled (Rule 16).
	const parent = await event.parent();
	const flags = (parent as { features?: Record<string, boolean> }).features ?? {};
	const rawSiteSettings = (parent as { siteSettings?: Record<string, unknown> }).siteSettings ?? {};
	// video.enabled defaults to false in the backend catalog; a missing public
	// setting must be treated as disabled (not "missing = enabled").
	const siteSettings = { ...rawSiteSettings };
	if (siteSettings['video.enabled'] !== true) {
		siteSettings['video.enabled'] = false;
	}

	const ctx = apiContext(event);
	const draftId = event.url.searchParams.get('draft');

	const [
		typesRes,
		spacesRes,
		groupsRes,
		seriesRes,
		orgsRes,
		compTypesRes,
		dimTypesRes,
		visibilityRes,
		assessmentRes
	] = await Promise.allSettled([
		apiList<ContentTypeMeta>(ctx, '/content-types'),
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
		apiGet<{ value: Visibility; label: string }[]>(ctx, '/content/visibility-options'),
		// Active assessment form and derived dimensions (§71) for the Journey panel.
		apiGet<AssessmentForm>(ctx, '/assessment')
	]);

	const rawTypes = typesRes.status === 'fulfilled' ? typesRes.value.items : [];
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
						// Exclude guide category-specific field groups (slug =
						// "guide_{category}_fields") — those are loaded on demand by
						// CreateShell when a guide category is selected, via GET
						// /custom-field-groups?entity_class=content_type:guide:{cat}.
						// The base "guide_fields" group must NOT be excluded — the
						// pattern matches it too, so exclude it explicitly.
						!(g.slug !== 'guide_fields' && g.slug?.startsWith('guide_') && g.slug?.endsWith('_fields'))
				)
			: [];
	const series = seriesRes.status === 'fulfilled' ? seriesRes.value.items : [];
	const organizations = orgsRes.status === 'fulfilled' ? orgsRes.value.items : [];
	// §30 — only offer types the caller's group has can_create access to;
	// system-managed types (e.g. cost_of_living) are filled by the platform.
	const comparisonTypes =
		compTypesRes.status === 'fulfilled'
			? compTypesRes.value.items.filter((t) => t.can_create !== false)
			: [];
	const dimensionTypes = dimTypesRes.status === 'fulfilled' ? dimTypesRes.value.items : [];
	const visibilityOptions =
		visibilityRes.status === 'fulfilled' ? visibilityRes.value : [];
	const assessment = assessmentRes.status === 'fulfilled' ? assessmentRes.value : null;

	let draft: ContentRecord | null = null;
	let comparisonDetail: ComparisonDetail | null = null;
	let draftTags: string[] = [];
	let coverMedia: MediaAsset | null = null;
	let draftSeriesId: string | null = null;
	let draftSpaces: ContentSpace[] = [];
	let videoSource: VideoSource | null = null;
	if (draftId) {
		try {
			// Ownership-scoped: GET /me/content/{id} returns the caller's own
			// content by ID (any status — published, scheduled, or draft),
			// author scoped. The public GET /content/{slug} resolves
			// by slug and is not ownership-scoped, so it cannot safely serve
			// the editor restore. The status='draft'-scoped /me/drafts/{id}
			// only covers drafts; /me/content/{id} covers the rest so the
			// Edit button on published content detail pages works too.
			// For non-owners (super-admin or `content.edit` RBAC holder), the
			// backend falls back to a platform-scoped fetch — see GetMyContent.
			// ?raw keeps authored {{...}} literals unresolved — the literal
			// middleware would otherwise rewrite the JSON response and the
			// next save would persist resolved text over the source.
			draft = await apiGet<ContentRecord>(ctx, `/me/content/${draftId}?raw`);
			// When the draft is a comparison, also fetch its items/dimensions
			// from the JCE endpoint so the ComparisonBuilder can render them.
			if (draft?.content_type === 'comparison') {
				try {
					// ?raw=1 keeps authored {{...}} literals unresolved so the
					// editor restores the original template source (the resolved
					// display payload would be persisted on the next save).
					const jce = await apiGet<JCEComparison>(ctx, `/jce/comparisons/${draft.id}?raw=1`);
					comparisonDetail = {
						id: jce.content.id,
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
					// §30 — restore the existing comparison matrix so the
					// ComparisonBuilder can display and continue editing cell values.
					try {
						const matrix = await apiGet<MatrixItem[]>(ctx, `/jce/comparisons/${draft.id}/matrix?raw=1`);
						const itemIndex = new Map<string, number>(comparisonDetail.items.map((it, i) => [it.id, i]));
						const dimIndex = new Map<string, number>(
							comparisonDetail.dimensions.map((d, i) => [d.id, i])
						);
						const matrixValues: Record<string, string> = {};
						for (const row of matrix ?? []) {
							const itemIdx = itemIndex.get(row.id);
							if (itemIdx === undefined) continue;
							for (const cell of row.dimensions) {
								const dimIdx = dimIndex.get(cell.dimension_id);
								if (dimIdx === undefined) continue;
								let v = '';
								if (cell.type === 'rating' && cell.value_rating != null) {
									v = String(cell.value_rating);
								} else if (cell.type === 'boolean' && cell.value_boolean != null) {
									v = String(cell.value_boolean);
								} else if (cell.value != null) {
									v = String(cell.value);
								}
								if (v !== '') {
									matrixValues[`${dimIdx}:${itemIdx}`] = v;
								}
							}
						}
						comparisonDetail.matrixValues = matrixValues;
					} catch {
						// Degrade gracefully — missing matrix values is not a blocker
						// for editing item/dimension metadata and body.
					}
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
			// §106 video — restore the linked video source so the VideoField
			// can show the thumbnail/title when editing an existing video.
			if (draft?.video_source_id) {
				try {
					videoSource = await apiGet<VideoSource>(ctx, `/videos/${draft.video_source_id}`);
				} catch {
					videoSource = null;
				}
			}
		} catch {
			draft = null;
		}
	}

	return {
		types: createAvailableTypes(rawTypes, flags, siteSettings),
		spaces,
		fieldGroups,
		series,
		organizations,
		draft,
		draftTags,
		videoSource,
		comparisonDetail,
		comparisonTypes,
		dimensionTypes,
		visibilityOptions,
		assessmentDimensions: assessment?.dimensions ?? [],
		coverMedia,
		draftSeriesId,
		draftSpaces,
		initialType,
		initialSpaceId,
		loadError: typesRes.status === 'rejected'
	};
};
