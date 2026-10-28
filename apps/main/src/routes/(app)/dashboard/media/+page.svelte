<script lang="ts">
	// Ref: japaconnect-engineering/sections/34-media-system.md (user media library)
	// Grid/list of the user's uploaded media with filter, search, pagination,
	// bulk actions, upload, and per-item actions (edit, delete, copy URL, details).
	// All data comes from /me/media, PATCH /media/{id}, DELETE /media/{id},
	// GET /media/{id}, and POST /media/bulk-delete — Rule 16.
	import { t } from '$lib/i18n';
	import { untrack } from 'svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import PaginationControls from '$lib/components/ui/PaginationControls.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import MediaLibraryModal from '$lib/components/media/MediaLibraryModal.svelte';
	import { Modal } from '$lib/components/ui';
	import {
		Image as ImageIcon,
		File,
		Search,
		Loader2,
		Pencil,
		Trash2,
		Copy,
		Check,
		X,
		Upload,
		LayoutGrid,
		List,
		Eye,
		Maximize2,
		AlertTriangle
	} from 'lucide-svelte';
	import { openLightbox } from '$lib/lightbox.svelte';
	import AppealButton from '$lib/components/appeals/AppealButton.svelte';
	import type { MediaAsset } from '$lib/types';
	import { listMyMedia, getMedia, updateMedia, deleteMedia, bulkDeleteMedia } from '$lib/api/create';

	let { data } = $props();

	const PAGE_SIZE = 48;

	type TypeFilter = '' | 'image/' | 'video/' | 'audio/' | 'application/';
	type StatusFilter = '' | 'approved' | 'pending' | 'quarantined' | 'rejected';
	type ViewMode = 'grid' | 'list';

	let items = $state<MediaAsset[]>(untrack(() => data.media?.items ?? []));
	let total = $state<number>(untrack(() => data.media?.total ?? 0));
	let totalPages = $state<number>(untrack(() => data.media?.total_pages ?? 1));

	// Re-sync the first page when the server load re-runs (invalidateAll or
	// a fresh SSR navigation). Client-side paging/filtering owns the state
	// between load runs, so this only fires on a new `data` payload.
	$effect(() => {
		items = data.media?.items ?? [];
		total = data.media?.total ?? 0;
		totalPages = data.media?.total_pages ?? 1;
	});
	let page = $state(1);
	let typeFilter = $state<TypeFilter>('');
	let statusFilter = $state<StatusFilter>('');
	let flaggedOnly = $state(false);
	let missingOnly = $state(false);
	let search = $state('');
	let viewMode = $state<ViewMode>('grid');
	let loading = $state(false);
	let error = $state<string | null>(null);

	let searchTimer: ReturnType<typeof setTimeout> | null = null;

	// Upload modal
	let showUpload = $state(false);

	// Bulk selection
	let selectedIds = $state<Set<string>>(new Set());
	let showBulkDelete = $state(false);
	let bulkDeleteSaving = $state(false);
	let bulkDeleteError = $state<string | null>(null);

	// Edit dialog
	let showEdit = $state(false);
	let editTarget = $state<MediaAsset | null>(null);
	let editAltText = $state('');
	let editCaption = $state('');
	let editSaving = $state(false);
	let editError = $state<string | null>(null);

	// Delete dialog
	let showDelete = $state(false);
	let deleteTarget = $state<MediaAsset | null>(null);
	let deleteSaving = $state(false);
	let deleteError = $state<string | null>(null);

	// Details dialog
	let showDetails = $state(false);
	let detailsTarget = $state<MediaAsset | null>(null);
	let detailsLoading = $state(false);
	let detailsError = $state<string | null>(null);

	// Copy-URL feedback
	let copiedId = $state<string | null>(null);

	const typeOptions = [
		{ value: '', label: t('media.library.all_types') },
		{ value: 'image/', label: t('media.library.type_images') },
		{ value: 'video/', label: t('media.library.type_videos') },
		{ value: 'audio/', label: t('media.library.type_audio') },
		{ value: 'application/', label: t('media.library.type_documents') }
	];

	const statusOptions = [
		{ value: '', label: t('media.library.status_all', 'All statuses') },
		{ value: 'approved', label: t('media.library.status_approved', 'Approved') },
		{ value: 'pending', label: t('media.library.status_pending', 'Pending') },
		{ value: 'quarantined', label: t('media.library.status_quarantined', 'Quarantined') },
		{ value: 'rejected', label: t('media.library.status_rejected', 'Rejected') }
	];

	function isImage(m: MediaAsset): boolean {
		return (m.mime_type ?? '').startsWith('image/');
	}
	function isVideo(m: MediaAsset): boolean {
		return (m.mime_type ?? '').startsWith('video/');
	}
	function formatSize(bytes?: number): string {
		if (!bytes) return '—';
		if (bytes < 1024) return `${bytes} B`;
		if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} KB`;
		return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
	}
	function formatDate(iso?: string): string {
		if (!iso) return '—';
		try {
			return new Date(iso).toLocaleString();
		} catch {
			return iso;
		}
	}
	function statusClass(status?: string): string {
		switch (status) {
			case 'approved':
				return 'bg-emerald-100 text-emerald-800';
			case 'pending':
				return 'bg-amber-100 text-amber-800';
			case 'quarantined':
				return 'bg-orange-100 text-orange-800';
			case 'rejected':
				return 'bg-red-100 text-red-800';
			default:
				return 'bg-gray-100 text-gray-800';
		}
	}

	async function fetchPage() {
		loading = true;
		error = null;
		const offset = (page - 1) * PAGE_SIZE;
		const res = await listMyMedia({
			type: typeFilter || undefined,
			q: search || undefined,
			status: statusFilter || undefined,
			flagged: flaggedOnly || undefined,
			missing: missingOnly ? 'only' : undefined,
			page_size: PAGE_SIZE,
			page_offset: offset
		});
		if (res.error || !res.data) {
			error = res.error?.message ?? 'Failed to load media';
			items = [];
			total = 0;
			totalPages = 1;
		} else {
			items = res.data;
			const envelope = res.envelope as { total?: number; total_pages?: number } | null;
			total = envelope?.total ?? res.data.length;
			totalPages = envelope?.total_pages ?? 1;
			// Clear any selections that are no longer on this page after a refresh.
			selectedIds = new Set([...selectedIds].filter((id) => items.some((i) => i.id === id)));
		}
		loading = false;
	}

	function onTypeChange(v: string) {
		typeFilter = v as TypeFilter;
		page = 1;
		fetchPage();
	}

	function onStatusChange(v: string) {
		statusFilter = v as StatusFilter;
		page = 1;
		fetchPage();
	}

	function onFlaggedToggle() {
		flaggedOnly = !flaggedOnly;
		page = 1;
		fetchPage();
	}

	function onMissingToggle() {
		missingOnly = !missingOnly;
		page = 1;
		fetchPage();
	}

	function onSearchInput(v: string) {
		search = v;
		if (searchTimer) clearTimeout(searchTimer);
		searchTimer = setTimeout(() => {
			page = 1;
			fetchPage();
		}, 350);
	}

	function onPageChange(p: number) {
		page = p;
		fetchPage();
	}

	function toggleSelect(id: string) {
		const next = new Set(selectedIds);
		if (next.has(id)) next.delete(id);
		else next.add(id);
		selectedIds = next;
	}

	function selectAllCurrent() {
		if (allCurrentSelected) {
			const next = new Set(selectedIds);
			for (const m of items) next.delete(m.id);
			selectedIds = next;
		} else {
			const next = new Set(selectedIds);
			for (const m of items) next.add(m.id);
			selectedIds = next;
		}
	}

	function clearSelection() {
		selectedIds = new Set();
	}

	let allCurrentSelected = $derived(items.length > 0 && items.every((m) => selectedIds.has(m.id)));
	let selectedCount = $derived(selectedIds.size);

	function openUpload() {
		showUpload = true;
	}
	function closeUpload() {
		showUpload = false;
	}
	function onUploadSelect(_media: MediaAsset) {
		showUpload = false;
		fetchPage();
	}

	function openBulkDelete() {
		showBulkDelete = true;
	}

	async function confirmBulkDelete() {
		if (selectedCount === 0) return;
		bulkDeleteSaving = true;
		bulkDeleteError = null;
		const res = await bulkDeleteMedia([...selectedIds]);
		if (res.error) {
			bulkDeleteError = res.error.message ?? t('media.library.bulk_delete_error', 'Failed to delete selected media');
		} else {
			showBulkDelete = false;
			selectedIds = new Set();
			fetchPage();
		}
		bulkDeleteSaving = false;
	}

	function openEdit(m: MediaAsset) {
		editTarget = m;
		editAltText = m.alt_text ?? '';
		editCaption = m.caption ?? '';
		editError = null;
		editSaving = false;
		showEdit = true;
	}

	function closeEdit() {
		showEdit = false;
		editTarget = null;
	}

	async function saveEdit() {
		if (!editTarget) return;
		editSaving = true;
		editError = null;
		const res = await updateMedia(editTarget.id, { alt_text: editAltText, caption: editCaption });
		if (res.error) {
			editError = res.error.message ?? t('media.library.edit_error');
		} else {
			closeEdit();
			fetchPage();
		}
		editSaving = false;
	}

	function openDelete(m: MediaAsset) {
		deleteTarget = m;
		deleteError = null;
		deleteSaving = false;
		showDelete = true;
	}

	function closeDelete() {
		showDelete = false;
		deleteTarget = null;
	}

	async function confirmDelete() {
		if (!deleteTarget) return;
		deleteSaving = true;
		deleteError = null;
		const res = await deleteMedia(deleteTarget.id);
		if (res.error) {
			deleteError = res.error.message ?? t('media.library.delete_error');
		} else {
			closeDelete();
			fetchPage();
		}
		deleteSaving = false;
	}

	function openDetails(m: MediaAsset) {
		detailsTarget = m;
		detailsError = null;
		detailsLoading = true;
		showDetails = true;
		getMedia(m.id)
			.then((res) => {
				if (res.data) detailsTarget = res.data;
				detailsError = res.error?.message ?? null;
			})
			.finally(() => {
				detailsLoading = false;
			});
	}

	function closeDetails() {
		showDetails = false;
		detailsTarget = null;
		detailsError = null;
	}

	async function copyUrl(m: MediaAsset) {
		if (!m.url) return;
		try {
			await navigator.clipboard.writeText(m.url);
			copiedId = m.id;
			setTimeout(() => (copiedId = null), 1500);
		} catch {
			// Clipboard may be unavailable; silently ignore.
		}
	}

	let rangeStart = $derived(total === 0 ? 0 : (page - 1) * PAGE_SIZE + 1);
	let rangeEnd = $derived(Math.min(page * PAGE_SIZE, total));
</script>

<section class="space-y-4">
	<header class="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
		<div>
			<h1 class="text-lg font-medium text-[var(--color-text)]">{t('dashboard.nav.media')}</h1>
			<p class="text-sm text-[var(--color-muted)]">{t('media.subtitle')}</p>
		</div>
		<div class="flex items-center gap-2">
			<button
				type="button"
				class="inline-flex items-center gap-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]"
				onclick={openUpload}
			>
				<Upload size={16} />
				{t('media.library.upload', 'Upload new')}
			</button>
			<div class="flex items-center rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-1">
				<button
					type="button"
					class="rounded p-1.5 text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)]"
					class:bg-[var(--color-surface-subtle)]={viewMode === 'grid'}
					class:text-[var(--color-text)]={viewMode === 'grid'}
					onclick={() => (viewMode = 'grid')}
					aria-label={t('media.library.grid_view', 'Grid view')}
				>
					<LayoutGrid size={16} />
				</button>
				<button
					type="button"
					class="rounded p-1.5 text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)]"
					class:bg-[var(--color-surface-subtle)]={viewMode === 'list'}
					class:text-[var(--color-text)]={viewMode === 'list'}
					onclick={() => (viewMode = 'list')}
					aria-label={t('media.library.list_view', 'List view')}
				>
					<List size={16} />
				</button>
			</div>
		</div>
	</header>

	<!-- Filters -->
	<div class="flex flex-wrap items-center gap-3">
		<div class="relative flex-1">
			<Search size={16} class="pointer-events-none absolute start-3 top-1/2 -translate-y-1/2 text-[var(--color-muted)]" />
			<input
				type="text"
				class="w-full min-w-0 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] py-2 ps-9 pe-3 text-sm text-[var(--color-text)] outline-none transition-colors focus:border-[var(--color-primary)] sm:w-64"
				placeholder={t('media.library.search_placeholder')}
				value={search}
				oninput={(e) => onSearchInput(e.currentTarget.value)}
			/>
		</div>
		<div class="w-40">
			<AutocompleteSelect
				value={typeFilter}
				options={typeOptions}
				ariaLabel={t('media.library.filter_type')}
				onValueChange={onTypeChange}
			/>
		</div>
		<div class="w-44">
			<AutocompleteSelect
				value={statusFilter}
				options={statusOptions}
				ariaLabel={t('media.library.status', 'Status')}
				onValueChange={onStatusChange}
			/>
		</div>
		<button
			type="button"
			class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border px-3 py-2 text-sm transition-colors"
			class:border-[var(--color-border)]={!flaggedOnly}
			class:bg-[var(--color-surface)]={!flaggedOnly}
			class:text-[var(--color-muted)]={!flaggedOnly}
			class:border-[var(--destructive,#dc2626)]={flaggedOnly}
			class:text-[var(--destructive,#dc2626)]={flaggedOnly}
			class:bg-red-50={flaggedOnly}
			onclick={onFlaggedToggle}
		>
			<AlertTriangle size={14} />
			{t('media.library.flagged_only', 'Flagged only')}
		</button>
		<button
			type="button"
			class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border px-3 py-2 text-sm transition-colors"
			class:border-[var(--color-border)]={!missingOnly}
			class:bg-[var(--color-surface)]={!missingOnly}
			class:text-[var(--color-muted)]={!missingOnly}
			class:border-[var(--destructive,#dc2626)]={missingOnly}
			class:text-[var(--destructive,#dc2626)]={missingOnly}
			class:bg-red-50={missingOnly}
			onclick={onMissingToggle}
		>
			<ImageIcon size={14} />
			{t('media.library.missing_only', 'Missing objects')}
		</button>
	</div>

	{#if error}
		<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3 text-sm text-[var(--destructive, #dc2626)]">
			{error}
		</div>
	{/if}

	{#if selectedCount > 0}
		<div class="flex items-center justify-between gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
			<p class="text-sm text-[var(--color-text)]">
				{t('media.library.selected_count', '{count} selected', { count: selectedCount })}
			</p>
			<div class="flex items-center gap-2">
				<button
					type="button"
					class="text-sm text-[var(--color-muted)] hover:text-[var(--color-text)]"
					onclick={clearSelection}
				>
					{t('media.library.clear_selection', 'Clear selection')}
				</button>
				<button
					type="button"
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--destructive,#dc2626)] px-3 py-2 text-sm text-white hover:opacity-90"
					onclick={openBulkDelete}
				>
					<Trash2 size={14} />
					{t('media.library.bulk_delete', 'Delete selected')}
				</button>
			</div>
		</div>
	{/if}

	{#if loading}
		<div class="flex items-center justify-center py-12 text-[var(--color-muted)]">
			<Loader2 size={24} class="animate-spin" />
		</div>
	{:else if items.length === 0}
		<EmptyState
			icon={ImageIcon}
			title={t('media.empty_title')}
			body={t('media.empty_body')}
		/>
	{:else if viewMode === 'grid'}
		<ul class="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
			{#each items as m (m.id)}
				<li class="group overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]">
					<div class="relative flex aspect-square items-center justify-center bg-[var(--color-surface-subtle)]">
						{#if isImage(m)}
							<img src={m.url} alt={m.alt_text ?? m.filename ?? ''} class="h-full w-full object-cover" loading="lazy" />
						{:else if isVideo(m)}
							<video src={m.url} class="h-full w-full object-cover" muted></video>
						{:else}
							<File class="h-8 w-8 text-[var(--color-muted)]" />
						{/if}
						{#if m.flagged}
							<div class="absolute start-2 top-2 flex items-center gap-1 rounded bg-red-500 px-1.5 py-0.5 text-[10px] font-medium text-white">
								<AlertTriangle size={10} />
								{t('media.library.flagged', 'Flagged')}
							</div>
						{/if}
						<div class="absolute end-2 top-2 z-10">
							<input
								type="checkbox"
								checked={selectedIds.has(m.id)}
								onchange={() => toggleSelect(m.id)}
								class="h-4 w-4 accent-[var(--color-primary)]"
							/>
						</div>
						<!-- Hover action overlay -->
						<div class="absolute inset-0 flex items-center justify-center gap-2 bg-black/40 opacity-0 transition-opacity group-hover:opacity-100">
							<button
								type="button"
								class="inline-flex h-8 w-8 items-center justify-center rounded-[var(--radius)] border border-white/30 bg-white/10 text-white backdrop-blur-sm transition-colors hover:bg-white/20"
								onclick={() => openDetails(m)}
								aria-label={t('media.library.view_details', 'View details')}
							>
								<Eye size={14} />
							</button>
							<button
								type="button"
								class="inline-flex h-8 w-8 items-center justify-center rounded-[var(--radius)] border border-white/30 bg-white/10 text-white backdrop-blur-sm transition-colors hover:bg-white/20"
								onclick={() => openEdit(m)}
								aria-label={t('media.library.edit')}
							>
								<Pencil size={14} />
							</button>
							<button
								type="button"
								class="inline-flex h-8 w-8 items-center justify-center rounded-[var(--radius)] border border-white/30 bg-white/10 text-white backdrop-blur-sm transition-colors hover:bg-white/20"
								onclick={() => copyUrl(m)}
								aria-label={t('media.library.copy_url')}
							>
								{#if copiedId === m.id}
									<Check size={14} />
								{:else}
									<Copy size={14} />
								{/if}
							</button>
							<button
								type="button"
								class="inline-flex h-8 w-8 items-center justify-center rounded-[var(--radius)] border border-white/30 bg-white/10 text-[var(--destructive, #fca5a5)] backdrop-blur-sm transition-colors hover:bg-white/20"
								onclick={() => openDelete(m)}
								aria-label={t('media.library.delete')}
							>
								<Trash2 size={14} />
							</button>
						</div>
					</div>
					<div class="p-2">
						<p class="truncate text-xs font-medium text-[var(--color-text)]">{m.filename ?? m.id.slice(0, 8)}</p>
						<div class="mt-0.5 flex items-center justify-between text-[10px] text-[var(--color-muted)]">
							<span>{formatSize(m.file_size)}</span>
							{#if m.status}
								<span class="rounded px-1.5 py-0.5 {statusClass(m.status)}">{m.status}</span>
							{/if}
						</div>
						{#if m.status === 'rejected' || m.status === 'quarantined'}
							<div class="mt-1.5">
								<AppealButton entityType="media" entityId={m.id} class="!px-2 !py-1 text-xs" />
							</div>
						{/if}
					</div>
				</li>
			{/each}
		</ul>
	{:else}
		<div class="overflow-x-auto rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]">
			<table class="w-full text-sm">
				<thead class="border-b border-[var(--color-border)] bg-[var(--color-surface-subtle)] text-start text-xs text-[var(--color-muted)]">
					<tr>
						<th class="w-10 px-3 py-2">
							<input
								type="checkbox"
								checked={allCurrentSelected}
								onchange={selectAllCurrent}
								class="h-4 w-4 accent-[var(--color-primary)]"
							/>
						</th>
						<th class="px-3 py-2">{t('media.library.filename', 'Filename')}</th>
						<th class="px-3 py-2">{t('media.library.type', 'Type')}</th>
						<th class="px-3 py-2">{t('media.size', 'Size')}</th>
						<th class="px-3 py-2">{t('media.status', 'Status')}</th>
						<th class="px-3 py-2">{t('media.uploaded', 'Uploaded')}</th>
						<th class="px-3 py-2 text-end">{t('media.library.actions', 'Actions')}</th>
					</tr>
				</thead>
				<tbody class="divide-y divide-[var(--color-border)]">
					{#each items as m (m.id)}
						<tr class="hover:bg-[var(--color-surface-subtle)]">
							<td class="px-3 py-2">
								<input
									type="checkbox"
									checked={selectedIds.has(m.id)}
									onchange={() => toggleSelect(m.id)}
									class="h-4 w-4 accent-[var(--color-primary)]"
								/>
							</td>
							<td class="px-3 py-2">
								<div class="flex items-center gap-2">
									<div class="h-8 w-8 overflow-hidden rounded-[var(--radius)] bg-[var(--color-surface-subtle)]">
										{#if isImage(m)}
											<img src={m.url} alt="" class="h-full w-full object-cover" loading="lazy" />
										{:else if isVideo(m)}
											<video src={m.url} class="h-full w-full object-cover" muted></video>
										{:else}
											<File class="h-4 w-4 text-[var(--color-muted)]" />
										{/if}
									</div>
									<span class="truncate font-medium">{m.filename ?? m.id.slice(0, 8)}</span>
								</div>
							</td>
							<td class="px-3 py-2 text-[var(--color-muted)]">{m.mime_type ?? '—'}</td>
							<td class="px-3 py-2 text-[var(--color-muted)]">{formatSize(m.file_size)}</td>
							<td class="px-3 py-2">
								<div class="flex items-center gap-2">
									{#if m.status}
										<span class="rounded px-1.5 py-0.5 text-xs {statusClass(m.status)}">{m.status}</span>
									{/if}
									{#if m.flagged}
										<span class="inline-flex items-center gap-1 rounded bg-red-500 px-1.5 py-0.5 text-xs text-white">
											<AlertTriangle size={10} />
											{t('media.library.flagged', 'Flagged')}
										</span>
									{/if}
									{#if m.status === 'rejected' || m.status === 'quarantined'}
										<AppealButton
											entityType="media"
											entityId={m.id}
											label={t('appeals.appeal', 'Appeal')}
											class="!px-2 !py-1 text-xs"
										/>
									{/if}
								</div>
							</td>
							<td class="px-3 py-2 text-[var(--color-muted)]">{formatDate(m.created_at)}</td>
							<td class="px-3 py-2 text-end">
								<div class="flex items-center justify-end gap-1">
									<button
										type="button"
										class="inline-flex h-7 w-7 items-center justify-center rounded text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)]"
										onclick={() => openDetails(m)}
										aria-label={t('media.library.view_details')}
									>
										<Eye size={14} />
									</button>
									<button
										type="button"
										class="inline-flex h-7 w-7 items-center justify-center rounded text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)]"
										onclick={() => openEdit(m)}
										aria-label={t('media.library.edit')}
									>
										<Pencil size={14} />
									</button>
									<button
										type="button"
										class="inline-flex h-7 w-7 items-center justify-center rounded text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)]"
										onclick={() => copyUrl(m)}
										aria-label={t('media.library.copy_url')}
									>
										{#if copiedId === m.id}
											<Check size={14} />
										{:else}
											<Copy size={14} />
										{/if}
									</button>
									<button
										type="button"
										class="inline-flex h-7 w-7 items-center justify-center rounded text-[var(--destructive,#dc2626)] hover:bg-red-50"
										onclick={() => openDelete(m)}
										aria-label={t('media.library.delete')}
									>
										<Trash2 size={14} />
									</button>
								</div>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}

	<!-- Pagination -->
	{#if items.length > 0}
		<div class="flex flex-col items-center justify-between gap-3 sm:flex-row">
			<p class="text-xs text-[var(--color-muted)]">
				{t('media.library.page_info', 'Showing {start}–{end} of {total}')
					.replace('{start}', String(rangeStart))
					.replace('{end}', String(rangeEnd))
					.replace('{total}', String(total))}
			</p>
			<PaginationControls {page} {totalPages} onPageChange={onPageChange} />
		</div>
	{/if}
</section>

<!-- Upload modal -->
{#if showUpload}
	<MediaLibraryModal
		onSelect={onUploadSelect}
		onClose={closeUpload}
		allowedType=""
		initialTab="upload"
	/>
{/if}

<!-- Bulk delete confirmation -->
<Modal open={showBulkDelete} onOpenChange={(v) => { if (!v) showBulkDelete = false; }} title={t('media.library.bulk_delete_title', 'Delete selected media')} description={t('media.library.bulk_delete_confirm', 'Delete {count} selected media files? This cannot be undone.', { count: selectedCount })} overlayClass="backdrop-blur-sm" zIndex={100} size="md">
	{#snippet children(close)}
		{#if bulkDeleteError}
			<p class="mb-3 text-xs text-[var(--destructive, #dc2626)]">{bulkDeleteError}</p>
		{/if}
		<div class="flex justify-end gap-2">
			<button type="button" onclick={close} class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]">
				{t('media.picker.cancel')}
			</button>
			<button
				type="button"
				class="rounded-[var(--radius)] bg-[var(--destructive,#dc2626)] px-3 py-2 text-sm text-white hover:opacity-90 disabled:opacity-50"
				onclick={confirmBulkDelete}
				disabled={bulkDeleteSaving}
			>
				{#if bulkDeleteSaving}
					<Loader2 size={14} class="animate-spin" />
				{:else}
					{t('media.library.bulk_delete')}
				{/if}
			</button>
		</div>
	{/snippet}
</Modal>

<!-- Edit dialog -->
<Modal open={showEdit} onOpenChange={(v) => { if (!v) closeEdit(); }} title={t('media.library.edit_title')} overlayClass="backdrop-blur-sm" zIndex={100} size="md">
	{#snippet children(close)}
			<div class="space-y-3">
				<div>
					<label for="edit-alt" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">{t('media.library.edit_alt')}</label>
					<p class="mt-1 text-xs text-[var(--color-muted)]">{t('media.alt_hint', 'Describes the image for screen readers and SEO.')}</p>
					<input
						id="edit-alt"
						type="text"
						class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
						bind:value={editAltText}
					/>
				</div>
				<div>
					<label for="edit-caption" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">{t('media.library.edit_caption')}</label>
					<p class="mt-1 text-xs text-[var(--color-muted)]">{t('media.caption_hint', 'Optional caption shown under the media.')}</p>
					<input
						id="edit-caption"
						type="text"
						class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
						bind:value={editCaption}
					/>
				</div>
				{#if editError}
					<p class="text-xs text-[var(--destructive, #dc2626)]">{editError}</p>
				{/if}
			</div>
			<div class="mt-5 flex justify-end gap-2">
				<button type="button" onclick={close} class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]">
					{t('media.picker.cancel')}
				</button>
				<button
					type="button"
					class="rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-2 text-sm text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
					onclick={saveEdit}
					disabled={editSaving}
				>
					{#if editSaving}
						<Loader2 size={14} class="animate-spin" />
					{:else}
						{t('media.library.edit_save')}
					{/if}
				</button>
			</div>
	{/snippet}
</Modal>

<!-- Delete confirmation dialog -->
<Modal open={showDelete} onOpenChange={(v) => { if (!v) closeDelete(); }} title={t('media.library.delete_title')} description={t('media.library.delete_confirm')} overlayClass="backdrop-blur-sm" zIndex={100} size="md">
	{#snippet children(close)}
		{#if deleteError}
			<p class="mb-3 text-xs text-[var(--destructive, #dc2626)]">{deleteError}</p>
		{/if}
		<div class="flex justify-end gap-2">
			<button type="button" onclick={close} class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]">
				{t('media.picker.cancel')}
			</button>
			<button
				type="button"
				class="rounded-[var(--radius)] bg-[var(--destructive,#dc2626)] px-3 py-2 text-sm text-white hover:opacity-90 disabled:opacity-50"
				onclick={confirmDelete}
				disabled={deleteSaving}
			>
				{#if deleteSaving}
					<Loader2 size={14} class="animate-spin" />
				{:else}
					{t('media.library.delete')}
				{/if}
			</button>
		</div>
	{/snippet}
</Modal>

<!-- Details dialog -->
<Modal open={showDetails} onOpenChange={(v) => { if (!v) closeDetails(); }} hideHeader ariaTitle={t('media.library.details_title', 'Media details')} padded={false} overlayClass="backdrop-blur-sm" zIndex={100} size="max-w-2xl">
	{#snippet children(close)}
			<div class="flex items-center justify-between border-b border-[var(--color-border)] p-4">
				<h2 class="text-base font-medium text-[var(--color-text)]">{t('media.library.details_title', 'Media details')}</h2>
				<button type="button" onclick={close} aria-label={t('common.close', 'Close')} class="text-[var(--color-muted)] hover:text-[var(--color-text)]">
					<X size={18} />
				</button>
			</div>
			<div class="flex-1 overflow-y-auto p-4">
				{#if detailsLoading && !detailsTarget}
					<div class="flex items-center justify-center py-12 text-[var(--color-muted)]">
						<Loader2 size={24} class="animate-spin" />
					</div>
				{:else if detailsError}
					<p class="text-sm text-[var(--destructive, #dc2626)]">{detailsError}</p>
				{:else if detailsTarget}
					{@const m = detailsTarget}
					<div class="flex flex-col gap-4 md:flex-row">
						<div class="group relative shrink-0 overflow-hidden rounded-[var(--radius)] bg-[var(--color-surface-subtle)] md:w-64">
							{#if isImage(m)}
								<img src={m.url} alt={m.alt_text ?? m.filename ?? ''} class="h-64 w-full object-contain md:h-48" loading="lazy" />
							{:else if isVideo(m)}
								<video src={m.url} class="h-64 w-full object-contain md:h-48" controls muted></video>
							{:else}
								<div class="flex h-64 items-center justify-center text-[var(--color-muted)] md:h-48">
									<File size={48} />
								</div>
							{/if}
							{#if isImage(m) || isVideo(m)}
								<button
									type="button"
									class="absolute end-2 top-2 grid h-8 w-8 place-items-center rounded-full bg-black/55 text-white opacity-0 transition-opacity hover:bg-black/75 focus-visible:opacity-100 group-hover:opacity-100"
									onclick={() => openLightbox(items, items.findIndex((i) => i.id === m.id))}
									aria-label={t('media.view_full', 'View full size')}
								>
									<Maximize2 size={15} />
								</button>
							{/if}
						</div>
						<div class="flex-1 space-y-3 text-sm">
							<div class="grid grid-cols-[6rem_1fr] gap-2">
								<span class="text-[var(--color-muted)]">{t('media.filename', 'Filename')}</span>
								<span class="break-all text-[var(--color-text)]">{m.filename ?? '—'}</span>

								<span class="text-[var(--color-muted)]">{t('media.type', 'Type')}</span>
								<span class="text-[var(--color-text)]">{m.mime_type ?? '—'}</span>

								<span class="text-[var(--color-muted)]">{t('media.size', 'Size')}</span>
								<span class="text-[var(--color-text)]">{formatSize(m.file_size)}</span>

								<span class="text-[var(--color-muted)]">{t('media.library.details_dimensions', 'Dimensions')}</span>
								<span class="text-[var(--color-text)]">{m.width && m.height ? `${m.width}×${m.height} px` : '—'}</span>

								{#if m.duration_secs}
									<span class="text-[var(--color-muted)]">{t('media.library.details_duration', 'Duration')}</span>
									<span class="text-[var(--color-text)]">{m.duration_secs}s</span>
								{/if}

								<span class="text-[var(--color-muted)]">{t('media.status', 'Status')}</span>
								<span class="inline-flex items-center gap-2 text-[var(--color-text)]">
									{#if m.status}
										<span class="rounded px-1.5 py-0.5 text-xs {statusClass(m.status)}">{m.status}</span>
									{/if}
									{#if m.flagged}
										<span class="inline-flex items-center gap-1 rounded bg-red-500 px-1.5 py-0.5 text-xs text-white">
											<AlertTriangle size={10} />
											{t('media.library.flagged')}
										</span>
									{/if}
								</span>

								<span class="text-[var(--color-muted)]">{t('media.library.details_uploaded', 'Uploaded')}</span>
								<span class="text-[var(--color-text)]">{formatDate(m.created_at)}</span>

								{#if m.alt_text}
									<span class="text-[var(--color-muted)]">{t('media.library.edit_alt')}</span>
									<span class="text-[var(--color-text)]">{m.alt_text}</span>
								{/if}
								{#if m.caption}
									<span class="text-[var(--color-muted)]">{t('media.library.edit_caption')}</span>
									<span class="text-[var(--color-text)]">{m.caption}</span>
								{/if}
							</div>

							{#if m.url}
								<div class="pt-2">
									<label for="details-url" class="mb-1 block text-xs text-[var(--color-muted)]">{t('media.library.details_url', 'URL')}</label>
									<div class="flex items-center gap-2">
										<input
											id="details-url"
											type="text"
											readonly
											value={m.url}
											class="min-w-0 flex-1 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1.5 text-xs text-[var(--color-text)]"
										/>
										<button
											type="button"
											class="inline-flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] px-2 py-1.5 text-xs hover:bg-[var(--color-surface-subtle)]"
											onclick={() => copyUrl(m)}
										>
											{#if copiedId === m.id}
												<Check size={12} />
											{:else}
												<Copy size={12} />
											{/if}
											{t('media.library.copy_url')}
										</button>
									</div>
								</div>
							{/if}
						</div>
					</div>
				{/if}
			</div>
			<div class="border-t border-[var(--color-border)] p-4 text-end">
				<button type="button" onclick={close} class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]">
					{t('media.picker.cancel')}
				</button>
			</div>
	{/snippet}
</Modal>
