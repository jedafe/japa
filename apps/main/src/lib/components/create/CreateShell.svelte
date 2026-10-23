<script lang="ts">
	import { onMount, onDestroy, untrack } from 'svelte';
	import { page } from '$app/state';
	import { goto, replaceState } from '$app/navigation';
	import { contentHref } from '$lib/utils/contentLink';
	import { t } from '$lib/i18n';
	import { ComposerState, type ComposerDeps } from './composerState.svelte';
	import { evaluatePublish } from './gate';
	import { resolveType } from './contentTypeRegistry';
	import * as createApi from '$lib/api/create';
	import { classifyError } from '$lib/api/create';
	import { api } from '$lib/api/client';
	import { excerpt } from '$lib/utils/seo-score';
	import { localDateTimeToIso, isoToLocalDateTime } from '$lib/utils/datetime';
	import type {
		ContentRecord,
		ContentSpace,
		CustomFieldGroup,
		MeSpace,
		BlogSeries,
		ComposerStatus,
		MediaAsset,
		Visibility,
		ComposerOrganization,
		GuideCategory,
		ComparisonDetail,
		ComparisonType,
		ListFieldValues
	} from '$lib/types';
	import type { TypeEntry } from './contentTypeRegistry';
	import SaveBar from './SaveBar.svelte';
	import TitleField from './TitleField.svelte';
	import { suggestSlug } from './slug';
	import PostEditor from './editors/PostEditor.svelte';
	import type { EditorMode } from './editors/PostEditor.svelte';
	import ComparisonBuilder from './editors/ComparisonBuilder.svelte';
	import ListBuilder from './editors/ListBuilder.svelte';
	import VideoField from './VideoField.svelte';
	import type { VideoSource } from '$lib/types';
	import CreateRightPanel from './panel/CreateRightPanel.svelte';
	import CustomFieldGroupCmp from './panel/CustomFieldGroup.svelte';
	import Autocomplete, { type AutocompleteItem } from './Autocomplete.svelte';
	import { staticAutocompleteFetcher } from '$lib/autocomplete';
	import AiBodyButton from '$lib/ai/AiBodyButton.svelte';
	import AiDraftPanel from '$lib/ai/AiDraftPanel.svelte';
	import { canUseAI } from '$lib/ai/canUseAI';
	import type { SessionUser } from '$lib/types';

	interface Props {
		types: TypeEntry[];
		spaces: MeSpace[];
		fieldGroups: CustomFieldGroup[];
		series: BlogSeries[];
		organizations: ComposerOrganization[];
		draft: ContentRecord | null;
		/** Tag slugs restored from GET /content/{id}/tags so the tag chips
		 *  reappear when editing existing content. Empty for new content.
		 *  Slugs (not names) are required because attachTag/detachTag send
		 *  the value as a slug to the backend. */
		draftTags?: string[];
		/** Comparison items+dimensions when restoring a comparison draft
		 *  (loaded from GET /jce/comparisons/{id} in +page.server.ts). */
		comparisonDetail?: ComparisonDetail | null;
		/** Admin-defined comparison types (Section 30) for the type selector. */
		comparisonTypes?: ComparisonType[];
		/** Valid dimension types from GET /jce/comparisons/dimension-types (Rule 16). */
		dimensionTypes?: { value: string; label: string }[];
		/** Cover image media asset restored from GET /media/{featured_image_id}
		 *  so the CoverUploader shows the preview when editing existing content. */
		coverMedia?: MediaAsset | null;
		/** Series ID restored from GET /content/{id}/series so the series
		 *  selector restores when editing existing content. */
		draftSeriesId?: string | null;
		/** Space assignments restored from GET /content/{id}/spaces so the
		 *  multi-space selector restores when editing existing content. */
		draftSpaces?: ContentSpace[];
		/** §106 video source restored from GET /videos/{id} so the VideoField
		 *  shows the linked source when editing an existing video. */
		videoSource?: VideoSource | null;
		/** Content type value from ?type — used to preselect the composer type
		 *  when creating new content. Ignored when restoring a draft. */
		initialType?: string | null;
		/** Space ID resolved from ?space — used to preselect the space
		 *  when creating new content. Ignored when restoring a draft. */
		initialSpaceId?: string | null;
		user: SessionUser | null;
		defaultEditor?: EditorMode;
		/** Visibility options from GET /content/visibility-options (Rule 16). */
		visibilityOptions?: { value: Visibility; label: string }[];
		/** Max tags per content item, from system setting content.max_tags (default 5). */
		maxTags?: number;
		/** Feature flags map, passed down to gate the AI assistant. */
		features?: Record<string, boolean>;
		/** Readiness dimensions from GET /assessment for the Journey panel (§71). */
		assessmentDimensions: string[];
	}

	let { types, spaces, fieldGroups, series = $bindable(), organizations, draft, draftTags = [], comparisonDetail = null, comparisonTypes = [], dimensionTypes = [], coverMedia = null, draftSeriesId = null, draftSpaces = [], videoSource = null, initialType = null, initialSpaceId = null, user, defaultEditor = 'block', visibilityOptions = [], maxTags = 5, features = {}, assessmentDimensions }: Props = $props();

	// ---- form state ----
	let selectedType = $state<TypeEntry>(
		untrack(() => {
			if (draft?.content_type) {
				const found = types.find((t) => t.value === draft.content_type);
				if (found) return found;
			}
			if (initialType) {
				const found = types.find((t) => t.value === initialType);
				if (found) return found;
			}
			return types[0] ?? resolveType('blog_post');
		})
	);
	let title = $state(untrack(() => draft?.title ?? ''));
	let slug = $state(untrack(() => draft?.slug ?? ''));
	let slugTouched = $state(untrack(() => Boolean(draft?.slug)));
	let bodyHtml = $state(untrack(() => draft?.body_html || draft?.body || ''));
	let spaceIds = $state<string[]>(
		untrack(() => {
			if (draft?.id && draftSpaces.length > 0) {
				return draftSpaces
					.slice()
					.sort((a, b) => Number(b.is_primary) - Number(a.is_primary))
					.map((s) => s.space_id)
					.filter((id) => spaces.some((s) => s.id === id));
			}
			if (initialSpaceId && spaces.some((s) => s.id === initialSpaceId)) return [initialSpaceId];
			const primary = spaces.find((s) => s.is_primary);
			return primary ? [primary.id] : [];
		})
	);
	// Post-as-org (ADR-062): null = personal post; an org id = post on behalf of
	// that org. Restored from draft so edit-as-org round-trips the assignment.
	let organizationId = $state<string | null>(untrack(() => draft?.organization_id ?? null));
	let visibility = $state<Visibility>(untrack(() => (draft?.visibility as Visibility) ?? 'public'));
	let canonicalUrl = $state(untrack(() => draft?.canonical_url ?? ''));
	// draft.publish_at is a zoned ISO instant (timestamptz) — convert to the
	// browser-local "YYYY-MM-DDTHH:mm" the datetime-local input requires.
	let publishAt = $state<string | null>(untrack(() => isoToLocalDateTime(draft?.publish_at)));
	let tags = $state<string[]>(untrack(() => draftTags ?? []));
	let seriesId = $state<string | null>(untrack(() => draftSeriesId ?? null));
	let fieldValues = $state<Record<string, unknown>>(untrack(() => draft?.field_values ?? {}));
	let coverMediaId = $state<string | null>(untrack(() => coverMedia?.id ?? null));
	let coverPreviewUrl = $state<string | null>(untrack(() => coverMedia?.url ?? null));
	// SEO + feature image fields (migration 000165).
	// Per Section 26 ResolveMetadata, empty SEO fields fall back to the
	// content's own title / body excerpt / cover image at render time.
	// Pre-fill the editor inputs with these fallbacks so the user sees
	// what search engines and social scrapers will actually use, and can
	// edit them directly rather than starting from empty fields.
	let featuredImageId = $state<string | null>(untrack(() => draft?.featured_image_id ?? null));
	// §106 video — the resolved/uploaded video_sources row linked to this
	// draft. Set by VideoField; persisted via the snapshot's video_source_id.
	let videoSourceId = $state<string | null>(untrack(() => draft?.video_source_id ?? null));
	let metaTitle = $state<string | null>(
		untrack(() => draft?.meta_title?.trim() || draft?.title?.trim() || null)
	);
	let metaDescription = $state<string | null>(
		untrack(() => draft?.meta_description?.trim() || excerpt(draft?.body_html || draft?.body || '', 160) || null)
	);
	let ogImageUrl = $state<string | null>(
		untrack(() => draft?.og_image_url?.trim() || coverMedia?.url?.trim() || null)
	);
	let ogImagePreviewUrl = $state<string | null>(
		untrack(() => draft?.og_image_url?.trim() || coverMedia?.url?.trim() || null)
	);
	let seoIndexable = $state<boolean>(untrack(() => draft?.seo_indexable ?? true));
	// comparison-only
	let items = $state<{ label: string }[]>([]);
	let dimensions = $state<{ label: string }[]>([]);

	// list-only (§14 list amendment) — field_values is the editing source for
	// the list composer. list_mode/items/intro_html are not custom-field keys,
	// so they bypass activeFieldValues via a direct snapshot spread (same
	// mechanism as the guide dimension/purpose keys). field_values is typed
	// Record<string, unknown> — the double cast is required because
	// ListFieldValues has no index signature (plain `as` fails typecheck).
	let listFieldValues = $state<ListFieldValues>(
		untrack(() =>
			draft?.content_type === 'list'
				? ((draft.field_values as unknown as ListFieldValues | null) ?? {
						list_mode: 'numbered',
						items: []
					})
				: { list_mode: 'numbered', items: [] }
		)
	);
	let listItemCount = $state(untrack(() => listFieldValues.items?.length ?? 0));

	// AI drafting panel (Task 7)
	let aiPanelOpen = $state(false);

	// ---- Guide category state ----
	let guideCategory = $state<string | null>(untrack(() => draft?.guide_category ?? null));
	// Journey-aware recommendation metadata (backend normalizes free-text input).
	let journeyStage = $state<string | null>(untrack(() => draft?.journey_stage ?? null));
	let visaRoute = $state<string | null>(untrack(() => draft?.visa_route ?? null));
	let sourceCountry = $state<string | null>(untrack(() => draft?.source_country ?? null));
	let targetCountry = $state<string | null>(untrack(() => draft?.target_country ?? null));
	let roadmapDimension = $state<string | null>(untrack(() => (draft?.field_values?.dimension as string) ?? null));
	let roadmapPurpose = $state<string | null>(untrack(() => (draft?.field_values?.purpose as string) ?? null));
	// Category-specific field groups loaded on-demand when a guide category is selected.
	// These are merged with the base content field groups for rendering in the settings panel.
	let categoryFieldGroups = $state<CustomFieldGroup[]>([]);

	// Content-type selector options for the header Autocomplete.
	const typeItems = $derived(types.map((ty): AutocompleteItem => ({ id: ty.value, label: ty.label })));

	// ---- Guide panel: track which field is focused for contextual help ----
	let focusedField = $state<'space' | 'tags' | 'series' | 'body' | null>(null);

	// ---- autosave machine ----
	let composer: ComposerState | null = $state(null);

	// The content_type is immutable after the row exists — PATCH /content/{id}
	// carries no content_type field. Switching the selector on an existing
	// piece would re-render the wrong form kind and PATCH the new shape's
	// field_values onto the old type's row (for a stored list that wipes
	// field_values.items — the backend normalizes an absent array to []).
	// Lock the selector once a row exists: a restored draft from the start, a
	// new piece after the first autosave creates it.
	const typeLocked = $derived.by(() => !!draft?.id || !!composer?.contentId);

	// Merge base content field groups with category-specific ones for the
	// settings panel and required-field validation. Deduplicate by group ID
	// because the base fetch (GET /custom-field-groups?entity_type=content)
	// may already include guide category groups that the category-specific
	// fetch also returns.
	const allFieldGroups = $derived.by(() => {
		const seen = new Set<string>();
		const merged: CustomFieldGroup[] = [];
		// Type-scoped groups ({type}_fields — e.g. story_fields) render only
		// for their own content type; category groups merge only while a
		// guide category is selected (categoryFieldGroups is empty otherwise).
		const typeSlug = `${selectedType.value}_fields`;
		for (const g of [...fieldGroups.filter((f) => f.slug === typeSlug), ...categoryFieldGroups]) {
			// Only include groups that have at least one field defined.
			if (!seen.has(g.id) && g.fields && g.fields.length > 0) {
				seen.add(g.id);
				merged.push(g);
			}
		}
		return merged;
	});

	// Only persist field_values for the custom field groups that are active
	// for the current content type. Prevents guide category field values from
	// leaking into other content types.
	const activeFieldKeys = $derived(new Set(allFieldGroups.flatMap((g) => g.fields.map((f) => f.key))));
	const activeFieldValues = $derived.by(() => {
		// Keys saved on this draft whose field group hasn't loaded yet (guide
		// category groups fetch asynchronously on mount) must survive the
		// filter — stripping them would wipe stored values on the next save
		// and permanently block publishing on required fields.
		const draftKeys =
			draft && draft.content_type === selectedType.value
				? new Set(Object.keys(draft.field_values ?? {}))
				: null;
		return Object.fromEntries(
			Object.entries(fieldValues).filter(([k]) => activeFieldKeys.has(k) || draftKeys?.has(k))
		);
	});

	const isGuide = $derived(selectedType.value === 'guide');
	const isVideo = $derived(selectedType.value === 'video');
	const isList = $derived(selectedType.value === 'list');
	const snapshot = $derived({
		content_type: selectedType.value,
		title,
		slug,
		body_html: bodyHtml,
		field_values: {
			...activeFieldValues,
			...(isGuide && roadmapDimension ? { dimension: roadmapDimension } : {}),
			...(isGuide && roadmapPurpose ? { purpose: roadmapPurpose } : {}),
			...(isList ? listFieldValues : {})
		},
		canonical_url: canonicalUrl || null,
		featured_image_id: featuredImageId,
		// §106 video — link the resolved/uploaded video_sources row. Only
		// sent for the video type so other content never carries a stray
		// reference (the backend treats null as "keep existing").
		video_source_id: isVideo ? videoSourceId : null,
		meta_title: metaTitle,
		meta_description: metaDescription,
		og_image_url: ogImageUrl,
		seo_indexable: seoIndexable,
		organization_id: organizationId,
		visibility,
		// Preserve the existing status so PATCH doesn't overwrite it with
		// an empty string (which clears the published/scheduled state).
		// New content defaults to 'draft' via the create handler.
		status: draft?.status ?? 'draft',
		// For already-published content, don't send publish_at — it's the
		// original publish time (in the past) and the PATCH handler rejects
		// past dates with 400 "publish_at_in_past". Only send it when the
		// user is actively scheduling (draft/scheduled content with a
		// future date set via the settings panel). The raw value is a
		// datetime-local string — normalise to an ISO instant (the API
		// stores timestamptz and only accepts zoned formats).
		publish_at: draft?.status === 'published' ? null : localDateTimeToIso(publishAt),
		space_id: spaceIds[0] ?? null,
		guide_category: isGuide ? guideCategory : null,
		// Journey fields apply to every content type — source/target country
		// feed geo recommendations and {{Source_Country}}/{{Destination_Country}}
		// literals, and journey_stage/visa_route feed §71 roadmap matching.
		journey_stage: journeyStage,
		visa_route: visaRoute,
		source_country: sourceCountry,
		target_country: targetCountry
	});

	const requiredFieldsFilled = $derived(
		allFieldGroups
			.flatMap((g) => g.fields)
			.every((f) => !f.is_required || fieldValues[f.key] != null)
	);

	const gate = $derived(
		evaluatePublish({
			title,
			primarySpaceId: spaceIds[0] ?? null,
			requiredFieldsFilled,
			formKind: selectedType.formKind,
			itemCount: isList ? listItemCount : items.length,
			dimensionCount: dimensions.length
		})
	);

	onMount(() => {
		if (!user) return;
		const deps: ComposerDeps = {
			create: createApi.createContent,
			patch: createApi.patchContent,
			publish: createApi.publishContent,
			schedule: createApi.scheduleContent,
			attachTag: createApi.attachTag,
			detachTag: createApi.detachTag,
			linkSeries: createApi.linkSeries,
			unlinkSeries: createApi.unlinkSeries,
			classify: classifyError
		};
		composer = new ComposerState(deps, {
			getSnapshot: () => snapshot,
			canPublish: () => gate.ok,
			canSave: () => title.trim().length > 0,
			getPublishAt: () => publishAt
		});
		// When restoring a draft, seed the composer with the existing content
		// ID so it PATCHes instead of creating a duplicate content row.
		if (draft?.id) {
			composer.contentId = draft.id;
			composer.status = 'saved';
		}
		// Replay the current tags/series into the freshly-created composer so
		// they are reconciled after the first create. (Fix: wire tags + series.)
		composer.setTags(tags);
		composer.setSeries(seriesId);
	});

	onDestroy(() => composer?.dispose());

	// Wire tags and series to the composer whenever they change — without
	// these the UI collects tags/series but never sends them to the backend.
	// (Fix: wire tags + series to composer reconciliation.)
	$effect(() => {
		composer?.setTags(tags);
	});
	$effect(() => {
		composer?.setSeries(seriesId);
	});

	// Load category-specific custom field groups when a guide category is
	// selected. The backend scopes field groups via entity_class =
	// "content_type:guide:{category}" in custom_field_group_assignments.
	// The GET /custom-field-groups?entity_class=... endpoint returns only
	// groups assigned to that entity_class.
	$effect(() => {
		const cat = guideCategory;
		const type = selectedType.value;
		if (!cat || type !== 'guide') {
			categoryFieldGroups = [];
			return;
		}
		void (async () => {
			const res = await api.get<CustomFieldGroup[]>(
				`/custom-field-groups?entity_class=${encodeURIComponent('content_type:guide:' + cat)}`
			);
			if (!res.error && res.data) {
				categoryFieldGroups = res.data;
			}
		})();
	});

	// Guide category picker — static option list from the content type's
	// categories, so a client-side fetcher is enough.
	const guideCategoryItems = $derived<AutocompleteItem[]>(
		(selectedType.categories ?? []).map((c) => ({ id: c.Slug, label: c.Label }))
	);
	const guideCategoryFetcher = $derived(staticAutocompleteFetcher(guideCategoryItems));
	// Falls back to the raw slug for saved categories no longer in the list.
	const guideCategoryLabel = $derived(
		guideCategoryItems.find((c) => c.id === guideCategory)?.label ?? guideCategory
	);

	function markDirty() {
		composer?.markDirty();
	}

	function handleTitle(v: string) {
		title = v;
		if (!slugTouched) slug = suggestSlug(v);
		markDirty();
	}

	function handleSlug(v: string) {
		slugTouched = true;
		slug = suggestSlug(v);
		markDirty();
	}

	async function saveSpaces(): Promise<void> {
		const id = composer?.contentId;
		const primary = spaceIds[0];
		if (!id || !primary) return;
		const additional = spaceIds.slice(1);
		const res = await createApi.patchContentSpaces(id, {
			primary_space_id: primary,
			additional_space_ids: additional
		});
		if (res.error) {
			composer?.setError(res.status, res.error?.message ?? 'Failed to save spaces');
		}
	}

	async function saveDraft() {
		await composer?.flush();
		await saveSpaces();
	}

	async function publish() {
		// On an already-scheduled draft, "Publish" means go live now — clear
		// the restored schedule so the composer calls /publish instead of
		// re-saving publish_at through /schedule (a no-op-looking resched).
		if (draft?.status === 'scheduled' && publishAt) publishAt = null;
		await composer?.flush();
		await saveSpaces();
		const ok = await composer?.publish();
		// Land on the published piece instead of silently staying in the
		// composer — the reader now expects to see it live. Only on an
		// actual publish: scheduled content isn't served on the canonical
		// route yet, and a missing slug means the href can't be trusted.
		if (ok && composer?.status === 'published' && slug) {
			await goto(contentHref({ slug, content_type: selectedType.value }));
		}
	}

	/** Ensure the composer has created its content row (POST /content) and
	 *  return the resulting content ID. Used by ComparisonBuilder before
	 *  adding items/dimensions — the items must attach to the same content
	 *  row the composer manages, not a separate one (Section 30). */
	async function ensureContentId(): Promise<string | null> {
		if (!composer) return null;
		if (composer.contentId) return composer.contentId;
		await composer.flush();
		await saveSpaces();
		return composer.contentId;
	}

	// ---- video composer (§106) ----
	// PostEditor instance — exposes appendVideoNode() for inserting the
	// backend-built `video` TipTap node after a resolve/upload.
	let postEditor = $state<{ appendVideoNode: (json: Record<string, unknown>) => void } | null>(null);

	function handleVideoBodyAppend(json: Record<string, unknown>) {
		postEditor?.appendVideoNode(json);
		markDirty();
	}

	function handleVideoResolved(src: VideoSource) {
		// VideoField already wrote the bound fields (title, description,
		// featuredImageId, videoSourceId). Keep the slug in sync when the
		// title was auto-filled (title === src.title) and the user hasn't
		// manually edited the slug.
		if (src.title && title === src.title && !slugTouched) {
			slug = suggestSlug(src.title);
		}
		if (src.thumbnail_url && !coverPreviewUrl) {
			coverPreviewUrl = src.thumbnail_url;
		}
		if (src.thumbnail_media_id && !coverMediaId) {
			coverMediaId = src.thumbnail_media_id;
		}
		markDirty();
	}
