<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
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
		Visibility
	} from '$lib/types';
	import type { TypeEntry } from './contentTypeRegistry';
	import SaveBar from './SaveBar.svelte';
	import TitleField from './TitleField.svelte';
	import SpaceSelector from './SpaceSelector.svelte';
	import TagInput from './TagInput.svelte';
	import CoverUploader from './CoverUploader.svelte';
	import PostEditor from './editors/PostEditor.svelte';
	import ComparisonBuilder from './editors/ComparisonBuilder.svelte';
	import CreateRightPanel from './panel/CreateRightPanel.svelte';

	interface Props {
		types: TypeEntry[];
		spaces: MeSpace[];
		fieldGroups: CustomFieldGroup[];
		series: BlogSeries[];
		draft: ContentRecord | null;
		user: { id: string } | null;
	}

	let { types, spaces, fieldGroups, series, draft, user }: Props = $props();

	// ---- form state ----
	let selectedType = $state<TypeEntry>(types[0] ?? resolveType('blog_post'));
	let title = $state(draft?.title ?? '');
	let slug = $state(draft?.slug ?? '');
	let bodyHtml = $state(draft?.body_html ?? '');
	let spaceId = $state<string | null>(draft?.space_id ?? spaces.find((s) => s.is_primary)?.id ?? null);
	let visibility = $state<Visibility>((draft?.visibility as Visibility) ?? 'public');
	let canonicalUrl = $state(draft?.canonical_url ?? '');
	let publishAt = $state(draft?.publish_at ?? null);
	let tags = $state<string[]>([]);
	let seriesId = $state<string | null>(null);
	let fieldValues = $state<Record<string, unknown>>(draft?.field_values ?? {});
	let coverMediaId = $state<string | null>(null);
	// comparison-only
	let items = $state<{ label: string }[]>([]);
	let dimensions = $state<{ label: string }[]>([]);

	// ---- autosave machine ----
	let composer: ComposerState | null = $state(null);

	const snapshot = $derived({
		content_type: selectedType.value,
		title,
		slug,
		body_html: bodyHtml,
		field_values: fieldValues,
		canonical_url: canonicalUrl || null,
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
			getPublishAt: () => publishAt
		});
	});

	onDestroy(() => composer?.dispose());

	function markDirty() {
		composer?.markDirty();
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
			<h1 class="text-lg font-semibold">{t('create.title')}</h1>
			<select
				class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm"
				value={selectedType.value}
				onchange={(e) => (selectedType = resolveType(e.currentTarget.value))}
			>
				{#each types as ty (ty.value)}
					<option value={ty.value}>{ty.label}</option>
				{/each}
			</select>
		</div>

		<TitleField {title} {slug} onTitle={(v) => { title = v; markDirty(); }} onSlug={(v) => { slug = v; markDirty(); }} />

		<SpaceSelector
			{spaces}
			selectedId={spaceId}
			onSelect={(id) => { spaceId = id; markDirty(); }}
		/>

		<TagInput {tags} onTags={(next) => { tags = next; markDirty(); }} />

		<CoverUploader onUploaded={(asset: MediaAsset) => { coverMediaId = asset.id; markDirty(); }} />

		{#if selectedType.formKind === 'comparison'}
			<ComparisonBuilder
				{title}
				{spaceId}
				oncounts={(n, d) => { items = new Array(n).fill({ label: '' }); dimensions = new Array(d).fill({ label: '' }); }}
				onedit={markDirty}
			/>
		{:else}
			<PostEditor html={bodyHtml} onedit={markDirty} />
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
			focusedField={null}
			{visibility}
			{canonicalUrl}
			{publishAt}
			{fieldValues}
			{fieldGroups}
			{series}
			{seriesId}
			onvisibility={(v) => { visibility = v; markDirty(); }}
			oncanonical={(v) => { canonicalUrl = v; markDirty(); }}
			onpublishat={(v) => { publishAt = v; markDirty(); }}
			onfieldvalues={(v) => { fieldValues = v; markDirty(); }}
			onseries={(id) => { seriesId = id; markDirty(); }}
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
	.composer-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
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
