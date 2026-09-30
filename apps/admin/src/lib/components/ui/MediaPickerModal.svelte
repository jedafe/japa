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
	import { t } from '$lib/stores/i18n.svelte';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { Upload, Search, Image as ImageIcon, X, Loader2, Download, Check, ChevronLeft, ChevronRight } from 'lucide-svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';

	type MediaItem = {
		id: string;
		filename: string;
		mime_type: string;
		file_size: number;
		alt_text?: string | null;
		status: string;
		created_at: string;
		url?: string;
	};

	let {
		open = $bindable(false),
		onselect,
		accept = 'image/*'
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

	// Clear any pending debounce timers when the modal unmounts.
	$effect(() => {
		return () => {
			if (debounceTimer) clearTimeout(debounceTimer);
			if (stockTimer) clearTimeout(stockTimer);
		};
	});

	// fixMediaUrl — the backend returns imgproxy URLs (e.g.
	// http://imgproxy:8080/{sig}/{opts}/{base64_source}@{format}) which
	// don't resolve in dev. Decode the base64 source segment to recover
	// the s3:// path and construct a direct /api/v1/media/serve/ URL.
	function fixMediaUrl(url: string): string {
		if (!url) return '';
		// Already a relative serve URL — use as-is.
		if (url.startsWith('/api/v1/media/serve/')) return url;
		// Not an imgproxy URL — return as-is (data: URIs, external URLs, etc.)
		if (!url.includes('/plain/')) return url;
		try {
			// Format: {base}/{sig}/{opts}/plain/{base64_source}@{format}
			// or:     {base}/{sig}/{opts}/{base64_source}@{format}
			const parts = url.split('/');
			// Find the base64-encoded source segment — it's the part after
			// "plain" (or the segment containing @ if no plain prefix).
			let encoded: string | undefined;
			const plainIdx = parts.indexOf('plain');
			if (plainIdx >= 0 && parts[plainIdx + 1]) {
				encoded = parts[plainIdx + 1].split('@')[0];
			} else {
				// No "plain" — look for a segment with @ in it
				for (let i = parts.length - 1; i >= 0; i--) {
					if (parts[i].includes('@')) {
						encoded = parts[i].split('@')[0];
						break;
					}
				}
			}
			if (!encoded) return url;
			// base64url decode
			const decoded = atob(encoded.replace(/-/g, '+').replace(/_/g, '/'));
			// decoded = "s3://{bucket}/{path}"
			if (decoded.startsWith('s3://')) {
				const path = decoded.slice(5); // strip "s3://"
				return '/api/v1/media/serve/' + path;
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
			const formData = new FormData();
			formData.append('file', uploadFile);
			const result = await apiFetch<{ media_id: string; url: string }>(
				'/admin/media/upload',
				{ method: 'POST', body: formData }
			);
			if (result.error) {
				errorMessage = result.error.message || t('common.upload_error', 'Upload failed');
			} else if (result.data) {
				onselect({
					id: result.data.media_id,
					url: fixMediaUrl(result.data.url),
					filename: uploadFile.name
				});
				open = false;
				uploadFile = null;
			}
		} catch {
			errorMessage = t('common.upload_error', 'Upload failed');
		} finally {
			uploading = false;
			uploadProgress = '';
		}
	}

	function handleSelect(item: MediaItem) {
		selectedId = item.id;
	}

	function handleConfirm() {
		const selected = items.find((i) => i.id === selectedId);
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
		searchQuery = '';
		uploadFile = null;
		errorMessage = '';
		stockQuery = '';
		stockResults = [];
		currentPage = 0;
		totalPages = 1;
		totalItems = 0;
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

<Modal bind:open title={t('ads.media_picker_title', 'Select Image')} onclose={handleClose}>
	<div class="space-y-4">
		{#if errorMessage}
			<div class="rounded-lg border border-danger/30 bg-danger/10 px-4 py-3 text-sm text-danger">
				{errorMessage}
			</div>
		{/if}

		<!-- Tab switcher -->
		<div class="flex gap-2 border-b" style="border-color: var(--color-border)">
			<button
				type="button"
				class="px-3 py-2 text-sm font-medium transition-colors"
				style={activeTab === 'browse'
					? 'border-bottom: 2px solid var(--color-primary); color: var(--color-primary)'
					: 'color: var(--color-muted)'}
				onclick={() => (activeTab = 'browse')}
			>
				<ImageIcon class="mr-1 inline h-4 w-4" />
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
				<Upload class="mr-1 inline h-4 w-4" />
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
				<Search class="mr-1 inline h-4 w-4" />
				{t('ads.media_stock', 'Stock Photos')}
			</button>
		</div>

		{#if activeTab === 'browse'}
			<!-- Search bar — always visible so typing doesn't lose focus -->
			<div class="relative">
				<Search class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2" style="color: var(--color-muted)" />
				<input
					type="text"
					value={searchQuery}
					oninput={handleSearchInput}
					placeholder={t('ads.media_search_placeholder', 'Search media...')}
					class="w-full rounded-md border py-2 pl-9 pr-3 text-sm"
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
				<div class="grid max-h-80 grid-cols-4 gap-3 overflow-y-auto">
					{#each filteredItems as item (item.id)}
						<button
							type="button"
							class="relative overflow-hidden rounded-lg border-2 transition-all"
							style={selectedId === item.id
								? 'border-color: var(--color-primary); box-shadow: 0 0 0 2px var(--color-primary)'
								: 'border-color: var(--color-border)'}
							onclick={() => handleSelect(item)}
						>
							{#if item.url}
								<img src={fixMediaUrl(item.url)} alt={item.alt_text || item.filename} class="h-20 w-full object-cover" loading="lazy" />
							{:else}
								<div class="flex h-20 items-center justify-center" style="background-color: var(--color-surface)">
									<ImageIcon class="h-8 w-8" style="color: var(--color-muted)" />
								</div>
							{/if}
							<p class="truncate px-1 py-1 text-xs" style="color: var(--color-muted)" title={item.filename}>
								{item.filename}
							</p>
						</button>
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
								<ChevronLeft class="h-4 w-4" />
							</Button>
							<Button
								variant="outline"
								size="sm"
								disabled={currentPage >= totalPages - 1 || loading}
								onclick={() => goToPage(currentPage + 1)}
							>
								<ChevronRight class="h-4 w-4" />
							</Button>
						</div>
					</div>
				{/if}
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
							<Loader2 class="mr-1 h-4 w-4 animate-spin" />
							{t('common.uploading', 'Uploading...')}
						{:else}
							<Upload class="mr-1 h-4 w-4" />
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
						<Search class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2" style="color: var(--color-muted)" />
						<input
							type="text"
							value={stockQuery}
							oninput={handleStockSearchInput}
							placeholder={t('ads.media_stock_placeholder', 'Search stock photos...')}
							class="w-full rounded-md border py-2 pl-9 pr-3 text-sm"
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
					<div class="grid max-h-80 grid-cols-4 gap-3 overflow-y-auto">
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
