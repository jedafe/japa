<script lang="ts">
	// MediaPickerModal — a reusable media library picker with three tabs:
	//   1. Browse Library — search and select from uploaded media
	//   2. Upload New — upload a file from disk
	//   3. Stock Search — search stock photo providers and import
	//
	// Ref: japaconnect-engineering/sections/25-ad-placement.md —
	// ad campaign content supports an image_media_id + image_url pair.
	// Ref: AGENTS.md Rule 16 — media comes from the API, never hardcoded.
	import { apiFetch, apiRaw, mediaAdminApi, stockAdminApi, type StockPhoto } from '$lib/api';
	import { PUBLIC_JC_API_URL } from '$env/static/public';
	import { buildApiUrl } from '@japaconnect/api-url';
	import { convertImageToModernFormat } from '$lib/utils/imageConvert';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { Upload, Search, Image as ImageIcon, X, Loader2, Download, Check, ChevronLeft, ChevronRight, Pencil, Trash2 } from 'lucide-svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';

	type MediaItem = {
		id: string;
		filename: string;
		mime_type: string;
		file_size: number;
		alt_text?: string | null;
		caption?: string | null;
		status: string;
		created_at: string;
		url?: string;
	};

	let {
		open = $bindable(false),
		onselect,
		accept = 'image/*,.heic,.heif'
	}: {
		open?: boolean;
		onselect: (media: { id: string; url: string; filename: string }) => void;
		accept?: string;
	} = $props();

	// --- Browse tab state ---
	let items = $state<MediaItem[]>([]);
	let loading = $state(false);
	let searchQuery = $state('');
	let debounceTimer: ReturnType<typeof setTimeout> | undefined;
	let selectedId = $state<string | null>(null);
	let activeTab = $state<'browse' | 'upload' | 'stock'>('browse');

	// --- Browse pagination ---
	const PAGE_SIZE = 24;
	let currentPage = $state(0); // 0-based
	let totalPages = $state(1);
	let totalItems = $state(0);

	// --- Upload tab state ---
	let uploadFile = $state<File | null>(null);
	let uploading = $state(false);
	let uploadProgress = $state('');
	let dragActive = $state(false);
	let fileInput = $state<HTMLInputElement | null>(null);

	// --- Stock tab state ---
	let stockProviders = $state<{ name: string; display_name: string }[]>([]);
	let stockProvidersLoaded = $state(false);
	let stockQuery = $state('');
	let stockResults = $state<StockPhoto[]>([]);
	let stockLoading = $state(false);
	let stockProviderFilter = $state('');
	let stockImporting = $state<Set<string>>(new Set());
	let stockImported = $state<Set<string>>(new Set());
	let stockTimer: ReturnType<typeof setTimeout> | undefined;

	let errorMessage = $state('');

	// --- Selected item metadata editor (alt text + title) ---
	let editAlt = $state('');
	let editCaption = $state('');
	let editSaving = $state(false);
	let editError = $state('');
	let editSaved = $state(false);
	let selectedItem = $derived(items.find((i) => i.id === selectedId) ?? null);
	// The metadata editor only renders for callers who can actually save —
	// PATCH /admin/media/{id} is gated by media.edit_any.
	let canEditMedia = $derived(hasPermission('media.edit_any'));
	// The edit/delete panel only opens via the thumbnail action buttons —
	// clicking a thumbnail just selects it for the Select button.
	let panelMode = $state<'edit' | 'delete' | null>(null);
	// DELETE /admin/media/{id} (media.delete_any) — soft delete.
	let canDeleteMedia = $derived(hasPermission('media.delete_any'));
	let deleteConfirming = $state(false);
	let deleting = $state(false);
	let deleteError = $state('');
	// Snapshot of the last picked item so the Select button still works when
	// the user paginates away from the page containing the selected thumb
	// (items only holds the current page).
	let pickedItem = $state<MediaItem | null>(null);

	// Clear any pending debounce timers when the modal unmounts.
	$effect(() => {
		return () => {
			if (debounceTimer) clearTimeout(debounceTimer);
			if (stockTimer) clearTimeout(stockTimer);
		};
	});

	// fixMediaUrl — the backend returns imgproxy URLs (e.g.
	// http://imgproxy:8080/{sig}/{opts}/{base64_source}.{format}) whose host
	// may not be publicly resolvable. Decode the base64 source segment to
	// recover the s3:// path and construct a media/serve URL anchored on the
	// API origin (PUBLIC_JC_API_URL). Anchoring matters: only prod's nginx
	// proxies /api on each app host — staging's admin subdomain would 404 a
	// relative /api/v1/... path against the SPA.
	function fixMediaUrl(url: string): string {
		if (!url) return '';
		// Relative serve URL — anchor it to the API origin.
		if (url.startsWith('/api/v1/media/serve/')) return buildApiUrl(url, PUBLIC_JC_API_URL ?? '');
		try {
			// Format:        {base}/{sig}/{opts}/{base64_source}.{format}
			// legacy:        {base}/{sig}/{opts}/plain/{base64_source}@{format}
			const parts = url.split('/');
			// Find the base64-encoded source segment — after "plain"
			// (legacy), or the last segment carrying an @ext/.ext suffix.
			let encoded: string | undefined;
			const plainIdx = parts.indexOf('plain');
			if (plainIdx >= 0 && parts[plainIdx + 1]) {
				encoded = parts[plainIdx + 1].split('@')[0];
			} else {
				const last = parts[parts.length - 1] ?? '';
				if (last.includes('@')) {
					encoded = last.split('@')[0];
				} else if (/^[A-Za-z0-9_-]+\.[a-z0-9]+$/i.test(last)) {
					encoded = last.slice(0, last.lastIndexOf('.'));
				}
			}
			if (!encoded) return url;
			// base64url decode
			const decoded = atob(encoded.replace(/-/g, '+').replace(/_/g, '/'));
			// decoded = "s3://{bucket}/{path}"
			if (decoded.startsWith('s3://')) {
				const path = decoded.slice(5); // strip "s3://"
				return buildApiUrl('/api/v1/media/serve/' + path, PUBLIC_JC_API_URL ?? '');
			}
			return url;
		} catch {
			return url;
		}
	}

	// Load media items when the modal opens.
	$effect(() => {
		if (open) {
			loadMedia();
			loadStockProviders();
		}
	});

	async function loadMedia(query?: string, page = currentPage) {
		loading = true;
		errorMessage = '';
		try {
			const params = new URLSearchParams();
			params.set('page_size', String(PAGE_SIZE));
			params.set('page_offset', String(page * PAGE_SIZE));
			params.set('type', 'image');
			if (query) params.set('q', query);
			// Use apiRaw to access top-level total/total_pages alongside data.
			const result = await apiRaw<{ data?: MediaItem[]; items?: MediaItem[]; total?: number; total_pages?: number }>(
				`/admin/media?${params.toString()}`
			);
			if (result.ok) {
				const d = result.data;
				items = d.data ?? d.items ?? [];
				totalItems = d.total ?? items.length;
				totalPages = d.total_pages ?? 1;
				currentPage = page;
			} else {
				errorMessage = result.error || t('common.load_error', 'Failed to load');
			}
		} catch {
			errorMessage = t('common.load_error', 'Failed to load');
		} finally {
			loading = false;
		}
	}

	function goToPage(page: number) {
		if (page < 0 || page >= totalPages || page === currentPage) return;
		loadMedia(searchQuery, page);
	}

	function handleSearchInput(e: Event) {
		const target = e.target as HTMLInputElement;
		searchQuery = target.value;
		if (debounceTimer) clearTimeout(debounceTimer);
		debounceTimer = setTimeout(() => loadMedia(searchQuery, 0), 300);
	}

	function handleFileSelect(e: Event) {
		const target = e.target as HTMLInputElement;
		uploadFile = target.files?.[0] ?? null;
	}

	function handleDrop(e: DragEvent) {
		e.preventDefault();
		dragActive = false;
		const file = e.dataTransfer?.files?.[0];
		if (file) uploadFile = file;
	}

	async function handleUpload() {
		if (!uploadFile) return;
		uploading = true;
		uploadProgress = t('common.uploading', 'Uploading...');
		errorMessage = '';
		try {
			const convertedFile = await convertImageToModernFormat(uploadFile);
			const formData = new FormData();
			formData.append('file', convertedFile);
			const result = await apiFetch<{ media_id: string; url: string }>(
				'/admin/media/upload',
				{ method: 'POST', body: formData }
			);
			if (result.error) {
				errorMessage = result.error.message || t('common.upload_error', 'Upload failed');
			} else if (result.data) {
				// Land on the Browse tab with the new asset highlighted —
				// the user confirms with Select instead of the upload
				// auto-inserting it. Clearing searchQuery keeps a stale
				// filter from hiding the new item (and from silently
				// re-applying on the next pagination click); the pending
				// debounce is cancelled so it can't fire an extra fetch.
				if (debounceTimer) clearTimeout(debounceTimer);
				searchQuery = '';
				activeTab = 'browse';
				loadMedia(undefined, 0);
				selectedId = result.data.media_id;
				pickedItem = {
					id: result.data.media_id,
					url: result.data.url,
					filename: convertedFile.name,
					mime_type: convertedFile.type,
					file_size: convertedFile.size,
					status: 'approved',
					created_at: new Date().toISOString()
				};
				panelMode = null;
				uploadFile = null;
			}
		} catch {
			errorMessage = t('common.upload_error', 'Upload failed');
		} finally {
			uploading = false;
			uploadProgress = '';
		}
	}

	// Fallback label derived from the filename — strips the extension and
	// normalises separators so "japa-guide_hero.webp" → "japa guide hero".
	// Pre-fills the alt/title fields when the image has none stored.
	function filenameLabel(filename?: string): string {
		const base = (filename ?? '').replace(/\.[^.]+$/, '');
		return base.replace(/[-_]+/g, ' ').trim();
	}

	// Pre-fill the edit fields from the item, falling back to a label derived
	// from the filename when the image has no stored alt text/title.
	function primeEditFields(item: MediaItem) {
		const fallback = filenameLabel(item.filename);
		editAlt = item.alt_text?.trim() || fallback;
		editCaption = item.caption?.trim() || fallback;
		editError = '';
		editSaved = false;
		deleteConfirming = false;
		deleteError = '';
	}

	function handleSelect(item: MediaItem) {
		if (selectedId === item.id) {
			clearSelection();
			return;
		}
		selectedId = item.id;
		pickedItem = item;
		primeEditFields(item);
		panelMode = null;
	}

	function clearSelection() {
		selectedId = null;
		pickedItem = null;
		panelMode = null;
		deleteConfirming = false;
		deleteError = '';
	}

	// Thumbnail pencil button — select the item and reveal the alt/title
	// editor (the only way the panel opens). Re-clicking the pencil on the
	// item already being edited is a no-op so unsaved edits aren't lost.
	function openEditPanel(item: MediaItem) {
		if (selectedId === item.id && panelMode === 'edit') return;
		selectedId = item.id;
		pickedItem = item;
		primeEditFields(item);
		panelMode = 'edit';
	}

	// Thumbnail trash button — select the item and go straight to the
	// delete-confirm step.
	function openDeleteConfirm(item: MediaItem) {
		selectedId = item.id;
		pickedItem = item;
		primeEditFields(item);
		deleteConfirming = true;
		panelMode = 'delete';
	}

	// PATCH /admin/media/{id} (media.edit_any) — '' clears a field; the
	// backend COALESCEs each column so a JSON null would mean "keep".
	async function saveMetadata() {
		if (!selectedId) return;
		// Capture the id before the await so a mid-flight selection change
		// can't apply this item's updates to a different list entry.
		const id = selectedId;
		editSaving = true;
		editError = '';
		editSaved = false;
		const updates = { alt_text: editAlt.trim(), caption: editCaption.trim() };
		try {
			const result = await mediaAdminApi.update(id, updates);
			if (result.ok) {
				items = items.map((i) => (i.id === id ? { ...i, ...updates } : i));
				if (pickedItem?.id === id) pickedItem = { ...pickedItem, ...updates };
				if (selectedId === id) editSaved = true;
			} else {
				editError = result.error || t('media.picker.edit_failed', 'Could not save changes');
			}
		} catch {
			editError = t('media.picker.edit_failed', 'Could not save changes');
		} finally {
			editSaving = false;
		}
	}

	// DELETE /admin/media/{id} (media.delete_any) — soft delete, then drop
	// the item from the grid and clear the selection.
	async function deleteSelected() {
		if (!canDeleteMedia || !selectedId) return;
		deleting = true;
		deleteError = '';
		const id = selectedId;
		try {
			const result = await mediaAdminApi.delete(id);
			if (result.ok) {
				items = items.filter((i) => i.id !== id);
				// Only clear the selection/panel/confirm if it still points
				// at the deleted item — the user may have selected another
				// thumbnail while the request was in flight.
				if (selectedId === id) {
					selectedId = null;
					pickedItem = null;
					panelMode = null;
					deleteConfirming = false;
				}
			} else {
				deleteError = result.error || t('media.picker.delete_failed', 'Could not delete media');
			}
		} catch {
			deleteError = t('media.picker.delete_failed', 'Could not delete media');
		} finally {
			deleting = false;
		}
	}

	function handleConfirm() {
		const selected = items.find((i) => i.id === selectedId) ?? (pickedItem?.id === selectedId ? pickedItem : null);
		if (selected) {
			onselect({
				id: selected.id,
				url: fixMediaUrl(selected.url ?? ''),
				filename: selected.filename
			});
			open = false;
		}
	}

	// --- Stock provider search ---

	async function loadStockProviders() {
		if (stockProvidersLoaded) return;
		try {
			const result = await stockAdminApi.listAvailableProviders();
			if (result.ok) {
				stockProviders = result.data.providers ?? [];
			}
		} catch {
			// Silent — stock tab just won't show provider filter
		}
		stockProvidersLoaded = true;
	}

	function handleStockSearchInput(e: Event) {
		const target = e.target as HTMLInputElement;
		stockQuery = target.value;
		if (stockTimer) clearTimeout(stockTimer);
		if (!stockQuery.trim()) {
			stockResults = [];
			return;
		}
		stockTimer = setTimeout(() => runStockSearch(), 400);
	}

	async function runStockSearch() {
		if (!stockQuery.trim()) return;
		stockLoading = true;
		errorMessage = '';
		try {
			const result = await stockAdminApi.search({
				q: stockQuery.trim(),
				provider: stockProviderFilter || undefined,
				per_page: 24
			});
			if (result.ok) {
				stockResults = result.data.photos ?? [];
			} else {
				errorMessage = result.error || t('ads.media_stock_error', 'Stock search failed');
			}
		} catch {
			errorMessage = t('ads.media_stock_error', 'Stock search failed');
		} finally {
			stockLoading = false;
		}
	}

	async function importStockPhoto(photo: StockPhoto) {
		const pkey = `${photo.provider}:${photo.id}`;
		stockImporting = new Set([...stockImporting, pkey]);
		errorMessage = '';
		try {
			const result = await stockAdminApi.import(photo.provider, photo.id);
			if (result.ok) {
				stockImported = new Set([...stockImported, pkey]);
				// Auto-select the imported photo
				onselect({
					id: result.data.media_id,
					url: fixMediaUrl(photo.full_url),
					filename: photo.description?.slice(0, 40) || `${photo.provider}-${photo.id}`
				});
				open = false;
			} else {
				errorMessage = result.error || t('ads.media_stock_import_error', 'Import failed');
			}
		} catch {
			errorMessage = t('ads.media_stock_import_error', 'Import failed');
		} finally {
			stockImporting = new Set([...stockImporting].filter((k) => k !== pkey));
		}
	}

	function handleClose() {
		selectedId = null;
		pickedItem = null;
		searchQuery = '';
		uploadFile = null;
		errorMessage = '';
		stockQuery = '';
		stockResults = [];
		currentPage = 0;
		totalPages = 1;
		totalItems = 0;
		editAlt = '';
		editCaption = '';
		editError = '';
		editSaved = false;
		panelMode = null;
		deleteConfirming = false;
		deleting = false;
		deleteError = '';
	}

	// Cleanup timers on destroy.
	$effect(() => {
		return () => {
			if (debounceTimer) clearTimeout(debounceTimer);
			if (stockTimer) clearTimeout(stockTimer);
		};
	});

	let filteredItems = $derived(
		items.filter((i) => i.mime_type.startsWith('image/'))
	);
