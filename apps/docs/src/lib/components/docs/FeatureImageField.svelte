<script lang="ts">
	// FeatureImageField — media-library picker for a doc page's feature image
	// (§44 amendment 5). Reuses the shared MediaPickerModal with the same
	// helpers DocEditor wires for inline images.
	import { MediaPickerModal } from '@japaconnect/ui';
	import { t } from '#lib/i18n/index.js';
	import { listMyMedia, uploadMediaImage, updateMediaImage, stockApi } from '#lib/api/media.js';
	import { ImagePlus, X } from 'lucide-svelte';

	interface Props {
		/** Selected media id ('' = none). */
		value: string;
		/** Served URL for the preview thumbnail. */
		imageUrl: string;
		onchange: (id: string, url: string) => void;
		disabled?: boolean;
	}

	let { value = '', imageUrl = '', onchange, disabled = false }: Props = $props();

	let pickerOpen = $state(false);
</script>

<div class="fif">
	{#if imageUrl}
		<div class="fif-preview">
			<img src={imageUrl} alt={t('edit.feature_image', 'Feature image')} />
			{#if !disabled}
				<button
					type="button"
					class="fif-remove"
					onclick={() => onchange('', '')}
					aria-label={t('edit.remove_image', 'Remove image')}
				>
					<X size={14} />
				</button>
			{/if}
		</div>
	{/if}
	<button
		type="button"
		class="fif-choose"
		onclick={() => (pickerOpen = true)}
		{disabled}
	>
		<ImagePlus size={14} />
		{imageUrl
			? t('edit.replace_image', 'Replace image')
			: t('edit.choose_image', 'Choose image')}
	</button>
</div>

{#if pickerOpen}
	<MediaPickerModal
		{t}
		allowedType="image"
		fetchItems={listMyMedia}
		onUpload={uploadMediaImage}
		onUpdate={updateMediaImage}
		stock={stockApi}
		onSelect={(item) => {
			onchange(item.id ?? '', item.url ?? '');
			pickerOpen = false;
		}}
		onClose={() => (pickerOpen = false)}
	/>
{/if}

<style>
	.fif {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		align-items: flex-start;
	}
	.fif-preview {
		position: relative;
		width: 100%;
		max-width: 320px;
	}
	.fif-preview img {
		width: 100%;
		aspect-ratio: 21 / 9;
		object-fit: cover;
		border-radius: var(--radius-card);
		border: 1px solid var(--color-border);
		display: block;
	}
	.fif-remove {
		position: absolute;
		top: 0.375rem;
		inset-inline-end: 0.375rem;
		display: grid;
		place-items: center;
		width: 1.5rem;
		height: 1.5rem;
		border: none;
		border-radius: 50%;
		background: color-mix(in srgb, var(--color-bg) 80%, transparent);
		color: var(--color-text);
		cursor: pointer;
	}
	.fif-remove:hover {
		background: var(--color-danger, #dc2626);
		color: #fff;
	}
	.fif-choose {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
		padding: 0.4375rem 0.875rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		color: var(--color-text);
		font-size: 0.8125rem;
		cursor: pointer;
	}
	.fif-choose:hover:not(:disabled) {
		border-color: var(--color-primary);
		color: var(--color-primary);
	}
	.fif-choose:disabled {
		opacity: 0.5;
		cursor: default;
	}
</style>
