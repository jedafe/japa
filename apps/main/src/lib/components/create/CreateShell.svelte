<script lang="ts">
	import { onMount, onDestroy, untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { ComposerState, type ComposerDeps } from './composerState.svelte';
	import { evaluatePublish } from './gate';
	import { resolveType } from './contentTypeRegistry';
	import * as createApi from '$lib/api/create';
	import { classifyError } from '$lib/api/create';
	import type {
		ContentRecord,
		CustomFieldGroup,
		MeSpace,
		BlogSeries,
		ComposerStatus,
		MediaAsset,
		Visibility,
		ComposerOrganization
	} from '$lib/types';
	import type { TypeEntry } from './contentTypeRegistry';
	import SaveBar from './SaveBar.svelte';
	import TitleField from './TitleField.svelte';
	import { suggestSlug } from './slug';
	import SpaceSelector from './SpaceSelector.svelte';
	import OrgSelector from './OrgSelector.svelte';
	import TagInput from './TagInput.svelte';
	import PostEditor from './editors/PostEditor.svelte';
	import type { EditorMode } from './editors/PostEditor.svelte';
	import ComparisonBuilder from './editors/ComparisonBuilder.svelte';
	import CreateRightPanel from './panel/CreateRightPanel.svelte';

	interface Props {
		types: TypeEntry[];
		spaces: MeSpace[];
		fieldGroups: CustomFieldGroup[];
		series: BlogSeries[];
		organizations: ComposerOrganization[];
		draft: ContentRecord | null;
		user: { id: string } | null;
		defaultEditor?: EditorMode;
	}

	let { types, spaces, fieldGroups, series = $bindable(), organizations, draft, user, defaultEditor = 'block' }: Props = $props();

	// ---- form state ----
	let selectedType = $state<TypeEntry>(untrack(() => types[0] ?? resolveType('blog_post')));
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
		space_id: spaceId
	});

	const requiredFieldsFilled = $derived(
		fieldGroups
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

		<TitleField {title} onTitle={handleTitle} />

		<div class="row-pair">
			<SpaceSelector
				{spaces}
				selectedId={spaceId}
				onSelect={(id) => { spaceId = id; markDirty(); }}
				onfocus={() => (focusedField = 'space')}
			/>
			<TagInput {tags} onTags={(next) => { tags = next; markDirty(); }} onfocus={() => (focusedField = 'tags')} />
		</div>

		<OrgSelector
			orgs={organizations}
			selectedId={organizationId}
			onSelect={(id) => { organizationId = id; markDirty(); }}
		/>

		{#if selectedType.formKind === 'comparison'}
			<ComparisonBuilder
				{title}
				{spaceId}
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

		<SaveBar
			status={composer?.status ?? ('pristine' as ComposerStatus)}
			savedAt={composer?.savedAt ?? null}
			canPublish={gate.ok}
			publishReason={gate.reason}
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
			{fieldValues}
			{fieldGroups}
			bind:series
			{seriesId}
			{coverMediaId}
			{coverPreviewUrl}
			{metaTitle}
			{metaDescription}
			{ogImageUrl}
			{ogImagePreviewUrl}
			{seoIndexable}
			onvisibility={(v) => { visibility = v; markDirty(); }}
			oncanonical={(v) => { canonicalUrl = v; markDirty(); }}
			onpublishat={(v) => { publishAt = v; markDirty(); }}
			onfieldvalues={(v) => { fieldValues = v; markDirty(); }}
			onslug={handleSlug}
			onseries={(id) => { seriesId = id; markDirty(); }}
			onseriesfocus={() => (focusedField = 'series')}
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
	.row-pair {
		display: flex;
		gap: 0.75rem;
		min-width: 0;
	}
	.row-pair > :global(*) {
		flex: 1 1 0;
		min-width: 0;
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
	.composer-right {
		min-width: 0;
	}
	@media (max-width: 1023px) {
		.composer-grid {
			grid-template-columns: 1fr;
		}
	}
</style>
