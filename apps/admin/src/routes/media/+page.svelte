<script lang="ts">
	// /media — migrated to the EntityPage scaffold.
	// Moderation entity: read-only list (no create from EntityPage) with state actions
	// (approve/reject/quarantine) and a page-local upload button + modal preserved from the legacy page.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { Card, CardContent } from '$lib/components/ui/card';
	import { mediaAdminApi, api, apiFetch, apiRaw, stockAdminApi, type StockPhoto } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, CheckCircle, XCircle, Shield, Trash2, Upload, Image, FileText, Film, Music, Search, Download, Loader2, Check } from 'lucide-svelte';
	import type { ActionItem } from '$lib/components/ui/data-table';

	type MediaItem = {
		id: string;
		filename: string;
		mime_type: string;
		file_size: number;
		alt_text?: string;
		status: string;
		flagged?: boolean;
		uploader?: string;
		uploader_username?: string;
		created_at: string;
		url?: string;
		user_group_id?: string;
		references?: number;
	};

	// Backend gates approve/reject/quarantine on media.quarantine.manage and hard-delete on media.delete_any.
	const canManageMedia = $derived(hasPermission('media.quarantine.manage') || hasPermission('media.delete_any'));
	const canDeleteMedia = $derived(hasPermission('media.delete_any'));

	// ── EntityPage controller + page error ─────────────────────────────────
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');

	// ── State-action confirmation modals (approve / reject / quarantine) ───
	// EntityPage's built-in confirm only covers the default delete; moderation state actions
	// keep page-local Modals, then call entityController?.reload() on success.
	let acting = $state(false);
	let actionTarget = $state<MediaItem | null>(null);
	let showApproveConfirm = $state(false);
	let showRejectConfirm = $state(false);
	let showQuarantineConfirm = $state(false);

	function openApprove(row: MediaItem) {
		actionTarget = row;
		showApproveConfirm = true;
	}
	function openReject(row: MediaItem) {
		actionTarget = row;
		showRejectConfirm = true;
	}
	function openQuarantine(row: MediaItem) {
		actionTarget = row;
		showQuarantineConfirm = true;
	}

	async function confirmApprove() {
		if (!actionTarget) return;
		acting = true;
		pageError = '';
		try {
			const result = await mediaAdminApi.approve(actionTarget.id);
			if (result.ok) {
				showApproveConfirm = false;
				actionTarget = null;
				entityController?.reload();
			} else {
				pageError = result.error ?? t('common.error', 'Failed to approve media');
			}
		} catch {
			pageError = t('common.error', 'Failed to approve media');
		} finally {
			acting = false;
		}
	}

	async function confirmReject() {
		if (!actionTarget) return;
		acting = true;
		pageError = '';
		try {
			const result = await mediaAdminApi.reject(actionTarget.id);
			if (result.ok) {
				showRejectConfirm = false;
				actionTarget = null;
				entityController?.reload();
			} else {
				pageError = result.error ?? t('common.error', 'Failed to reject media');
			}
		} catch {
			pageError = t('common.error', 'Failed to reject media');
		} finally {
			acting = false;
		}
	}

	async function confirmQuarantine() {
		if (!actionTarget) return;
		acting = true;
		pageError = '';
		try {
			const result = await mediaAdminApi.updateStatus(actionTarget.id, 'quarantined');
			if (result.ok) {
				showQuarantineConfirm = false;
				actionTarget = null;
				entityController?.reload();
			} else {
				pageError = result.error ?? t('common.error', 'Failed to quarantine media');
			}
		} catch {
			pageError = t('common.error', 'Failed to quarantine media');
		} finally {
			acting = false;
		}
	}

	// ── Bulk actions (approve / reject / delete) with in-flight guard ─────
	let bulkActing = $state(false);

	async function bulkApprove(ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0) return;
		if (bulkActing) return; // in-flight guard
		bulkActing = true;
		pageError = '';
		try {
			await Promise.all(idArr.map((id) => mediaAdminApi.approve(id)));
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'Failed to approve media');
		} finally {
			bulkActing = false;
		}
	}

	async function bulkReject(ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0) return;
		if (bulkActing) return; // in-flight guard
		bulkActing = true;
		pageError = '';
		try {
			await Promise.all(idArr.map((id) => mediaAdminApi.reject(id)));
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'Failed to reject media');
		} finally {
			bulkActing = false;
		}
	}

	async function bulkDelete(ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0) return;
		if (bulkActing) return; // in-flight guard
		bulkActing = true;
		pageError = '';
		try {
			await Promise.all(idArr.map((id) => mediaAdminApi.delete(id)));
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'Failed to delete media');
		} finally {
			bulkActing = false;
		}
	}

	// ── Row actions: custom actions replace defaults, so View/Delete are included explicitly ──
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row, ctx) => ctx?.openView(_row) }
		];
		if (canManageMedia) {
			acts.push({
				label: t('app.media.action_approve', 'Approve'),
				icon: CheckCircle,
				onclick: openApprove,
				show: (row: MediaItem) => row.status === 'pending' || !!row.flagged
			});
			acts.push({
				label: t('app.media.action_reject', 'Reject'),
				icon: XCircle,
				variant: 'destructive',
				onclick: openReject,
				show: (row: MediaItem) => row.status === 'pending' || !!row.flagged
			});
			acts.push({
				label: t('app.media.action_quarantine', 'Quarantine'),
				icon: Shield,
				variant: 'destructive',
				onclick: openQuarantine,
				show: (row: MediaItem) => row.status !== 'quarantined'
			});
		}
		if (canDeleteMedia) {
			acts.push({
				label: t('common.delete', 'Delete'),
				icon: Trash2,
				variant: 'destructive',
				onclick: (_row, ctx) => ctx?.openDelete(_row)
			});
		}
		return acts;
	});

	// `fields` is required by EntityPage for edit-form seeding; this page is read-only
	// (no create/edit slide-over), so a minimal metadata field set is declared.
	const fields: FieldDef[] = [
		{ key: 'alt_text', label: t('app.media.field_alt_text', 'Alt Text'), type: 'text' },
		{ key: 'status', label: t('app.media.field_status', 'Status'), type: 'text' }
	];

	function formatFileSize(bytes: number): string {
		if (!bytes || bytes === 0) return '0 Bytes';
		const k = 1024;
		const sizes = ['Bytes', 'KB', 'MB', 'GB'];
		const i = Math.floor(Math.log(bytes) / Math.log(k));
		return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
	}

	function getIcon(mimeType: string) {
		if (mimeType?.startsWith('image/')) return Image;
		if (mimeType?.startsWith('video/')) return Film;
		if (mimeType?.startsWith('audio/')) return Music;
		return FileText;
	}

	const config = $derived<EntityPageConfig>({
		title: t('app.media.page_title', 'Media Library'),
		description: t('app.media.page_desc', 'Manage uploaded files and media'),
		readonly: true,
		deletePermission: 'media.delete_any',
		pageSize: 25,
		pageSizeOptions: [10, 25, 50, 100],
		headerActions: canManageMedia
			? [
					{
						label: t('app.media.upload', 'Upload'),
						icon: Upload,
						variant: 'default',
						onclick: openUploadModal
					}
				]
			: undefined,
		columns: [
			{
				key: 'filename',
				label: t('app.media.col_file', 'File'),
				sortable: true,
				componentSnippet: PreviewCell
			},
			{
				key: 'mime_type',
				label: t('app.media.col_type', 'Type'),
				sortable: true,
				render: (v: string) =>
					`<span class="inline-flex items-center rounded-full bg-surface px-2.5 py-0.5 text-xs font-medium text-foreground">${v || 'unknown'}</span>`
			},
			{
				key: 'uploader',
				label: t('app.media.col_uploader', 'Uploader'),
				sortable: true,
				render: (_v, row) =>
					row.uploader_username || row.uploader
						? `<span class="text-sm text-foreground">${row.uploader_username || row.uploader}</span>`
						: `<span style="color: var(--color-muted)">—</span>`
			},
			{
				key: 'status',
				label: t('app.media.col_status', 'Status'),
				sortable: true,
				render: (v: string) => {
					const variants: Record<string, string> = {
						approved: 'bg-success/15 text-success',
						pending: 'bg-warning/15 text-warning',
						quarantined: 'bg-danger/15 text-danger',
						rejected: 'bg-surface text-foreground'
					};
					return `<span class="inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${variants[v] || 'bg-surface text-foreground'}">${v || 'pending'}</span>`;
				}
			},
			{
				key: 'flagged',
				label: t('app.media.col_flagged', 'Flagged'),
				sortable: true,
				componentSnippet: FlaggedCell
			},
			{
				key: 'created_at',
				label: t('app.media.col_uploaded', 'Uploaded'),
				sortable: true,
				render: (v: string) =>
					new Date(v).toLocaleDateString('en-US', { year: 'numeric', month: 'short', day: 'numeric' })
			}
		],
		filters: [
			{
				key: 'mime_type',
				label: t('app.media.filter_file_type', 'File Type'),
				options: [
					{ value: 'image/', label: t('app.media.type_images', 'Images') },
					{ value: 'video/', label: t('app.media.type_videos', 'Videos') },
					{ value: 'audio/', label: t('app.media.type_audio', 'Audio') },
					{ value: 'application/', label: t('app.media.type_documents', 'Documents') }
				]
			},
			{
				key: 'status',
				label: t('app.media.filter_status', 'Status'),
				options: [
					{ value: 'approved', label: t('app.media.status_approved', 'Approved') },
					{ value: 'pending', label: t('app.media.status_pending', 'Pending') },
					{ value: 'quarantined', label: t('app.media.status_quarantined', 'Quarantined') }
				]
			}
		],
		actions: rowActions,
		bulkActions: canManageMedia
			? [
					{ label: t('app.media.action_approve', 'Approve'), icon: CheckCircle, onclick: bulkApprove },
					{ label: t('app.media.action_reject', 'Reject'), icon: XCircle, variant: 'destructive', onclick: bulkReject },
					{ label: t('common.deleteSelected', 'Delete Selected'), icon: Trash2, variant: 'destructive', onclick: bulkDelete }
				]
			: undefined,
		fields,
		defaultViewMode: 'grid',
		gridSnippet: MediaGridCard,
		remove: async (id) => {
			const res = await mediaAdminApi.delete(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		load: async ({ page, pageSize, q, filter }) => {
			const params = new URLSearchParams();
			params.set('page_size', String(pageSize));
			params.set('page_offset', String(page * pageSize));
			if (q) params.set('q', q);
			if (filter.status) params.set('status', filter.status);
			if (filter.mime_type) params.set('type', filter.mime_type);
			// Use apiRaw to access top-level total/total_pages alongside data.
			const result = await apiRaw<{ data?: MediaItem[]; items?: MediaItem[]; total?: number; total_pages?: number }>(`/admin/media?${params.toString()}`);
			if (result.ok) {
				const d = result.data;
				const items = d.data ?? d.items ?? [];
				return {
					data: items,
					total: d.total ?? items.length,
					totalPages: d.total_pages ?? 1
				};
			}
			throw new Error(result.error ?? t('app.media.err_load', 'Failed to load media'));
		},
		viewSnippet: MediaView
	});

	// ── Page-local upload flow (preserved from legacy) ────────────────────
	let showUploadModal = $state(false);
	let uploadFiles = $state<File[]>([]);
	let uploadUserGroupId = $state('');
	let uploadAltText = $state('');
	let uploadSubmitting = $state(false);
	let uploadProgress = $state<{ current: number; total: number; filename: string } | null>(null);
	let userGroups = $state<Array<{ id: string; name: string; slug: string }>>([]);

	// Upload modal sub-tab: "upload" (file picker) or "stock" (provider search)
	let uploadModalTab = $state<'upload' | 'stock'>('upload');

	// Stock search state (inside upload modal)
	let stockModalProviders = $state<{ name: string; display_name: string }[]>([]);
	let stockModalProvidersLoaded = $state(false);
	let stockModalQuery = $state('');
	let stockModalResults = $state<StockPhoto[]>([]);
	let stockModalLoading = $state(false);
	let stockModalHasMore = $state(false);
	let stockModalPage = $state(1);
	let stockModalProviderFilter = $state('');
	let stockModalImporting = $state<Set<string>>(new Set());
	let stockModalImported = $state<Set<string>>(new Set());
	let stockModalError = $state('');
	let stockModalTimer: ReturnType<typeof setTimeout> | undefined;

	async function loadUserGroups() {
		try {
			const result = await api<{ groups: Array<{ id: string; name: string; slug: string }>; total: number }>('/admin/user-groups');
			if (result.ok) {
				userGroups = result.data?.groups ?? [];
			}
		} catch {
			// keep defaults
		}
	}

	function openUploadModal() {
		uploadFiles = [];
		uploadUserGroupId = '';
		uploadAltText = '';
		uploadProgress = null;
		uploadModalTab = 'upload';
		stockModalQuery = '';
		stockModalResults = [];
		stockModalError = '';
		stockModalImported = new Set();
		stockModalImporting = new Set();
		showUploadModal = true;
		loadUserGroups();
		loadStockModalProviders();
	}

	// ── Stock search inside upload modal ──────────────────────────────────

	async function loadStockModalProviders() {
		if (stockModalProvidersLoaded) return;
		const result = await stockAdminApi.listAvailableProviders();
		if (result.ok) {
			stockModalProviders = result.data.providers ?? [];
		}
		stockModalProvidersLoaded = true;
	}

	function runStockModalSearch(q: string) {
		stockModalQuery = q;
		stockModalPage = 1;
		if (stockModalTimer) clearTimeout(stockModalTimer);
		if (!q.trim()) {
			stockModalResults = [];
			stockModalHasMore = false;
			return;
		}
		stockModalTimer = setTimeout(async () => {
			stockModalLoading = true;
			stockModalError = '';
			const result = await stockAdminApi.search({
				q: q.trim(),
				provider: stockModalProviderFilter || undefined,
				page: stockModalPage,
				per_page: 20
			});
			if (result.ok) {
				stockModalResults = result.data.photos ?? [];
				stockModalHasMore = result.data.has_next_page ?? false;
			} else {
				stockModalError = result.error ?? 'Search failed';
				stockModalResults = [];
				stockModalHasMore = false;
			}
			stockModalLoading = false;
		}, 350);
	}

	async function loadMoreStockModal() {
		stockModalPage++;
		stockModalLoading = true;
		const result = await stockAdminApi.search({
			q: stockModalQuery.trim(),
			provider: stockModalProviderFilter || undefined,
			page: stockModalPage,
			per_page: 20
		});
		if (result.ok) {
			stockModalResults = [...stockModalResults, ...(result.data.photos ?? [])];
			stockModalHasMore = result.data.has_next_page ?? false;
		}
		stockModalLoading = false;
	}

	function setStockModalProvider(name: string) {
		stockModalProviderFilter = name;
		if (stockModalQuery.trim()) runStockModalSearch(stockModalQuery);
	}

	async function importStockModalPhoto(photo: StockPhoto) {
		const pkey = `${photo.provider}:${photo.id}`;
		stockModalImporting = new Set([...stockModalImporting, pkey]);
		const result = await stockAdminApi.import(photo.provider, photo.id);
		stockModalImporting = new Set([...stockModalImporting].filter((k) => k !== pkey));
		if (result.ok) {
			stockModalImported = new Set([...stockModalImported, pkey]);
			entityController?.reload();
		} else {
			stockModalError = result.error ?? 'Import failed';
		}
	}

	async function handleUpload() {
		if (!uploadFiles.length) return;
		uploadSubmitting = true;
		pageError = '';
		const total = uploadFiles.length;
		let failed = 0;
		try {
			for (let i = 0; i < total; i++) {
				const file = uploadFiles[i];
				uploadProgress = { current: i + 1, total, filename: file.name };

				const formData = new FormData();
				formData.append('file', file);

				const result = await apiFetch<{ media_id: string; url: string }>('/admin/media/upload', {
					method: 'POST',
					body: formData
				});

				if (result.error) {
					failed++;
					continue;
				}

				const mediaId = result.data?.media_id;
				if (uploadUserGroupId && mediaId) {
					await api(`/admin/media/${mediaId}`, { method: 'PATCH', body: { user_group_id: uploadUserGroupId } });
				}
				if (uploadAltText && mediaId) {
					await api(`/admin/media/${mediaId}`, { method: 'PATCH', body: { alt_text: uploadAltText } });
				}
			}

			if (failed === total) {
				pageError = t('app.media.err_upload', 'Failed to upload media');
			}
			showUploadModal = false;
			entityController?.reload();
		} catch {
			pageError = t('app.media.err_upload', 'Failed to upload media');
		} finally {
			uploadSubmitting = false;
			uploadProgress = null;
		}
	}
</script>

<svelte:head>
	<title>{t('app.media.page_title', 'Media Library')} — JapaConnect Admin</title>
</svelte:head>

{#if pageError}
	<div
		class="mb-4 rounded-lg p-3 text-sm"
		style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
	>
		{pageError}
	</div>
{/if}

<EntityPage {config} bind:controller={entityController} />

<!-- ── State-action confirmation modals ─────────────────────────────────── -->

<!-- Approve -->
<Modal bind:open={showApproveConfirm} title={t('app.media.modal_approve_title', 'Approve Media')}>
	{#if actionTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('app.media.action_approve', 'Approve')} <strong>{actionTarget.filename}</strong>?
		</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showApproveConfirm = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button variant="default" onclick={confirmApprove} disabled={acting}>
			{acting ? t('common.saving', 'Saving...') : t('app.media.action_approve', 'Approve')}
		</Button>
	</div>
</Modal>

<!-- Reject -->
<Modal bind:open={showRejectConfirm} title={t('app.media.modal_reject_title', 'Reject Media')}>
	{#if actionTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('app.media.action_reject', 'Reject')} <strong>{actionTarget.filename}</strong>?
		</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showRejectConfirm = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={confirmReject} disabled={acting}>
			{acting ? t('common.saving', 'Saving...') : t('app.media.action_reject', 'Reject')}
		</Button>
	</div>
</Modal>

<!-- Quarantine -->
<Modal bind:open={showQuarantineConfirm} title={t('app.media.modal_quarantine_title', 'Quarantine Media')}>
	{#if actionTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('app.media.action_quarantine', 'Quarantine')} <strong>{actionTarget.filename}</strong>?
		</p>
		<p class="mt-2 text-sm" style="color: var(--color-danger)">
			{t('app.media.quarantine_warning', 'This media will be hidden pending review.')}
		</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showQuarantineConfirm = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={confirmQuarantine} disabled={acting}>
			{acting ? t('common.saving', 'Saving...') : t('app.media.action_quarantine', 'Quarantine')}
		</Button>
	</div>
</Modal>

<!-- Upload Modal (page-local, preserved from legacy) -->
<Modal bind:open={showUploadModal} title={t('app.media.modal_upload_title', 'Upload Media')}>
	<!-- Sub-tab switcher: Upload files vs Stock photos -->
	<div class="mb-4 inline-flex h-10 items-center justify-center rounded-lg p-1" style="background-color: var(--color-surface); color: var(--color-muted)">
		<button
			type="button"
			class="inline-flex items-center justify-center whitespace-nowrap rounded-md px-3 py-1.5 text-sm font-medium transition-all"
			style={uploadModalTab === 'upload'
				? 'background-color: var(--color-surface); color: var(--color-foreground); box-shadow: 0 1px 2px rgba(0,0,0,0.05)'
				: 'color: var(--color-muted)'}
			onclick={() => (uploadModalTab = 'upload')}
		>
			<Upload class="mr-2 h-4 w-4" />
			Upload
		</button>
		<button
			type="button"
			class="inline-flex items-center justify-center whitespace-nowrap rounded-md px-3 py-1.5 text-sm font-medium transition-all"
			style={uploadModalTab === 'stock'
				? 'background-color: var(--color-surface); color: var(--color-foreground); box-shadow: 0 1px 2px rgba(0,0,0,0.05)'
				: 'color: var(--color-muted)'}
			onclick={() => (uploadModalTab = 'stock')}
		>
			<Search class="mr-2 h-4 w-4" />
			Stock Photos
		</button>
	</div>

	{#if uploadModalTab === 'upload'}
		<div class="space-y-4">
			<div>
				<label for="media-upload-files" class="mb-1 block text-sm font-medium text-foreground">{t('app.media.upload_files', 'Files')}</label>
				<input
					id="media-upload-files"
					type="file"
					multiple
					accept="image/*,video/*,audio/*,application/pdf,.doc,.docx"
					onchange={(e) => {
						const files = e.currentTarget.files;
						if (files) uploadFiles = [...uploadFiles, ...Array.from(files)];
					}}
					class="block w-full text-sm text-foreground file:mr-4 file:rounded-lg file:border-0 file:bg-primary file:px-4 file:py-2 file:text-sm file:font-semibold file:text-white hover:file:bg-primary/80"
				/>
			</div>
			{#if uploadFiles.length > 0}
				<div class="max-h-40 space-y-1 overflow-y-auto">
					{#each uploadFiles as file, i}
						<div class="flex items-center justify-between rounded-lg bg-surface px-3 py-1.5 text-sm">
							<span class="truncate text-foreground">{file.name}</span>
							<span class="ml-2 shrink-0 text-xs text-muted">{formatFileSize(file.size)}</span>
							<button
								type="button"
								class="ml-2 shrink-0 text-muted hover:text-danger"
								onclick={() => { uploadFiles = uploadFiles.filter((_, j) => j !== i); }}
							>
								✕
							</button>
						</div>
					{/each}
				</div>
			{/if}
			<div>
				<label for="media-upload-group" class="mb-1 block text-sm font-medium text-foreground">{t('app.media.upload_user_group', 'User Group (optional)')}</label>
				<select
					id="media-upload-group"
					bind:value={uploadUserGroupId}
					class="w-full rounded-lg border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary"
				>
					<option value="">{t('app.media.upload_universal', 'Universal (all users)')}</option>
					{#each userGroups as group}
						<option value={group.id}>{group.name}</option>
					{/each}
				</select>
			</div>
			<div>
				<label for="media-upload-alt" class="mb-1 block text-sm font-medium text-foreground">{t('app.media.upload_alt_text', 'Alt Text (optional)')}</label>
				<input
					id="media-upload-alt"
					type="text"
					bind:value={uploadAltText}
					placeholder={t('app.media.upload_alt_placeholder', 'Describe the media...')}
					class="w-full rounded-lg border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary"
				/>
			</div>
			{#if uploadProgress}
				<div class="text-sm text-muted">
					{t('app.media.upload_progress', 'Uploading {current} of {total}...').replace('{current}', String(uploadProgress.current)).replace('{total}', String(uploadProgress.total))}
					<span class="text-foreground-alt">— {uploadProgress.filename}</span>
				</div>
			{/if}
		</div>
		<div class="mt-4 flex justify-end gap-2">
			<Button variant="ghost" onclick={() => (showUploadModal = false)}>{t('common.cancel', 'Cancel')}</Button>
			<Button variant="default" onclick={handleUpload} disabled={!uploadFiles.length || uploadSubmitting}>
				{uploadSubmitting
					? t('app.media.uploading', 'Uploading...')
					: t('app.media.upload_n_files', 'Upload {n} files').replace('{n}', String(uploadFiles.length))}
			</Button>
		</div>
	{:else}
		<!-- Stock photo search + import -->
		<div class="space-y-3">
			{#if !stockModalProvidersLoaded}
				<div class="flex items-center justify-center py-8">
					<Loader2 class="h-5 w-5 animate-spin" style="color: var(--color-muted)" />
					<span class="ml-2 text-sm" style="color: var(--color-muted)">Loading providers...</span>
				</div>
			{:else if stockModalProviders.length === 0}
				<div class="py-8 text-center text-sm" style="color: var(--color-muted)">
					No stock providers available. Enable providers in the Stock Providers admin tab.
				</div>
			{:else}
				<!-- Search bar -->
				<div class="relative">
					<Search class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2" style="color: var(--color-muted)" />
					<input
						type="text"
						placeholder="Search stock photos..."
						class="w-full rounded-lg border py-2 pl-9 pr-3 text-sm"
						style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
						value={stockModalQuery}
						oninput={(e) => runStockModalSearch(e.currentTarget.value)}
					/>
				</div>

				<!-- Provider filter chips -->
				<div class="flex flex-wrap gap-2">
					<button
						type="button"
						class="rounded-full px-3 py-1 text-xs font-medium transition-colors"
						style={stockModalProviderFilter === ''
							? 'background-color: var(--color-primary); color: var(--color-primary-contrast, white)'
							: 'background-color: var(--color-surface); color: var(--color-muted); border: 1px solid var(--color-border)'}
						onclick={() => setStockModalProvider('')}
					>All</button>
					{#each stockModalProviders as p (p.name)}
						<button
							type="button"
							class="rounded-full px-3 py-1 text-xs font-medium transition-colors"
							style={stockModalProviderFilter === p.name
								? 'background-color: var(--color-primary); color: var(--color-primary-contrast, white)'
								: 'background-color: var(--color-surface); color: var(--color-muted); border: 1px solid var(--color-border)'}
							onclick={() => setStockModalProvider(p.name)}
						>{p.display_name}</button>
					{/each}
				</div>

				{#if stockModalError}
					<div class="rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-danger); color: var(--color-danger)">
						{stockModalError}
					</div>
				{/if}

				<!-- Results -->
				<div class="max-h-[400px] overflow-y-auto">
					{#if stockModalLoading && stockModalResults.length === 0}
						<div class="flex items-center justify-center py-8">
							<Loader2 class="h-5 w-5 animate-spin" style="color: var(--color-muted)" />
							<span class="ml-2 text-sm" style="color: var(--color-muted)">Searching...</span>
						</div>
					{:else if stockModalResults.length === 0 && stockModalQuery}
						<div class="py-8 text-center text-sm" style="color: var(--color-muted)">
							No results found for "{stockModalQuery}"
						</div>
					{:else if stockModalResults.length === 0}
						<div class="py-8 text-center text-sm" style="color: var(--color-muted)">
							Search for photos to import into your media library.
						</div>
					{:else}
						<div class="grid grid-cols-2 gap-3 sm:grid-cols-3">
							{#each stockModalResults as photo (photo.provider + ':' + photo.id)}
								{@const pkey = photo.provider + ':' + photo.id}
								<div class="group relative overflow-hidden rounded-lg border" style="border-color: var(--color-border)">
									<img src={photo.thumbnail_url} alt={photo.description ?? ''} loading="lazy" class="aspect-square w-full object-cover" />
									<div class="absolute inset-0 flex flex-col justify-between bg-gradient-to-t from-black/60 via-transparent to-transparent p-2">
										<span class="truncate text-xs text-white/90">{photo.photographer}</span>
										<div class="flex items-center justify-between">
											{#if photo.license && (photo.license === 'cc-by' || photo.license === 'cc-by-sa' || photo.license === 'cc0')}
												<span class="rounded bg-white/20 px-1.5 py-0.5 text-[10px] font-medium text-white">{photo.license.toUpperCase()}</span>
											{:else}
												<span></span>
											{/if}
											{#if stockModalImported.has(pkey)}
												<span class="inline-flex items-center gap-1 rounded bg-green-500/80 px-2 py-1 text-xs text-white">
													<Check class="h-3 w-3" /> Imported
												</span>
											{:else if stockModalImporting.has(pkey)}
												<Loader2 class="h-4 w-4 animate-spin text-white" />
											{:else}
												<button
													type="button"
													class="inline-flex items-center gap-1 rounded bg-primary px-2 py-1 text-xs text-white hover:bg-primary/90"
													onclick={() => importStockModalPhoto(photo)}
												>
													<Download class="h-3 w-3" /> Import
												</button>
											{/if}
										</div>
									</div>
								</div>
							{/each}
						</div>
						{#if stockModalHasMore}
							<div class="mt-3 flex justify-center">
								<Button variant="ghost" size="sm" onclick={loadMoreStockModal} disabled={stockModalLoading}>
									{stockModalLoading ? 'Loading...' : 'Load more'}
								</Button>
							</div>
						{/if}
					{/if}
				</div>
			{/if}
		</div>
		<div class="mt-4 flex justify-end">
			<Button variant="ghost" onclick={() => (showUploadModal = false)}>Close</Button>
		</div>
	{/if}
</Modal>

<!-- ── Snippets ─────────────────────────────────────────────────────────── -->

{#snippet PreviewCell({ row }: { value: any; row: MediaItem })}
	<div class="flex items-center gap-3">
		<div class="flex h-10 w-10 shrink-0 items-center justify-center overflow-hidden rounded bg-surface">
			{#if row.mime_type?.startsWith('image/') && row.url}
				<img src={row.url} alt={row.alt_text || row.filename} class="h-full w-full object-cover" />
			{:else}
				{@const Icon = getIcon(row.mime_type)}
				<Icon class="h-5 w-5 text-muted" />
			{/if}
		</div>
		<div class="min-w-0">
			<p class="truncate font-medium text-foreground">{row.filename}</p>
			<p class="truncate text-xs text-muted">{row.alt_text || ''}</p>
		</div>
	</div>
{/snippet}

{#snippet FlaggedCell({ value }: { value: any; row: MediaItem })}
	{#if value}
		<span class="inline-flex items-center rounded-full bg-danger/15 px-2 py-0.5 text-xs font-medium text-danger">
			{t('app.media.flagged_yes', 'Flagged')}
		</span>
	{:else}
		<span style="color: var(--color-muted)">—</span>
	{/if}
{/snippet}

<!-- Grid/card view: image thumbnail + hover action overlay + filename/status footer. -->
{#snippet MediaGridCard({ row }: { row: MediaItem })}
	<Card class="group relative cursor-pointer overflow-hidden transition-shadow hover:shadow-md">
		<div class="relative flex aspect-square items-center justify-center bg-surface">
			{#if row.mime_type?.startsWith('image/') && row.url}
				<img src={row.url} alt={row.alt_text || row.filename} class="h-full w-full object-cover" />
			{:else}
				{@const Icon = getIcon(row.mime_type)}
				<Icon class="h-8 w-8 text-muted" />
			{/if}
			<!-- Hover action overlay -->
			<div class="absolute inset-0 flex items-center justify-center gap-2 bg-black/50 opacity-0 transition-opacity group-hover:opacity-100">
				<Button variant="secondary" size="sm" class="h-8 w-8 p-0" onclick={() => entityController?.openView(row)}>
					<Eye class="h-4 w-4" />
				</Button>
				{#if canManageMedia}
					{#if row.status === 'quarantined' || row.status === 'pending'}
						<Button variant="secondary" size="sm" class="h-8 w-8 p-0" onclick={() => openApprove(row)} style="color: var(--color-success)">
							<CheckCircle class="h-4 w-4" />
						</Button>
						<Button variant="secondary" size="sm" class="h-8 w-8 p-0" onclick={() => openReject(row)} style="color: var(--color-warning)">
							<XCircle class="h-4 w-4" />
						</Button>
					{/if}
					<Button variant="secondary" size="sm" class="h-8 w-8 p-0" onclick={() => entityController?.openDelete(row)} style="color: var(--color-danger)">
						<Trash2 class="h-4 w-4" />
					</Button>
				{/if}
			</div>
		</div>
		<CardContent class="p-3">
			<p class="truncate text-sm font-medium text-foreground">{row.filename}</p>
			<div class="mt-1 flex items-center justify-between">
				<p class="text-xs text-muted">{formatFileSize(row.file_size)}</p>
				<span class="inline-flex items-center rounded-full px-1.5 py-0.5 text-[10px] font-medium {row.status === 'approved' ? 'bg-success/15 text-success' : row.status === 'pending' ? 'bg-warning/15 text-warning' : row.status === 'quarantined' ? 'bg-danger/15 text-danger' : 'bg-surface text-foreground-alt'}">
					{row.status || 'pending'}
				</span>
			</div>
		</CardContent>
	</Card>
{/snippet}

<!-- View slide-over body (mirrors the legacy view modal) -->
{#snippet MediaView({ row }: { row: MediaItem })}
	<div class="space-y-4">
		<div class="flex aspect-video items-center justify-center rounded-lg" style="background: var(--color-surface)">
			{#if row.mime_type?.startsWith('image/') && row.url}
				<img src={row.url} alt={row.alt_text || row.filename} class="max-h-full rounded-lg object-contain" />
			{:else}
				{@const Icon = getIcon(row.mime_type)}
				<Icon class="h-12 w-12" style="color: var(--color-muted)" />
			{/if}
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.media.field_filename', 'Filename')}</p>
				<p class="mt-1 break-all text-sm font-medium" style="color: var(--color-foreground)">{row.filename}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.media.field_type', 'Type')}</p>
				<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.mime_type}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.media.field_size', 'Size')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{formatFileSize(row.file_size)}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.media.col_uploader', 'Uploader')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.uploader_username || row.uploader || '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.media.field_status', 'Status')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.media.col_flagged', 'Flagged')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
					{row.flagged ? t('app.media.flagged_yes', 'Flagged') : t('app.media.flagged_no', 'Not Flagged')}
				</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.media.col_uploaded', 'Uploaded')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{new Date(row.created_at).toLocaleString()}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.media.col_references', 'References')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.references ?? '—'}</p>
			</div>
			<div class="col-span-2">
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.media.field_alt_text', 'Alt Text')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.alt_text || '—'}</p>
			</div>
			{#if row.url}
				<div class="col-span-2">
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.media.field_url', 'URL')}</p>
					<p class="mt-1 break-all text-sm font-mono" style="color: var(--color-foreground)">{row.url}</p>
				</div>
			{/if}
		</div>
	</div>
{/snippet}
