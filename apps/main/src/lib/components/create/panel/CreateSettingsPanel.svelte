<script lang="ts">
	import { t } from '$lib/i18n';
	import { ImagePlus, X } from 'lucide-svelte';
	import type { BlogSeries, CustomFieldGroup, MediaAsset, Visibility } from '$lib/types';
	import SeriesSelector from './SeriesSelector.svelte';
	import CustomFieldGroupCmp from './CustomFieldGroup.svelte';
	import MediaLibraryModal from '$lib/components/media/MediaLibraryModal.svelte';

	interface Props {
		visibility: Visibility;
		canonicalUrl: string;
		publishAt: string | null;
		fieldValues: Record<string, unknown>;
		fieldGroups: CustomFieldGroup[];
		series: BlogSeries[];
		seriesId: string | null;
		metaTitle: string | null;
		metaDescription: string | null;
		/** OG image URL stored on the content (sent to the API). */
		ogImageUrl: string | null;
		/** OG image preview URL for rendering (may differ from ogImageUrl when set from library). */
		ogImagePreviewUrl: string | null;
		seoIndexable: boolean;
		onvisibility: (v: Visibility) => void;
		oncanonical: (v: string) => void;
		onpublishat: (v: string | null) => void;
		onfieldvalues: (v: Record<string, unknown>) => void;
		onseries: (id: string | null) => void;
		onseriesfocus: () => void;
		onmetatitle: (v: string | null) => void;
		onmetadescription: (v: string | null) => void;
		/** Called when the user picks/removes an OG image from the library. */
		onogimageselect: (asset: MediaAsset | null) => void;
		onseoindexable: (v: boolean) => void;
		onedit: () => void;
	}

	let {
		visibility,
		canonicalUrl,
		publishAt,
		fieldValues,
		fieldGroups,
		series,
		seriesId,
		metaTitle,
		metaDescription,
		ogImageUrl,
		ogImagePreviewUrl,
		seoIndexable,
		onvisibility,
		oncanonical,
		onpublishat,
		onfieldvalues,
		onseries,
		onseriesfocus,
		onmetatitle,
		onmetadescription,
		onogimageselect,
		onseoindexable,
		onedit
	}: Props = $props();

	let ogPickerOpen = $state(false);

	function handleOgSelect(asset: MediaAsset) {
		ogPickerOpen = false;
		onogimageselect(asset);
	}

	function handleOgRemove() {
		onogimageselect(null);
	}
</script>