</script>

{#snippet aiButtonSnippet()}
	{#if canUseAI(user, features)}
		<AiBodyButton floating onclick={() => (aiPanelOpen = true)} />
	{/if}
{/snippet}

<div class="composer-grid">
	<div class="composer-main">
		<div class="composer-header">
			<h1 class="composer-title">{t('create.title')}</h1>
			<div
				class="composer-type-select"
				title={typeLocked ? t('create.type_locked', 'Content type cannot be changed after creation') : undefined}
			>
				<Autocomplete
					disabled={typeLocked}
					placeholder={t('create.type_placeholder', 'Content type')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={staticAutocompleteFetcher(typeItems)}
					initialFetcher={staticAutocompleteFetcher(typeItems)}
					selectedId={selectedType.value}
					selectedLabel={selectedType.label}
					onSelect={(item) => {
						selectedType = types.find((ty) => ty.value === item.id) ?? resolveType(item.id);
						const url = new URL(page.url);
						url.searchParams.set('type', selectedType.value);
						replaceState(url, {});
						markDirty();
					}}
				/>
			</div>
		</div>

		{#if selectedType.hasCategory && selectedType.categories?.length}
			<div class="guide-category-selector">
				<label for="guide-category" class="guide-category-label">
					{t('create.guide.category_label')}
				</label>
				<Autocomplete
					id="guide-category"
					placeholder={t('create.guide.category_placeholder')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={guideCategoryFetcher}
					initialFetcher={guideCategoryFetcher}
					selectedId={guideCategory}
					selectedLabel={guideCategoryLabel}
					onSelect={(item) => { guideCategory = item.id; markDirty(); }}
					onClear={() => { guideCategory = null; markDirty(); }}
				/>
			</div>
		{/if}

		<TitleField {title} onTitle={handleTitle} />

		{#if isVideo}
			<VideoField
				bind:title
				bind:description={metaDescription}
				bind:featuredImageId
				bind:videoSourceId
				initialSource={videoSource}
				onBodyAppend={handleVideoBodyAppend}
				onResolved={handleVideoResolved}
			/>
		{/if}

		{#if selectedType.formKind === 'comparison'}
			<ComparisonBuilder
				contentId={composer?.contentId ?? null}
				onEnsureContent={ensureContentId}
				initialItems={comparisonDetail?.items ?? []}
				initialDimensions={comparisonDetail?.dimensions ?? []}
				initialCells={comparisonDetail?.matrixValues ?? {}}
				{comparisonTypes}
				{dimensionTypes}
				{bodyHtml}
				onhtml={(v) => { bodyHtml = v; }}
				{defaultEditor}
				oncounts={(n, d) => { items = new Array(n).fill({ label: '' }); dimensions = new Array(d).fill({ label: '' }); }}
				onedit={markDirty}
				aiTrigger={aiButtonSnippet}
			/>
		{:else if selectedType.formKind === 'list'}
			<ListBuilder
				initial={draft?.content_type === 'list' ? (draft.field_values as unknown as ListFieldValues) : null}
				defaultEditor={defaultEditor}
				{features}
				onvalues={(fv) => { listFieldValues = fv; }}
				onhtml={(v) => { bodyHtml = v; }}
				oncounts={(n) => { listItemCount = n; }}
				onedit={markDirty}
			/>
		{:else}
			<PostEditor
				bind:this={postEditor}
				html={bodyHtml}
				onedit={markDirty}
				onhtml={(v) => { bodyHtml = v; }}
				onfocus={() => (focusedField = 'body')}
				defaultMode={defaultEditor}
				aiTrigger={aiButtonSnippet}
			/>
		{/if}

		<AiDraftPanel
			open={aiPanelOpen}
			contentType={selectedType.value}
			{title}
			body={bodyHtml}
			fieldSchema={allFieldGroups.flatMap((g) => g.fields).map((f) => ({
				key: f.key,
				type: f.field_type,
				label: f.label,
				options: Array.isArray(f.options)
					? f.options.map((o) => (typeof o === 'string' ? { value: o, label: o } : o))
					: undefined,
			}))}
			onApply={(payload) => {
				if (payload.title) {
					title = payload.title;
					handleTitle(payload.title);
				}
				if (payload.body !== undefined) {
					bodyHtml = payload.body;
				}
				if (payload.fieldValues) {
					fieldValues = { ...fieldValues, ...payload.fieldValues };
				}
				// §20 assist actions: summarize→meta description, og→meta
				// title+description, generate-tags→merge into the tag field
				// (slugs, deduped, capped at maxTags).
				if (payload.metaTitle) metaTitle = payload.metaTitle;
				if (payload.metaDescription) metaDescription = payload.metaDescription;
				if (payload.tags && payload.tags.length > 0) {
					tags = [...new Set([...tags, ...payload.tags])].slice(0, maxTags);
				}
				markDirty();
			}}
			onClose={() => (aiPanelOpen = false)}
		/>

		{#if allFieldGroups.length > 0}
			<div class="composer-field-groups">
				{#each allFieldGroups as group (group.id)}
					<div class="composer-field-group">
						<h4 class="composer-field-group-title">{group.name}</h4>
						<CustomFieldGroupCmp
							fields={group.fields}
							values={fieldValues}
							onvalues={(v) => { fieldValues = v; markDirty(); }}
							onedit={markDirty}
						/>
					</div>
				{/each}
			</div>
		{/if}

		<SaveBar
			status={composer?.status ?? ('pristine' as ComposerStatus)}
			savedAt={composer?.savedAt ?? null}
			canPublish={gate.ok}
			publishReason={gate.reason}
			orgs={organizations}
			selectedOrgId={organizationId}
			onSelectOrg={(id) => { organizationId = id; markDirty(); }}
			onSaveDraft={saveDraft}
			onPublish={publish}
			isPublished={draft?.status === 'published'}
		/>
	</div>

	<div class="composer-right">
		<CreateRightPanel
			{focusedField}
			{journeyStage}
			{visaRoute}
			{sourceCountry}
			{targetCountry}
			dimension={roadmapDimension}
			purpose={roadmapPurpose}
			assessmentDimensions={assessmentDimensions}
			onjourney={(v) => { journeyStage = v; markDirty(); }}
			onvisa={(v) => { visaRoute = v; markDirty(); }}
			onsource={(v) => { sourceCountry = v; markDirty(); }}
			ontarget={(v) => { targetCountry = v; markDirty(); }}
			ondimension={(v) => { roadmapDimension = v; markDirty(); }}
			onpurpose={(v) => { roadmapPurpose = v; markDirty(); }}
			{slug}
			{visibility}
			{canonicalUrl}
			{publishAt}
			{visibilityOptions}
			bind:series
			{seriesId}
			bind:spaces
			{spaceIds}
			{tags}
			{maxTags}
			{coverMediaId}
			{coverPreviewUrl}
			{metaTitle}
			{metaDescription}
			{ogImageUrl}
			{ogImagePreviewUrl}
			{seoIndexable}
			{title}
			{bodyHtml}
			contentType={selectedType.value}
			typeLabel={selectedType.label}
			supportsSeries={selectedType.supportsSeries}
			onvisibility={(v) => { visibility = v; markDirty(); }}
			oncanonical={(v) => { canonicalUrl = v; markDirty(); }}
			onpublishat={(v) => { publishAt = v; markDirty(); }}
			onslug={handleSlug}
			onseries={(id) => { seriesId = id; markDirty(); }}
			onseriesfocus={() => (focusedField = 'series')}
			onspace={(ids) => { spaceIds = ids; markDirty(); }}
			onspacefocus={() => (focusedField = 'space')}
			ontags={(next) => { tags = next; markDirty(); }}
			ontagsfocus={() => (focusedField = 'tags')}
			oncoveruploaded={(asset: MediaAsset) => {
				coverMediaId = asset.id;
				coverPreviewUrl = asset.url;
				featuredImageId = asset.id;
				markDirty();
			}}
			oncoverremove={() => {
				coverMediaId = null;
				coverPreviewUrl = null;
				featuredImageId = null;
				markDirty();
			}}
			onmetatitle={(v) => { metaTitle = v; markDirty(); }}
			onmetadescription={(v) => { metaDescription = v; markDirty(); }}
			onogimageselect={(asset: MediaAsset | null) => {
				ogImageUrl = asset?.url ?? null;
				ogImagePreviewUrl = asset?.url ?? null;
				markDirty();
			}}
			onseoindexable={(v) => { seoIndexable = v; markDirty(); }}
			onedit={markDirty}
		/>
	</div>
</div>

<style>
	.composer-grid {
		display: grid;
		grid-template-columns: 1fr 320px;
		gap: 1.5rem;
		max-width: 1200px;
		margin: 0 auto;
		padding: 1.5rem;
	}

	.composer-main {
		display: flex;
		flex-direction: column;
		gap: 1rem;
		min-width: 0;
	}
	.composer-field-groups {
		display: flex;
		flex-direction: column;
		gap: 1rem;
		padding: 1rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
	}
	.composer-field-group {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}
	.composer-field-group-title {
		margin: 0;
		font-size: 0.875rem;
		font-weight: 600;
		color: var(--color-text);
	}
	.composer-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
	}
	.composer-title {
		margin: 0;
		font-size: 1.125rem;
		font-weight: 600;
		color: var(--color-text);
	}
	/* The type selector is an Autocomplete — this wrapper only constrains
	 * its width; the field chrome lives in Autocomplete's own styles. */
	.composer-type-select {
		width: 15rem;
		flex-shrink: 0;
	}
	.guide-category-selector {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}
	.guide-category-label {
		font-size: 0.75rem;
		font-weight: 500;
		color: var(--color-muted);
	}
	.composer-right {
		min-width: 0;
	}
	@media (max-width: 1023px) {
		.composer-grid {
			grid-template-columns: 1fr;
		}
	}
</style>
