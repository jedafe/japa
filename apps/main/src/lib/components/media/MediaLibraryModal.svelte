<script lang="ts">
	import { t } from '$lib/i18n';
	import { Dialog } from 'bits-ui';
	import { X, Search, Loader2, ImagePlus, Upload } from 'lucide-svelte';
	import {
		listMyMedia,
		initiateUpload,
		uploadToPresigned,
		confirmMedia
	} from '$lib/api/create';
	import type { MediaAsset } from '$lib/types';

	interface Props {
		/** Called with the selected media when the user confirms. */
		onSelect: (media: MediaAsset) => void;
		/** Called when the modal is dismissed (cancel / overlay / X). */
		onClose: () => void;
		/** MIME type filter passed to /me/media?type=… (default: image). */
		allowedType?: string;
	}

	let { onSelect, onClose, allowedType = 'image' }: Props = $props();

	type Tab = 'library' | 'upload';
	let tab = $state<Tab>('library');
	let items = $state<MediaAsset[]>([]);
	let loading = $state(false);
	let error = $state<string | null>(null);
	let search = $state('');
	let searchTimer: ReturnType<typeof setTimeout> | null = null;
	let selectedId = $state<string | null>(null);
	let uploading = $state(false);
	let uploadError = $state<string | null>(null);

	// Load the user's media library on mount.
	$effect(() => {
		loadLibrary();
	});

	function loadLibrary(q = search) {
		loading = true;
		error = null;
		listMyMedia({ type: allowedType, q: q || undefined })
			.then((res) => {
				if (res.error || !res.data) {
					error = res.error?.message ?? 'Failed to load media';
					items = [];
				} else {
					items = res.data;
				}
			})
			.catch(() => {
				error = 'Failed to load media';
				items = [];
			})
			.finally(() => {
				loading = false;
			});
	}

	function onSearchInput(v: string) {
		search = v;
		if (searchTimer) clearTimeout(searchTimer);
		searchTimer = setTimeout(() => loadLibrary(v), 350);
	}

	function selectItem(item: MediaAsset) {
		selectedId = item.id;
	}

	function confirmSelection() {
		const picked = items.find((i) => i.id === selectedId);
		if (picked) onSelect(picked);
	}

	function handleEscape(e: KeyboardEvent) {
		if (e.key === 'Escape') onClose();
	}

	async function handleUpload(e: Event) {
		const input = e.currentTarget as HTMLInputElement;
		const file = input.files?.[0];
		if (!file) return;
		uploading = true;
		uploadError = null;
		try {
			const init = await initiateUpload({
				filename: file.name,
				mime_type: file.type,
				file_size: file.size
			});
			if (init.error || !init.data) throw new Error(init.error?.message ?? 'initiate failed');
			const up = await uploadToPresigned(init.data.presigned_url, file);
			if (!up.ok) throw new Error('upload failed');
			const conf = await confirmMedia(init.data.media_id);
			if (conf.error || !conf.data) throw new Error(conf.error?.message ?? 'confirm failed');
			// Switch to library tab and select the freshly uploaded asset.
			tab = 'library';
			loadLibrary();
			selectedId = conf.data.id;
			onSelect(conf.data);
		} catch (err) {
			uploadError = err instanceof Error ? err.message : 'Upload failed';
		} finally {
			uploading = false;
			input.value = '';
		}
	}
</script>

