<script lang="ts">
	import { onMount, onDestroy, untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { ComposerState, type ComposerDeps } from './composerState.svelte';
	import { evaluatePublish } from './gate';
	import { resolveType } from './contentTypeRegistry';
	import * as createApi from '$lib/api/create';
	import { classifyError } from '$lib/api/create';
	import { api } from '$lib/api/client';
	import type {
		ContentRecord,
		CustomFieldGroup,
		MeSpace,
		BlogSeries,
		ComposerStatus,
		MediaAsset,
		Visibility,
		ComposerOrganization,
		GuideCategory,
		ComparisonDetail,
		ComparisonType
	} from '$lib/types';
	import type { TypeEntry } from './contentTypeRegistry';
	import SaveBar from './SaveBar.svelte';
	import TitleField from './TitleField.svelte';
	import { suggestSlug } from './slug';
	import PostEditor from './editors/PostEditor.svelte';
	import type { EditorMode } from './editors/PostEditor.svelte';
	import ComparisonBuilder from './editors/ComparisonBuilder.svelte';
	import CreateRightPanel from './panel/CreateRightPanel.svelte';
	import CustomFieldGroupCmp from './panel/CustomFieldGroup.svelte';

	interface Props {
		types: TypeEntry[];
		spaces: MeSpace[];
		fieldGroups: CustomFieldGroup[];
		series: BlogSeries[];
		organizations: ComposerOrganization[];
		draft: ContentRecord | null;
		/** Comparison items+dimensions when restoring a comparison draft
		 *  (loaded from GET /comparisons/{id} in +page.server.ts). */
		comparisonDetail?: ComparisonDetail | null;
		/** Admin-defined comparison types (Section 30) for the type selector. */
		comparisonTypes?: ComparisonType[];
		user: { id: string } | null;
		defaultEditor?: EditorMode;
		/** Max tags per content item, from system setting content.max_tags (default 5). */
		maxTags?: number;
	}

	let { types, spaces, fieldGroups, series = $bindable(), organizations, draft, comparisonDetail = null, comparisonTypes = [], user, defaultEditor = 'block', maxTags = 5 }: Props = $props();

	// ---- form state ----
	let selectedType = $state<TypeEntry>(
		untrack(() => {
			if (draft?.content_type) {
				const found = types.find((t) => t.value === draft.content_type);
				if (found) return found;
			}
			return types[0] ?? resolveType('blog_post');
		})
	);
	let title = $state(untrack(() => draft?.title ?? ''));
	let slug = $state(untrack(() => draft?.slug ?? ''));
	let slugTouched = $state(untrack(() => Boolean(draft?.slug)));
	let bodyHtml = $state(untrack(() => draft?.body_html ?? ''));
	let spaceId = $state<string | null>(untrack(() => draft?.space_id ?? spaces.find((s) => s.is_primary)?.id ?? null));
	// Post-as-org (ADR-062): null = personal post; an org id = post on behalf of
	// that org. Restored from draft so edit-as-org round-trips the assignment.
	let organizationId = $state<string | null>(untrack(() => draft?.organization_id ?? null));
	let visibility = $state<Visibility>(untrack(() => (draft?.visibility as Visibility) ?? 'public'));
	let canonicalUrl = $state(untrack(() => draft?.canonical_url ?? ''));
	let publishAt = $state(untrack(() => draft?.publish_at ?? null));
	let tags = $state<string[]>([]);
	let seriesId = $state<string | null>(null);
	let fieldValues = $state<Record<string, unknown>>(untrack(() => draft?.field_values ?? {}));
	let coverMediaId = $state<string | null>(null);
	let coverPreviewUrl = $state<string | null>(null);
	// SEO + feature image fields (migration 000165)
	let featuredImageId = $state<string | null>(untrack(() => draft?.featured_image_id ?? null));
	let metaTitle = $state<string | null>(untrack(() => draft?.meta_title ?? null));
	let metaDescription = $state<string | null>(untrack(() => draft?.meta_description ?? null));
	let ogImageUrl = $state<string | null>(untrack(() => draft?.og_image_url ?? null));
	let ogImagePreviewUrl = $state<string | null>(null);
	let seoIndexable = $state<boolean>(untrack(() => draft?.seo_indexable ?? true));
	// comparison-only
	let items = $state<{ label: string }[]>([]);
	let dimensions = $state<{ label: string }[]>([]);

	// ---- Guide category state ----
	let guideCategory = $state<string | null>(untrack(() => draft?.guide_category ?? null));
	// Category-specific field groups loaded on-demand when a guide category is selected.
	// These are merged with the base content field groups for rendering in the settings panel.
	let categoryFieldGroups = $state<CustomFieldGroup[]>([]);

	// ---- Guide panel: track which field is focused for contextual help ----
	let focusedField = $state<'space' | 'tags' | 'series' | 'body' | null>(null);

	// ---- autosave machine ----
	let composer: ComposerState | null = $state(null);

	const snapshot = $derived({
		content_type: selectedType.value,
		title,
		slug,
		body_html: bodyHtml,
		field_values: fieldValues,
		canonical_url: canonicalUrl || null,
		featured_image_id: featuredImageId,
		meta_title: metaTitle,
		meta_description: metaDescription,
		og_image_url: ogImageUrl,
		seo_indexable: seoIndexable,
		organization_id: organizationId,
		visibility,
		publish_at: publishAt,
		space_id: spaceId,
		guide_category: guideCategory
	});

	// Merge base content field groups with category-specific ones for the
	// settings panel and required-field validation. Deduplicate by group ID
	// because the base fetch (GET /custom-field-groups?entity_type=content)
	// may already include guide category groups that the category-specific
	// fetch also returns.
	const allFieldGroups = $derived.by(() => {
		const seen = new Set<string>();
		const merged: CustomFieldGroup[] = [];
		for (const g of [...fieldGroups, ...categoryFieldGroups]) {
			// Only include groups that have at least one field defined.
			if (!seen.has(g.id) && g.fields && g.fields.length > 0) {
				seen.add(g.id);
				merged.push(g);
			}
		}
		return merged;
	});

	const requiredFieldsFilled = $derived(
		allFieldGroups
			.flatMap((g) => g.fields)
			.every((f) => !f.is_required || fieldValues[f.key] != null)
	);

	const gate = $derived(
		evaluatePublish({
			title,
			spaceId,
			requiredFieldsFilled,
			formKind: selectedType.formKind,
			itemCount: items.length,
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
		if (!cat) {
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

	async function saveDraft() {
		await composer?.flush();
	}

	async function publish() {
		await composer?.publish();
	}

	/** Ensure the composer has created its content row (POST /content) and
	 *  return the resulting content ID. Used by ComparisonBuilder before
	 *  adding items/dimensions — the items must attach to the same content
	 *  row the composer manages, not a separate one (Section 30). */
	async function ensureContentId(): Promise<string | null> {
		if (!composer) return null;
		if (composer.contentId) return composer.contentId;
		await composer.flush();
		return composer.contentId;
	}
</script>

<div class="composer-grid">
	<div class="composer-main">
		<div class="composer-header">
			<h1 class="composer-title">{t('create.title')}</h1>
			<select
				class="composer-type-select"
				value={selectedType.value}
				onchange={(e) => {
				const found = types.find((ty) => ty.value === e.currentTarget.value);
				selectedType = found ?? resolveType(e.currentTarget.value);
			}}
			>
				{#each types as ty (ty.value)}
					<option value={ty.value}>{ty.label}</option>
				{/each}
			</select>
		</div>

		{#if selectedType.hasCategory && selectedType.categories?.length}
			<div class="guide-category-selector">
				<label for="guide-category" class="guide-category-label">
					{t('create.guide.category_label')}
				</label>
				<select
					id="guide-category"
					class="guide-category-select"
					value={guideCategory ?? ''}
					onchange={(e) => {
						guideCategory = e.currentTarget.value || null;
						markDirty();
					}}
				>
					<option value="">{t('create.guide.category_placeholder')}</option>
					{#each selectedType.categories as cat (cat.Slug)}
						<option value={cat.Slug}>{cat.Label}</option>
					{/each}
				</select>
			</div>
		{/if}

		<TitleField {title} onTitle={handleTitle} />

		{#if selectedType.formKind === 'comparison'}
			<ComparisonBuilder
				contentId={composer?.contentId ?? null}
				onEnsureContent={ensureContentId}
				initialItems={comparisonDetail?.items ?? []}
				initialDimensions={comparisonDetail?.dimensions ?? []}
				comparisonTypes={comparisonTypes}
				{bodyHtml}
				onhtml={(v) => { bodyHtml = v; }}
				{defaultEditor}
				oncounts={(n, d) => { items = new Array(n).fill({ label: '' }); dimensions = new Array(d).fill({ label: '' }); }}
				onedit={markDirty}
			/>
		{:else}
			<PostEditor
				html={bodyHtml}
				onedit={markDirty}
				onhtml={(v) => { bodyHtml = v; }}
				onfocus={() => (focusedField = 'body')}
				defaultMode={defaultEditor}
			/>
		{/if}

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
		/>
	</div>

	<div class="composer-right">
		<CreateRightPanel
			{focusedField}
			{slug}
			{visibility}
			{canonicalUrl}
			{publishAt}
			bind:series
			{seriesId}
			bind:spaces
			{spaceId}
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
			onvisibility={(v) => { visibility = v; markDirty(); }}
			oncanonical={(v) => { canonicalUrl = v; markDirty(); }}
			onpublishat={(v) => { publishAt = v; markDirty(); }}
			onslug={handleSlug}
			onseries={(id) => { seriesId = id; markDirty(); }}
			onseriesfocus={() => (focusedField = 'series')}
			onspace={(id) => { spaceId = id; markDirty(); }}
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
	.composer-type-select {
		padding: 6px 12px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		color: var(--color-text);
		font-size: 0.875rem;
		outline: none;
		cursor: pointer;
		transition: border-color 0.15s, box-shadow 0.15s;
	}
	.composer-type-select:focus {
		border-color: var(--color-primary);
		box-shadow: 0 0 0 3px color-mix(in srgb, var(--color-primary) 15%, transparent);
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
	.guide-category-select {
		width: 100%;
		padding: 10px 14px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		color: var(--color-text);
		font-size: 1rem;
		outline: none;
		transition: border-color 0.15s, box-shadow 0.15s;
	}
	.guide-category-select:focus {
		border-color: var(--color-primary);
		box-shadow: 0 0 0 3px color-mix(in srgb, var(--color-primary) 15%, transparent);
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
