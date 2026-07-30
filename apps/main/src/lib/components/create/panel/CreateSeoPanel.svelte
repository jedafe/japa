<script lang="ts">
	import { t } from '$lib/i18n';
	import { ImagePlus, X } from 'lucide-svelte';
	import type { MediaAsset } from '$lib/types';
	import MediaLibraryModal from '$lib/components/media/MediaLibraryModal.svelte';

	interface Props {
		metaTitle: string | null;
		metaDescription: string | null;
		ogImageUrl: string | null;
		ogImagePreviewUrl: string | null;
		seoIndexable: boolean;
		onmetatitle: (v: string | null) => void;
		onmetadescription: (v: string | null) => void;
		onogimageselect: (asset: MediaAsset | null) => void;
		onseoindexable: (v: boolean) => void;
		onedit: () => void;
	}

	let {
		metaTitle,
		metaDescription,
		ogImageUrl,
		ogImagePreviewUrl,
		seoIndexable,
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

<div class="seo-panel">
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

{#if ogPickerOpen}
	<MediaLibraryModal onSelect={handleOgSelect} onClose={() => (ogPickerOpen = false)} />
{/if}

<style>
	.seo-panel {
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
		color: var(--color-surface);
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