<Dialog.Root open={true} onOpenChange={(v) => { if (!v) onClose(); }}>
	<Dialog.Portal>
		<Dialog.Overlay class="fixed inset-0 z-[100] bg-black/50 backdrop-blur-sm" />
		<Dialog.Content class="fixed left-1/2 top-1/2 z-[101] flex max-h-[85vh] w-full max-w-[720px] -translate-x-1/2 -translate-y-1/2 flex-col overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] shadow-2xl">
			<header class="modal-header">
				<h2 class="modal-title">{t('media.picker.title')}</h2>
				<Dialog.Close class="modal-close">
					<X size={18} />
				</Dialog.Close>
			</header>

		<!-- Tabs -->
		<div class="modal-tabs">
			<button
				type="button"
				class="modal-tab"
				class:active={tab === 'library'}
				onclick={() => (tab = 'library')}
			>
				{t('media.picker.tab_library')}
			</button>
			<button
				type="button"
				class="modal-tab"
				class:active={tab === 'upload'}
				onclick={() => (tab = 'upload')}
			>
				<Upload size={14} />
				{t('media.picker.tab_upload')}
			</button>
		</div>

		{#if tab === 'library'}
			<!-- Search bar -->
			<div class="modal-search">
				<Search size={16} class="modal-search-icon" />
				<input
					type="text"
					class="modal-search-input"
					placeholder={t('media.picker.search_placeholder')}
					value={search}
					oninput={(e) => onSearchInput(e.currentTarget.value)}
				/>
			</div>

			<!-- Grid -->
			<div class="modal-grid-wrap">
				{#if loading}
					<div class="modal-state">
						<Loader2 size={24} class="animate-spin" />
						<span>{t('media.picker.loading')}</span>
					</div>
				{:else if error}
					<div class="modal-state modal-state-error">{error}</div>
				{:else if items.length === 0}
					<div class="modal-state">{t('media.picker.empty')}</div>
				{:else}
					<div class="modal-grid">
						{#each items as item (item.id)}
							<button
								type="button"
								class="media-thumb"
								class:selected={selectedId === item.id}
								onclick={() => selectItem(item)}
							>
								<img src={item.url} alt={item.alt_text ?? item.filename ?? ''} loading="lazy" />
								{#if item.filename}
									<span class="media-thumb-name">{item.filename}</span>
								{/if}
							</button>
						{/each}
					</div>
				{/if}
			</div>

			<!-- Footer -->
			<footer class="modal-footer">
				<button type="button" class="btn-secondary" onclick={onClose}>
					{t('media.picker.cancel')}
				</button>
				<button
					type="button"
					class="btn-primary"
					disabled={!selectedId}
					onclick={confirmSelection}
				>
					{t('media.picker.select')}
				</button>
			</footer>
		{:else}
			<!-- Upload tab -->
			<div class="modal-upload-area">
				<label class="upload-dropzone" class:busy={uploading}>
					{#if uploading}
						<Loader2 size={28} class="animate-spin" />
						<span>{t('media.picker.uploading')}</span>
					{:else}
						<ImagePlus size={28} />
						<span>{t('media.picker.upload_placeholder')}</span>
					{/if}
					<input
						type="file"
						accept="image/*"
						class="hidden-input"
						onchange={handleUpload}
						disabled={uploading}
					/>
				</label>
				{#if uploadError}
					<span class="upload-error">{uploadError}</span>
				{/if}
			</div>
		{/if}
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>

<style>
	.modal-overlay {
		position: fixed;
		inset: 0;
		z-index: 100;
		display: flex;
		align-items: center;
		justify-content: center;
		padding: 1rem;
		background: rgb(0 0 0 / 0.5);
		backdrop-filter: blur(2px);
	}

	.modal-panel {
		display: flex;
		flex-direction: column;
		width: 100%;
		max-width: 720px;
		max-height: 85vh;
		border: 1px solid var(--color-border);
		border-radius: var(--radius-card);
		background: var(--color-surface);
		box-shadow: 0 20px 60px rgb(0 0 0 / 0.25);
		overflow: hidden;
	}

	.modal-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 1rem 1.25rem;
		border-bottom: 1px solid var(--color-border);
	}
	.modal-title {
		margin: 0;
		font-size: 1.125rem;
		font-weight: 600;
		color: var(--color-text);
	}
	.modal-close {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 32px;
		height: 32px;
		border: none;
		border-radius: var(--radius);
		background: transparent;
		color: var(--color-muted);
		cursor: pointer;
		transition: background 0.15s, color 0.15s;
	}
	.modal-close:hover {
		background: var(--color-surface-subtle);
		color: var(--color-text);
	}

	.modal-tabs {
		display: flex;
		gap: 2px;
		padding: 0 1.25rem;
		border-bottom: 1px solid var(--color-border);
	}
	.modal-tab {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		padding: 10px 14px;
		border: none;
		border-bottom: 2px solid transparent;
		background: transparent;
		color: var(--color-muted);
		font-size: 0.875rem;
		font-weight: 500;
		cursor: pointer;
		transition: color 0.15s, border-color 0.15s;
	}
	.modal-tab:hover {
		color: var(--color-text);
	}
	.modal-tab.active {
		color: var(--color-primary);
		border-bottom-color: var(--color-primary);
	}

	.modal-search {
		position: relative;
		padding: 0.75rem 1.25rem;
	}
	.modal-search :global(.modal-search-icon) {
		position: absolute;
		left: 1.75rem;
		top: 50%;
		transform: translateY(-50%);
		color: var(--color-muted);
		pointer-events: none;
	}
	.modal-search-input {
		width: 100%;
		padding: 8px 12px 8px 38px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		color: var(--color-text);
		font-size: 0.875rem;
		outline: none;
		transition: border-color 0.15s;
	}
	.modal-search-input:focus {
		border-color: var(--color-primary);
	}

	.modal-grid-wrap {
		flex: 1;
		overflow-y: auto;
		padding: 0 1.25rem 0.75rem;
		min-height: 200px;
	}

	.modal-state {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 0.75rem;
		padding: 3rem 1rem;
		color: var(--color-muted);
		font-size: 0.875rem;
		text-align: center;
	}
	.modal-state-error {
		color: var(--destructive, #dc2626);
	}

	.modal-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(120px, 1fr));
		gap: 10px;
		padding-top: 0.5rem;
	}

	.media-thumb {
		display: flex;
		flex-direction: column;
		gap: 4px;
		padding: 4px;
		border: 2px solid transparent;
		border-radius: var(--radius);
		background: var(--color-surface-subtle);
		cursor: pointer;
		transition: border-color 0.15s, transform 0.1s;
		overflow: hidden;
	}
	.media-thumb:hover {
		border-color: var(--color-border);
	}
	.media-thumb.selected {
		border-color: var(--color-primary);
		box-shadow: 0 0 0 2px color-mix(in srgb, var(--color-primary) 20%, transparent);
	}
	.media-thumb img {
		width: 100%;
		aspect-ratio: 1;
		object-fit: cover;
		border-radius: calc(var(--radius) - 2px);
		background: var(--color-surface-subtle);
	}
	.media-thumb-name {
		font-size: 0.7rem;
		color: var(--color-text-subtle);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
		text-align: center;
	}

	.modal-footer {
		display: flex;
		justify-content: flex-end;
		gap: 8px;
		padding: 0.75rem 1.25rem;
		border-top: 1px solid var(--color-border);
	}
	.btn-secondary,
	.btn-primary {
		padding: 8px 16px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		font-size: 0.875rem;
		font-weight: 500;
		cursor: pointer;
		transition: background 0.15s, border-color 0.15s, opacity 0.15s;
	}
	.btn-secondary {
		background: transparent;
		color: var(--color-text);
	}
	.btn-secondary:hover {
		background: var(--color-surface-subtle);
	}
	.btn-primary {
		background: var(--color-primary);
		border-color: var(--color-primary);
		color: var(--color-primary-contrast);
	}
	.btn-primary:hover:not(:disabled) {
		background: var(--color-primary-hover);
		border-color: var(--color-primary-hover);
	}
	.btn-primary:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	/* Upload tab */
	.modal-upload-area {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 1rem;
		padding: 2rem 1.25rem;
	}
	.upload-dropzone {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 0.75rem;
		width: 100%;
		max-width: 360px;
		padding: 2.5rem 1.5rem;
		border: 2px dashed var(--color-border);
		border-radius: var(--radius-card);
		background: var(--color-surface-subtle);
		color: var(--color-muted);
		font-size: 0.875rem;
		cursor: pointer;
		transition: border-color 0.15s, color 0.15s;
	}
	.upload-dropzone:hover {
		border-color: var(--color-primary);
		color: var(--color-primary);
	}
	.upload-dropzone.busy {
		opacity: 0.7;
		cursor: progress;
	}
	.hidden-input {
		display: none;
	}
	.upload-error {
		font-size: 0.8rem;
		color: var(--destructive, #dc2626);
	}

	@media (max-width: 640px) {
		.modal-panel {
			max-height: 90vh;
		}
		.modal-grid {
			grid-template-columns: repeat(auto-fill, minmax(90px, 1fr));
		}
	}
</style>
