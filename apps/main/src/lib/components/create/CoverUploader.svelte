<script lang="ts">
	import { t } from '$lib/i18n';
	import { ImagePlus, Loader2, X } from 'lucide-svelte';
	import type { MediaAsset } from '$lib/types';
	import MediaLibraryModal from '$lib/components/media/MediaLibraryModal.svelte';

	interface Props {
		/** Current cover media id (null = no cover). */
		mediaId?: string | null;
		/** Current cover image URL for preview (null = no preview). */
		previewUrl?: string | null;
		onUploaded: (asset: MediaAsset) => void;
		onRemove?: () => void;
	}

	let { mediaId = null, previewUrl = null, onUploaded, onRemove }: Props = $props();

	let busy = $state(false);
	let error = $state<string | null>(null);
	let pickerOpen = $state(false);

	function openPicker() {
		if (busy) return;
		pickerOpen = true;
	}

	function handleSelect(asset: MediaAsset) {
		pickerOpen = false;
		onUploaded(asset);
	}
</script>

<div class="cover-field">
	{#if previewUrl}
		<!-- Preview state: show the current cover with hover actions -->
		<div class="cover-preview">
			<img src={previewUrl} alt="" class="cover-preview-img" />
			<div class="cover-preview-actions">
				<button type="button" class="cover-action-btn" onclick={openPicker} disabled={busy}>
					{t('media.picker.tab_library')}
				</button>
				{#if onRemove}
					<button
						type="button"
						class="cover-action-btn cover-action-remove"
						onclick={onRemove}
						aria-label={t('create.action.remove_tag')}
					>
						<X size={14} />
					</button>
				{/if}
			</div>
		</div>
	{:else}
		<!-- Empty state: single button that opens the Media Library modal
		     (which has both Library and Upload New tabs) -->
		<button type="button" class="cover-empty-btn" onclick={openPicker} disabled={busy}>
			{#if busy}
				<Loader2 size={18} class="animate-spin" />
				{t('create.status.uploading')}
			{:else}
				<ImagePlus size={18} />
				{t('create.placeholder.cover')}
			{/if}
		</button>
	{/if}

	{#if error}
		<span class="cover-error">{error}</span>
	{/if}
</div>

{#if pickerOpen}
	<MediaLibraryModal onSelect={handleSelect} onClose={() => (pickerOpen = false)} />
{/if}

<style>
	.cover-field {
		display: flex;
		flex-direction: column;
		gap: 6px;
	}

	/* Preview state */
	.cover-preview {
		position: relative;
		overflow: hidden;
		border: 1px solid var(--color-border);
		border-radius: var(--radius-card);
		background: var(--color-surface-subtle);
		aspect-ratio: 16 / 9;
	}
	.cover-preview-img {
		width: 100%;
		height: 100%;
		object-fit: cover;
	}
	.cover-preview-actions {
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
	.cover-preview:hover .cover-preview-actions {
		opacity: 1;
	}
	.cover-action-btn {
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
	.cover-action-btn:hover {
		background: rgb(0 0 0 / 0.8);
	}
	.cover-action-remove {
		padding: 6px;
	}

	/* Empty state */
	.cover-empty-btn {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 8px;
		width: 100%;
		padding: 1.5rem 1rem;
		border: 2px dashed var(--color-border);
		border-radius: var(--radius-card);
		background: var(--color-surface);
		color: var(--color-muted);
		font-size: 0.875rem;
		cursor: pointer;
		transition: border-color 0.15s, color 0.15s;
	}
	.cover-empty-btn:hover:not(:disabled) {
		border-color: var(--color-primary);
		color: var(--color-primary);
	}
	.cover-empty-btn:disabled {
		opacity: 0.6;
		cursor: progress;
	}
	.cover-error {
		font-size: 0.75rem;
		color: var(--destructive, #dc2626);
	}
</style>
