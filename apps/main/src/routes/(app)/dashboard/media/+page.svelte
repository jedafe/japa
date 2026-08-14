<script lang="ts">
	// Ref: japaconnect-engineering/sections/34-media.md (user media library)
	// Grid of the user's uploaded media with filter, search, pagination, and
	// per-item actions (edit alt_text/caption, delete, copy URL). All data
	// comes from /me/media, PATCH /media/{id}, DELETE /media/{id} — Rule 16.
	import { t } from '$lib/i18n';
	import { untrack } from 'svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import PaginationControls from '$lib/components/ui/PaginationControls.svelte';
	import SelectField from '$lib/components/ui/SelectField.svelte';
	import { Image as ImageIcon, File, Search, Loader2, Pencil, Trash2, Copy, Check, X } from 'lucide-svelte';
	import type { MediaAsset } from '$lib/types';
	import { listMyMedia, updateMedia, deleteMedia } from '$lib/api/create';

	let { data } = $props();

	const PAGE_SIZE = 24;

	type TypeFilter = '' | 'image/' | 'video/' | 'audio/' | 'application/';
	let items = $state<MediaAsset[]>(untrack(() => data.media?.items ?? []));
	let total = $state<number>(untrack(() => data.media?.total ?? 0));
	let totalPages = $state<number>(untrack(() => data.media?.total_pages ?? 1));
	let page = $state(1);
	let typeFilter = $state<TypeFilter>('');
	let search = $state('');
	let loading = $state(false);
	let error = $state<string | null>(null);

	let searchTimer: ReturnType<typeof setTimeout> | null = null;

	// Edit modal state
	let showEdit = $state(false);
	let editTarget = $state<MediaAsset | null>(null);
	let editAltText = $state('');
	let editCaption = $state('');
	let editSaving = $state(false);
	let editError = $state<string | null>(null);

	// Delete confirm state
	let showDelete = $state(false);
	let deleteTarget = $state<MediaAsset | null>(null);
	let deleteSaving = $state(false);
	let deleteError = $state<string | null>(null);

	// Copy-URL feedback
	let copiedId = $state<string | null>(null);

	const typeOptions = [
		{ value: '', label: t('media.library.all_types') },
		{ value: 'image/', label: t('media.library.type_images') },
		{ value: 'video/', label: t('media.library.type_videos') },
		{ value: 'audio/', label: t('media.library.type_audio') },
		{ value: 'application/', label: t('media.library.type_documents') }
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

	async function fetchPage() {
		loading = true;
		error = null;
		const offset = (page - 1) * PAGE_SIZE;
		const res = await listMyMedia({
			type: typeFilter || undefined,
			q: search || undefined,
			page_size: PAGE_SIZE,
			page_offset: offset
		});
		if (res.error || !res.data) {
			error = res.error?.message ?? 'Failed to load media';
			items = [];
			total = 0;
			totalPages = 1;
		} else {
			const payload = res.data as unknown as { data?: MediaAsset[]; items?: MediaAsset[]; total?: number; total_pages?: number };
			items = payload.data ?? payload.items ?? (Array.isArray(res.data) ? (res.data as MediaAsset[]) : []);
			total = payload.total ?? items.length;
			totalPages = payload.total_pages ?? 1;
		}
		loading = false;
	}

	function onTypeChange(v: string) {
		typeFilter = v as TypeFilter;
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

	function openEdit(m: MediaAsset) {
		editTarget = m;
		editAltText = m.alt_text ?? '';
		editCaption = '';
		editError = null;
		editSaving = false;
		showEdit = true;
	}

	async function saveEdit() {
		if (!editTarget) return;
		editSaving = true;
		editError = null;
		const res = await updateMedia(editTarget.id, { alt_text: editAltText, caption: editCaption });
		if (res.error) {
			editError = res.error.message ?? t('media.library.edit_error');
		} else {
			showEdit = false;
			editTarget = null;
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

	async function confirmDelete() {
		if (!deleteTarget) return;
		deleteSaving = true;
		deleteError = null;
		const res = await deleteMedia(deleteTarget.id);
		if (res.error) {
			deleteError = res.error.message ?? t('media.library.delete_error');
		} else {
			showDelete = false;
			deleteTarget = null;
			fetchPage();
		}
		deleteSaving = false;
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
	<header>
		<h1 class="text-lg font-medium text-[var(--color-text)]">{t('dashboard.nav.media')}</h1>
		<p class="text-sm text-[var(--color-muted)]">{t('media.subtitle')}</p>
	</header>

	<!-- Filter + search bar -->
	<div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
		<div class="flex items-center gap-2">
			<Search size={16} class="text-[var(--color-muted)]" />
			<input
				type="text"
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition-colors focus:border-[var(--color-primary)] sm:w-64"
				placeholder={t('media.library.search_placeholder')}
				value={search}
				oninput={(e) => onSearchInput(e.currentTarget.value)}
			/>
		</div>
		<div class="w-full sm:w-48">
			<SelectField
				value={typeFilter}
				options={typeOptions}
				placeholder={t('media.library.filter_type')}
				onValueChange={onTypeChange}
			/>
		</div>
	</div>

	{#if error}
		<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3 text-sm text-[var(--destructive, #dc2626)]">
			{error}
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
	{:else}
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
						<!-- Hover action overlay -->
						<div class="absolute inset-0 flex items-center justify-center gap-2 bg-black/40 opacity-0 transition-opacity group-hover:opacity-100">
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
						<p class="mt-0.5 text-xs text-[var(--color-muted)]">{formatSize(m.file_size)}</p>
					</div>
				</li>
			{/each}
		</ul>

		<!-- Pagination -->
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

<!-- Edit modal -->
{#if showEdit && editTarget}
	<div class="fixed inset-0 z-[100] flex items-center justify-center bg-black/50 backdrop-blur-sm" role="dialog" aria-modal="true" aria-label={t('media.library.edit_title')}>
		<div class="w-full max-w-md rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5 shadow-2xl">
			<div class="mb-4 flex items-center justify-between">
				<h2 class="text-base font-medium text-[var(--color-text)]">{t('media.library.edit_title')}</h2>
				<button type="button" class="text-[var(--color-muted)] hover:text-[var(--color-text)]" onclick={() => (showEdit = false)} aria-label="Close">
					<X size={18} />
				</button>
			</div>
			<div class="space-y-3">
				<div>
					<label for="edit-alt" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">{t('media.library.edit_alt')}</label>
					<input
						id="edit-alt"
						type="text"
						class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
						bind:value={editAltText}
					/>
				</div>
				<div>
					<label for="edit-caption" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">{t('media.library.edit_caption')}</label>
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
				<button type="button" class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]" onclick={() => (showEdit = false)}>
					{t('media.picker.cancel')}
				</button>
				<button type="button" class="rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-2 text-sm text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)] disabled:opacity-50" onclick={saveEdit} disabled={editSaving}>
					{#if editSaving}
						<Loader2 size={14} class="animate-spin" />
					{:else}
						{t('media.library.edit_save')}
					{/if}
				</button>
			</div>
		</div>
	</div>
{/if}

<!-- Delete confirm modal -->
{#if showDelete && deleteTarget}
	<div class="fixed inset-0 z-[100] flex items-center justify-center bg-black/50 backdrop-blur-sm" role="dialog" aria-modal="true" aria-label={t('media.library.delete_title')}>
		<div class="w-full max-w-md rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5 shadow-2xl">
			<h2 class="mb-2 text-base font-medium text-[var(--color-text)]">{t('media.library.delete_title')}</h2>
			<p class="mb-4 text-sm text-[var(--color-muted)]">{t('media.library.delete_confirm')}</p>
			{#if deleteError}
				<p class="mb-3 text-xs text-[var(--destructive, #dc2626)]">{deleteError}</p>
			{/if}
			<div class="flex justify-end gap-2">
				<button type="button" class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]" onclick={() => (showDelete = false)}>
					{t('media.picker.cancel')}
				</button>
				<button type="button" class="rounded-[var(--radius)] bg-[var(--destructive, #dc2626)] px-3 py-2 text-sm text-white hover:opacity-90 disabled:opacity-50" onclick={confirmDelete} disabled={deleteSaving}>
					{#if deleteSaving}
						<Loader2 size={14} class="animate-spin" />
					{:else}
						{t('media.library.delete')}
					{/if}
				</button>
			</div>
		</div>
	</div>
{/if}