</script>

<Modal bind:open elevated title={t('ads.media_picker_title', 'Select Image')} onclose={handleClose}>
	<div class="space-y-4">
		{#if errorMessage}
			<div class="rounded-lg border border-danger/30 bg-danger/10 px-4 py-3 text-sm text-danger">
				{errorMessage}
			</div>
		{/if}

		<!-- Tab switcher -->
		<div class="flex gap-2 overflow-x-auto border-b" style="border-color: var(--color-border)">
			<button
				type="button"
				class="px-3 py-2 text-sm font-medium transition-colors"
				style={activeTab === 'browse'
					? 'border-bottom: 2px solid var(--color-primary); color: var(--color-primary)'
					: 'color: var(--color-muted)'}
				onclick={() => (activeTab = 'browse')}
			>
				<ImageIcon class="me-1 inline h-4 w-4" />
				{t('ads.media_browse', 'Browse Library')}
			</button>
			<button
				type="button"
				class="px-3 py-2 text-sm font-medium transition-colors"
				style={activeTab === 'upload'
					? 'border-bottom: 2px solid var(--color-primary); color: var(--color-primary)'
					: 'color: var(--color-muted)'}
				onclick={() => (activeTab = 'upload')}
			>
				<Upload class="me-1 inline h-4 w-4" />
				{t('ads.media_upload', 'Upload New')}
			</button>
			<button
				type="button"
				class="px-3 py-2 text-sm font-medium transition-colors"
				style={activeTab === 'stock'
					? 'border-bottom: 2px solid var(--color-primary); color: var(--color-primary)'
					: 'color: var(--color-muted)'}
				onclick={() => (activeTab = 'stock')}
			>
				<Search class="me-1 inline h-4 w-4" />
				{t('ads.media_stock', 'Stock Photos')}
			</button>
		</div>

		{#if activeTab === 'browse'}
			<!-- Search bar — always visible so typing doesn't lose focus -->
			<div class="relative">
				<Search class="absolute start-3 top-1/2 h-4 w-4 -translate-y-1/2" style="color: var(--color-muted)" />
				<input
					type="text"
					value={searchQuery}
					oninput={handleSearchInput}
					placeholder={t('ads.media_search_placeholder', 'Search media...')}
					class="w-full rounded-md border py-2 ps-9 pe-3 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
				/>
			</div>

			<!-- Media grid — only this area shows the loading spinner -->
			{#if loading}
				<div class="flex items-center justify-center py-12">
					<Loader2 class="h-6 w-6 animate-spin" style="color: var(--color-muted)" />
				</div>
			{:else if filteredItems.length === 0}
				<p class="py-12 text-center text-sm" style="color: var(--color-muted)">
					{t('ads.media_empty', 'No images found')}
				</p>
			{:else}
				<div class="grid max-h-80 grid-cols-2 gap-3 overflow-y-auto sm:grid-cols-4">
					{#each filteredItems as item (item.id)}
						<!-- group wrapper keeps the edit/delete icon buttons as siblings
						     of the select button (no nested interactive elements). -->
						<div
							class="group relative overflow-hidden rounded-lg border-2 transition-all"
							style={selectedId === item.id
								? 'border-color: var(--color-primary); box-shadow: 0 0 0 2px var(--color-primary)'
								: 'border-color: var(--color-border)'}
						>
							<button
								type="button"
								class="block w-full"
								aria-pressed={selectedId === item.id}
								onclick={() => handleSelect(item)}
							>
								{#if item.url}
									<img src={fixMediaUrl(item.url)} alt={item.alt_text || item.filename} class="h-20 w-full object-cover" loading="lazy" />
								{:else}
									<div class="flex h-20 items-center justify-center" style="background-color: var(--color-surface)">
										<ImageIcon class="h-8 w-8" style="color: var(--color-muted)" />
									</div>
								{/if}
								<!-- Saved title (caption) overrides the filename as the
								     thumbnail label once set. -->
								<p class="truncate px-1 py-1 text-xs" style="color: var(--color-muted)" title={item.caption || item.filename}>
									{item.caption || item.filename}
								</p>
							</button>
							{#if canEditMedia || canDeleteMedia}
								<div
									class="absolute end-1.5 top-1.5 flex gap-1 transition-opacity {selectedId === item.id
										? 'opacity-100'
										: 'pointer-events-none opacity-0 group-focus-within:pointer-events-auto group-focus-within:opacity-100 group-hover:pointer-events-auto group-hover:opacity-100 [@media(hover:none)]:pointer-events-auto [@media(hover:none)]:opacity-100'}"
								>
									{#if canEditMedia}
										<button
											type="button"
											class="grid h-6 w-6 place-items-center rounded-full text-white transition-colors hover:bg-black/80"
											style="background: rgb(0 0 0 / 0.55)"
											title={t('media.picker.edit', 'Edit')}
											aria-label={t('media.picker.edit', 'Edit')}
											onclick={() => openEditPanel(item)}
										>
											<Pencil class="h-3 w-3" />
										</button>
									{/if}
									{#if canDeleteMedia}
										<button
											type="button"
											class="grid h-6 w-6 place-items-center rounded-full text-white transition-colors hover:bg-red-600"
											style="background: rgb(0 0 0 / 0.55)"
											title={t('media.picker.delete', 'Delete')}
											aria-label={t('media.picker.delete', 'Delete')}
											onclick={() => openDeleteConfirm(item)}
										>
											<Trash2 class="h-3 w-3" />
										</button>
									{/if}
								</div>
							{/if}
						</div>
					{/each}
				</div>

				<!-- Pagination -->
				{#if totalPages > 1}
					<div class="flex items-center justify-between border-t pt-3" style="border-color: var(--color-border)">
						<span class="text-xs" style="color: var(--color-muted)">
							{t('common.page', 'Page')} {currentPage + 1} / {totalPages}
							{#if totalItems > 0}
								· {totalItems} {t('common.items', 'items')}
							{/if}
						</span>
						<div class="flex items-center gap-1">
							<Button
								variant="outline"
								size="sm"
								disabled={currentPage === 0 || loading}
								onclick={() => goToPage(currentPage - 1)}
							>
								<ChevronLeft class="h-4 w-4 rtl:-scale-x-100" />
							</Button>
							<Button
								variant="outline"
								size="sm"
								disabled={currentPage >= totalPages - 1 || loading}
								onclick={() => goToPage(currentPage + 1)}
							>
								<ChevronRight class="h-4 w-4 rtl:-scale-x-100" />
							</Button>
						</div>
					</div>
				{/if}
			{/if}

			<!-- Item action panel — opened by the thumbnail edit/delete buttons
			     (never by clicking the image itself). 'edit' shows the alt/title
			     editor (PATCH /admin/media/{id}) plus a Delete button; 'delete'
			     goes straight to the inline confirm (DELETE /admin/media/{id}). -->
			{#if selectedItem && panelMode && (canEditMedia || canDeleteMedia)}
				<div class="rounded-lg border p-3" style="border-color: var(--color-border); background-color: var(--color-surface)">
					{#if panelMode === 'edit' && canEditMedia}
						<div class="flex flex-col gap-3 sm:flex-row sm:items-end">
							<label class="flex-1 text-xs font-medium" style="color: var(--color-muted)">
								{t('media.picker.alt_text', 'Alt text')}
								<input
									type="text"
									bind:value={editAlt}
									oninput={() => (editSaved = false)}
									placeholder={t('media.picker.alt_placeholder', 'Describe the image for screen readers')}
									class="mt-1 w-full rounded-md border px-2 py-1.5 text-sm font-normal"
									style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
								/>
							</label>
							<label class="flex-1 text-xs font-medium" style="color: var(--color-muted)">
								{t('media.picker.caption', 'Title')}
								<input
									type="text"
									bind:value={editCaption}
									oninput={() => (editSaved = false)}
									placeholder={t('media.picker.caption_placeholder', 'Optional caption/title')}
									class="mt-1 w-full rounded-md border px-2 py-1.5 text-sm font-normal"
									style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
								/>
							</label>
							<Button onclick={saveMetadata} disabled={editSaving} size="sm">
								{#if editSaving}
									<Loader2 class="me-1 h-3.5 w-3.5 animate-spin" />
								{/if}
								{t('media.picker.save', 'Save')}
							</Button>
							{#if canDeleteMedia}
								<Button
									variant="outline"
									size="sm"
									disabled={deleting}
									onclick={() => {
										// Clear a stale "Saved"/error so it can't render
										// alongside the confirm row.
										editSaved = false;
										editError = '';
										deleteConfirming = true;
									}}
								>
									<Trash2 class="me-1 h-3.5 w-3.5" />
									{t('media.picker.delete', 'Delete')}
								</Button>
							{/if}
						</div>
					{/if}
					{#if deleteConfirming && canDeleteMedia}
						<div class="flex items-center gap-2 text-xs" class:mt-2={panelMode === 'edit'}>
							<span class="flex-1" style="color: var(--color-foreground)">
								{t('media.picker.delete_confirm', 'Delete this image? This cannot be undone.')}
							</span>
							<Button variant="destructive" size="sm" disabled={deleting} onclick={deleteSelected}>
								{#if deleting}
									<Loader2 class="me-1 h-3 w-3 animate-spin" />
								{/if}
								{t('media.picker.delete', 'Delete')}
							</Button>
							<Button
								variant="outline"
								size="sm"
								disabled={deleting}
								onclick={() => {
									deleteConfirming = false;
									// 'delete' mode has no fields to return to —
									// close the panel rather than leave an empty box.
									if (panelMode === 'delete') panelMode = null;
								}}
							>
								{t('media.picker.cancel', 'Cancel')}
							</Button>
						</div>
					{/if}
					{#if editError}
						<p class="mt-2 text-xs text-danger">{editError}</p>
					{:else if deleteError}
						<p class="mt-2 text-xs text-danger">{deleteError}</p>
					{:else if editSaved}
						<p class="mt-2 text-xs" style="color: var(--color-success, var(--color-muted))">{t('media.picker.saved', 'Saved')}</p>
					{/if}
				</div>
			{/if}

			<!-- Confirm button -->
			<div class="flex justify-end gap-2 border-t pt-4" style="border-color: var(--color-border)">
				<Button variant="ghost" onclick={() => (open = false)}>{t('common.cancel', 'Cancel')}</Button>
				<Button onclick={handleConfirm} disabled={!selectedId}>
					{t('ads.media_select', 'Select Image')}
				</Button>
			</div>
		{:else if activeTab === 'upload'}
			<!-- Upload tab -->
			<div class="space-y-4">
				<!-- Drag-and-drop upload zone -->
				<div
					class="flex flex-col items-center justify-center gap-3 rounded-xl border-2 border-dashed py-10 transition-colors"
					style={dragActive
						? 'border-color: var(--color-primary); background: color-mix(in srgb, var(--color-primary) 5%, transparent)'
						: 'border-color: var(--color-border); background: var(--color-surface)'}
					ondragover={(e) => { e.preventDefault(); dragActive = true; }}
					ondragleave={() => { dragActive = false; }}
					ondrop={handleDrop}
					role="button"
					tabindex="0"
					onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') fileInput?.click(); }}
					onclick={() => fileInput?.click()}
				>
					<div class="flex h-12 w-12 items-center justify-center rounded-full" style="background: color-mix(in srgb, var(--color-primary) 10%, transparent)">
						<Upload class="h-6 w-6" style="color: var(--color-primary)" />
					</div>
					<div class="text-center">
						<p class="text-sm font-medium" style="color: var(--color-foreground)">
							{t('ads.media_drop_here', 'Drop image here or click to browse')}
						</p>
						<p class="text-xs mt-1" style="color: var(--color-muted)">
							{t('ads.media_upload_formats', 'PNG, JPG, WebP, GIF, SVG')}
						</p>
					</div>
					<input
						bind:this={fileInput}
						type="file"
						{accept}
						onchange={handleFileSelect}
						class="hidden"
					/>
				</div>

				{#if uploadFile}
					<div class="flex items-center gap-3 rounded-lg border p-3" style="border-color: var(--color-border); background-color: var(--color-surface)">
						<ImageIcon class="h-8 w-8" style="color: var(--color-muted)" />
						<div class="flex-1 min-w-0">
							<p class="truncate text-sm font-medium" style="color: var(--color-foreground)">{uploadFile.name}</p>
							<p class="text-xs" style="color: var(--color-muted)">{(uploadFile.size / 1024).toFixed(1)} KB</p>
						</div>
						<button type="button" onclick={() => (uploadFile = null)} class="hover:opacity-70" aria-label={t('common.remove', 'Remove')}>
							<X class="h-4 w-4" style="color: var(--color-muted)" />
						</button>
					</div>
				{/if}

				{#if uploadProgress}
					<p class="text-sm" style="color: var(--color-muted)">{uploadProgress}</p>
				{/if}

				<div class="flex justify-end gap-2 border-t pt-4" style="border-color: var(--color-border)">
					<Button variant="ghost" onclick={() => (open = false)}>{t('common.cancel', 'Cancel')}</Button>
					<Button onclick={handleUpload} disabled={!uploadFile || uploading}>
						{#if uploading}
							<Loader2 class="me-1 h-4 w-4 animate-spin" />
							{t('common.uploading', 'Uploading...')}
						{:else}
							<Upload class="me-1 h-4 w-4" />
							{t('ads.media_upload_button', 'Upload & Select')}
						{/if}
					</Button>
				</div>
			</div>
		{:else}
			<!-- Stock search tab -->
			<div class="space-y-4">
				<!-- Provider filter + search -->
				<div class="flex gap-2">
					{#if stockProviders.length > 0}
						<AutocompleteSelect
							bind:value={stockProviderFilter}
							onValueChange={() => { if (stockQuery.trim()) runStockSearch(); }}
							options={[
								{ value: '', label: t('ads.media_all_providers', 'All providers') },
								...stockProviders.map((p) => ({ value: p.name, label: p.display_name }))
							]}
							class="w-44"
						/>
					{/if}
					<div class="relative flex-1">
						<Search class="absolute start-3 top-1/2 h-4 w-4 -translate-y-1/2" style="color: var(--color-muted)" />
						<input
							type="text"
							value={stockQuery}
							oninput={handleStockSearchInput}
							placeholder={t('ads.media_stock_placeholder', 'Search stock photos...')}
							class="w-full rounded-md border py-2 ps-9 pe-3 text-sm"
							style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
						/>
					</div>
				</div>

				<!-- Stock results grid -->
				{#if stockLoading}
					<div class="flex items-center justify-center py-12">
						<Loader2 class="h-6 w-6 animate-spin" style="color: var(--color-muted)" />
					</div>
				{:else if stockResults.length === 0}
					<p class="py-12 text-center text-sm" style="color: var(--color-muted)">
						{stockQuery.trim() ? t('ads.media_stock_empty', 'No stock photos found') : t('ads.media_stock_hint', 'Type to search stock photo providers')}
					</p>
				{:else}
					<div class="grid max-h-80 grid-cols-2 gap-3 overflow-y-auto sm:grid-cols-4">
						{#each stockResults as photo (photo.provider + ':' + photo.id)}
							<div class="relative overflow-hidden rounded-lg border" style="border-color: var(--color-border)">
								<img src={photo.thumbnail_url} alt={photo.description ?? ''} class="h-20 w-full object-cover" loading="lazy" />
								<div class="absolute inset-0 flex items-center justify-center bg-black/50 opacity-0 transition-opacity hover:opacity-100">
									<button
										type="button"
										class="rounded-full p-2"
										style="background-color: var(--color-primary); color: white"
										onclick={() => importStockPhoto(photo)}
										disabled={stockImporting.has(`${photo.provider}:${photo.id}`)}
										aria-label={t('ads.media_stock_import', 'Import & select')}
									>
										{#if stockImporting.has(`${photo.provider}:${photo.id}`)}
											<Loader2 class="h-4 w-4 animate-spin" />
										{:else if stockImported.has(`${photo.provider}:${photo.id}`)}
											<Check class="h-4 w-4" />
										{:else}
											<Download class="h-4 w-4" />
										{/if}
									</button>
								</div>
								<p class="truncate px-1 py-0.5 text-[10px]" style="color: var(--color-muted)" title={photo.photographer}>
									{photo.photographer}
								</p>
							</div>
						{/each}
					</div>
				{/if}
			</div>
		{/if}
	</div>
</Modal>