<div class="settings-panel">
	<!-- Visibility -->
	<select
		class="field-input"
		value={visibility}
		onchange={(e) => onvisibility(e.currentTarget.value as Visibility)}
	>
		<option value="public">{t('create.settings.visibility.public')}</option>
		<option value="unlisted">{t('create.settings.visibility.unlisted')}</option>
		<option value="private">{t('create.settings.visibility.private')}</option>
	</select>

	<!-- Canonical URL -->
	<input
		type="url"
		class="field-input"
		placeholder={t('create.label.canonical_url')}
		value={canonicalUrl}
		oninput={(e) => {
			oncanonical(e.currentTarget.value);
			onedit();
		}}
	/>

	<!-- Publish at -->
	<input
		type="datetime-local"
		class="field-input"
		placeholder={t('create.label.publish_at')}
		value={publishAt ?? ''}
		onchange={(e) => {
			onpublishat(e.currentTarget.value || null);
			onedit();
		}}
	/>

	<SeriesSelector
		{series}
		{seriesId}
		onselect={(id) => { onseries(id); onedit(); }}
		onfocus={onseriesfocus}
	/>

	{#each fieldGroups as group (group.id)}
		<div class="field-group">
			<h4 class="field-group-title">{group.name}</h4>
			<CustomFieldGroupCmp
				fields={group.fields}
				values={fieldValues}
				onvalues={onfieldvalues}
				{onedit}
			/>
		</div>
	{/each}

	<!-- SEO section (migration 000165): per-content overrides for search & social -->
	<div class="seo-section">
		<h4 class="seo-title">{t('create.label.seo')}</h4>

		<input
			type="text"
			class="field-input"
			placeholder={t('create.label.meta_title')}
			value={metaTitle ?? ''}
			oninput={(e) => {
				onmetatitle(e.currentTarget.value || null);
				onedit();
			}}
		/>

		<textarea
			class="field-input field-textarea"
			rows="2"
			placeholder={t('create.label.meta_description')}
			oninput={(e) => {
				onmetadescription(e.currentTarget.value || null);
				onedit();
			}}
		>{metaDescription ?? ''}</textarea>

		<!-- OG image: image picker + preview render (replaces the old URL text input) -->
		{#if ogImagePreviewUrl}
			<div class="og-preview">
				<img src={ogImagePreviewUrl} alt="" class="og-preview-img" />
				<div class="og-preview-actions">
					<button type="button" class="og-action-btn" onclick={() => (ogPickerOpen = true)}>
						{t('media.picker.tab_library')}
					</button>
					<button
						type="button"
						class="og-action-btn og-action-remove"
						onclick={handleOgRemove}
						aria-label={t('create.action.remove_tag')}
					>
						<X size={14} />
					</button>
				</div>
			</div>
		{:else}
			<button type="button" class="og-empty" onclick={() => (ogPickerOpen = true)}>
				<ImagePlus size={20} />
				<span>{t('create.label.og_image')}</span>
			</button>
		{/if}

		<label class="seo-checkbox">
			<input
				type="checkbox"
				checked={seoIndexable}
				onchange={(e) => {
					onseoindexable(e.currentTarget.checked);
					onedit();
				}}
			/>
			{t('create.label.seo_indexable')}
		</label>
	</div>
</div>

{#if ogPickerOpen}
	<MediaLibraryModal onSelect={handleOgSelect} onClose={() => (ogPickerOpen = false)} />
{/if}

<style>
	.settings-panel {
		display: flex;
		flex-direction: column;
		gap: 12px;
		padding: 0.5rem;
	}

	.field-input {
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
	.field-input::placeholder {
		color: var(--color-muted);
	}
	.field-input:focus {
		border-color: var(--color-primary);
		box-shadow: 0 0 0 3px color-mix(in srgb, var(--color-primary) 15%, transparent);
	}
	.field-textarea {
		resize: vertical;
		min-height: 60px;
		font-family: inherit;
	}

	.field-group {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}
	.field-group-title {
		margin: 0;
		font-size: 0.875rem;
		font-weight: 600;
		color: var(--color-text);
	}

	/* SEO section */
	.seo-section {
		display: flex;
		flex-direction: column;
		gap: 12px;
		padding-top: 12px;
		border-top: 1px solid var(--color-border);
	}
	.seo-title {
		margin: 0;
		font-size: 0.875rem;
		font-weight: 600;
		color: var(--color-text);
	}

	.seo-checkbox {
		display: flex;
		align-items: center;
		gap: 8px;
		font-size: 0.875rem;
		color: var(--color-text);
		cursor: pointer;
	}
	.seo-checkbox input {
		width: 16px;
		height: 16px;
		accent-color: var(--color-primary);
		cursor: pointer;
	}

	/* OG image picker */
	.og-empty {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 8px;
		width: 100%;
		aspect-ratio: 16 / 9;
		border: 2px dashed var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		color: var(--color-muted);
		font-size: 0.8rem;
		cursor: pointer;
		transition: border-color 0.15s, color 0.15s;
	}
	.og-empty:hover {
		border-color: var(--color-primary);
		color: var(--color-primary);
	}

	.og-preview {
		position: relative;
		overflow: hidden;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface-subtle);
		aspect-ratio: 16 / 9;
	}
	.og-preview-img {
		width: 100%;
		height: 100%;
		object-fit: cover;
	}
	.og-preview-actions {
		position: absolute;
		inset: 0;
		display: flex;
		align-items: flex-end;
		justify-content: flex-end;
		gap: 6px;
		padding: 8px;
		background: linear-gradient(to top, rgb(0 0 0 / 0.55) 0%, transparent 50%);
		opacity: 0;
		transition: opacity 0.2s;
	}
	.og-preview:hover .og-preview-actions {
		opacity: 1;
	}
	.og-action-btn {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		padding: 6px 10px;
		border: 1px solid rgb(255 255 255 / 0.3);
		border-radius: var(--radius);
		background: rgb(0 0 0 / 0.6);
		color: #fff;
		font-size: 0.75rem;
		font-weight: 500;
		cursor: pointer;
		transition: background 0.15s;
	}
	.og-action-btn:hover {
		background: rgb(0 0 0 / 0.8);
	}
	.og-action-remove {
		padding: 6px;
	}
</style>
