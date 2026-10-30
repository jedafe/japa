<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	// /media — migrated to the EntityPage scaffold.
	// Moderation entity: read-only list (no create from EntityPage) with state actions
	// (approve/reject/quarantine) and a page-local upload button + modal preserved from the legacy page.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { Card, CardContent } from '$lib/components/ui/card';
	import { mediaAdminApi, api, apiFetch, apiRaw, stockAdminApi, adminCatalogApi, type StockPhoto, type StockProviderAdmin, type CatalogOption } from '$lib/api';
	import { t, locale } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, CheckCircle, XCircle, Shield, Trash2, Upload, Image as ImageIcon, FileText, Film, Music, Search, Download, Loader2, Check, RefreshCw, ZoomIn, ZoomOut, Maximize, Pencil, Save, X } from 'lucide-svelte';
	import type { ActionItem } from '$lib/components/ui/data-table';
	import { initTabParam, bindTabParam } from '$lib/utils/url-sync.svelte';
	import { convertImageToModernFormat } from '$lib/utils/imageConvert';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';

	type MediaItem = {
		id: string;
		filename: string;
		mime_type: string;
		file_size: number;
		alt_text?: string;
		caption?: string;
		width?: number;
		height?: number;
		status: string;
		flagged?: boolean;
		uploader?: string;
		uploader_username?: string;
		deleted_at?: string;
		created_at: string;
		url?: string;
		user_group_id?: string;
		references?: number;
	};

	// Backend gates approve/reject/quarantine on media.quarantine.manage and hard-delete on media.delete_any.
	const canManageMedia = $derived(hasPermission('media.quarantine.manage') || hasPermission('media.delete_any'));
	const canDeleteMedia = $derived(hasPermission('media.delete_any'));
	const canEditMedia = $derived(hasPermission('media.edit_any'));
	// Rule 11 amendment: permanent delete requires the dedicated node.
	const canHardDelete = $derived(hasPermission('admin.hard_delete'));

	// ── EntityPage controller + page error ─────────────────────────────────
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');

	let mediaMimeTypeOptions = $state<CatalogOption[]>([
		{ value: 'image/', label: t('app.media.type_images', 'Images') },
		{ value: 'video/', label: t('app.media.type_videos', 'Videos') },
		{ value: 'audio/', label: t('app.media.type_audio', 'Audio') },
		{ value: 'application/', label: t('app.media.type_documents', 'Documents') }
	]);

	let mediaStatusOptions = $state<CatalogOption[]>([
		{ value: 'approved', label: t('app.media.status_approved', 'Approved') },
		{ value: 'pending', label: t('app.media.status_pending', 'Pending') },
		{ value: 'quarantined', label: t('app.media.status_quarantined', 'Quarantined') },
		{ value: 'rejected', label: t('app.media.status_rejected', 'Rejected') }
	]);

	$effect(() => {
		void (async () => {
			const [mimeRes, statusRes] = await Promise.all([
				adminCatalogApi.get('media.mime_type'),
				adminCatalogApi.get('media.status')
			]);
			if (mimeRes.ok && mimeRes.data?.length) mediaMimeTypeOptions = mimeRes.data;
			if (statusRes.ok && statusRes.data?.length) mediaStatusOptions = statusRes.data;
		})();
	});

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

	// Row map so bulk actions can resolve ids → rows after EntityPage clears
	// the selection.
	const mediaRowMap = new Map<string, MediaItem>();
	const mediaRowsFor = (ids: Set<string>): MediaItem[] =>
		Array.from(ids)
			.map((id) => mediaRowMap.get(id))
			.filter((r): r is MediaItem => !!r);

	// ── Force delete (admin.hard_delete) — single + bulk share one modal ──
	let forceTargets = $state<MediaItem[]>([]);
	let showForceModal = $state(false);

	function openForceDelete(row: MediaItem) {
		forceTargets = [row];
		showForceModal = true;
	}

	function openBulkForceDelete(ids: Set<string>) {
		const rows = mediaRowsFor(ids).filter((r) => !!r.deleted_at);
		if (rows.length === 0) return;
		forceTargets = rows;
		showForceModal = true;
	}

	async function confirmForceDelete() {
		if (bulkActing || forceTargets.length === 0) return;
		bulkActing = true;
		pageError = '';
		try {
			const targets = forceTargets;
			let failed = 0;
			let lastErr = '';
			for (const row of targets) {
				const res = await mediaAdminApi.hardDelete(row.id);
				if (!res.ok) {
					failed++;
					lastErr = res.error ?? '';
				}
			}
			showForceModal = false;
			forceTargets = [];
			if (failed > 0) {
				pageError =
					t('common.bulk_partial', '{failed} of {n} items could not be completed')
						.replace('{failed}', String(failed))
						.replace('{n}', String(targets.length)) + (lastErr ? ` — ${lastErr}` : '');
			}
			entityController?.reload();
		} finally {
			bulkActing = false;
		}
	}

	// ── Row actions: custom actions replace defaults, so View/Delete are included explicitly ──
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row, ctx) => ctx?.openView(_row) }
		];
		if (canEditMedia) {
			acts.push({
				label: t('common.edit', 'Edit'),
				icon: Pencil,
				onclick: (row: MediaItem) => openEditModal(row),
				show: (row: MediaItem) => !row.deleted_at
			});
		}
		if (canManageMedia) {
			acts.push({
				label: t('app.media.action_approve', 'Approve'),
				icon: CheckCircle,
				onclick: openApprove,
				show: (row: MediaItem) => (row.status === 'pending' || !!row.flagged) && !row.deleted_at
			});
			acts.push({
				label: t('app.media.action_reject', 'Reject'),
				icon: XCircle,
				variant: 'destructive',
				onclick: openReject,
				show: (row: MediaItem) => (row.status === 'pending' || !!row.flagged) && !row.deleted_at
			});
			acts.push({
				label: t('app.media.action_quarantine', 'Quarantine'),
				icon: Shield,
				variant: 'destructive',
				onclick: openQuarantine,
				show: (row: MediaItem) => row.status !== 'quarantined' && !row.deleted_at
			});
		}
		if (canDeleteMedia) {
			acts.push({
				label: t('common.delete', 'Delete'),
				icon: Trash2,
				variant: 'destructive',
				onclick: (_row, ctx) => ctx?.openDelete(_row),
				show: (row: MediaItem) => !row.deleted_at
			});
		}
		if (canHardDelete) {
			// Permanent removal — only for already-soft-deleted rows.
			acts.push({
				label: t('common.force_delete', 'Force Delete'),
				icon: Trash2,
				variant: 'destructive',
				onclick: openForceDelete,
				show: (row: MediaItem) => !!row.deleted_at
			});
		}
		return acts;
	});

	// `fields` is required by EntityPage for edit-form seeding; this page is read-only
	// (no create/edit modal), so a minimal metadata field set is declared.
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
		if (mimeType?.startsWith('image/')) return ImageIcon;
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
				render: (v: string, row: MediaItem) => {
					if (row.deleted_at) {
						return `<span class="inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium bg-danger/15 text-danger">${t('common.filter_deleted', 'Deleted')}</span>`;
					}
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
					new Date(v).toLocaleDateString(locale.current, { year: 'numeric', month: 'short', day: 'numeric' })
			}
		],
		filters: [
			{
				key: 'mime_type',
				label: t('app.media.filter_file_type', 'File Type'),
				options: mediaMimeTypeOptions
			},
			{
				key: 'status',
				label: t('app.media.filter_status', 'Status'),
				options: mediaStatusOptions
			},
			{
				key: 'flagged',
				label: t('app.media.col_flagged', 'Flagged'),
				options: [
					{ value: 'true', label: t('app.media.flagged_yes', 'Flagged') },
					{ value: 'false', label: t('app.media.flagged_no', 'Not Flagged') }
				]
			},
			// Rows whose storage object is absent are hidden by default
			// (m000924 missing_at) — this filter surfaces them for audit.
			{
				key: 'missing',
				label: t('app.media.filter_missing', 'Missing Object'),
				options: [
					{ value: 'only', label: t('app.media.missing_only', 'Missing only') },
					{ value: 'all', label: t('app.media.missing_all', 'Include missing') }
				]
			},
			// Soft-deleted media is hidden by default — this filter surfaces it
			// so the Force Delete action has targets.
			{
				key: 'deleted',
				label: t('common.filter_deleted', 'Deleted'),
				options: [
					{ value: 'only', label: t('common.filter_deleted_only', 'Deleted only') },
					{ value: 'all', label: t('common.filter_deleted_all', 'Include deleted') }
				]
			}
		],
		actions: rowActions,
		bulkActions: [
			...(canManageMedia
				? [
						{ label: t('app.media.action_approve', 'Approve'), icon: CheckCircle, onclick: bulkApprove, disabled: (_ids: Set<string>, rows?: MediaItem[]) => (rows ?? []).some((r) => !!r.deleted_at) },
						{ label: t('app.media.action_reject', 'Reject'), icon: XCircle, variant: 'destructive' as const, onclick: bulkReject, disabled: (_ids: Set<string>, rows?: MediaItem[]) => (rows ?? []).some((r) => !!r.deleted_at) },
						{ label: t('common.deleteSelected', 'Delete Selected'), icon: Trash2, variant: 'destructive' as const, onclick: bulkDelete, disabled: (_ids: Set<string>, rows?: MediaItem[]) => (rows ?? []).some((r) => !!r.deleted_at) }
					]
				: []),
			...(canHardDelete
				? [
						{
							label: t('common.force_delete', 'Force Delete Selected'),
							icon: Trash2,
							variant: 'destructive' as const,
							onclick: openBulkForceDelete,
							disabled: (ids: Set<string>) =>
								bulkActing || !mediaRowsFor(ids).some((r) => !!r.deleted_at)
						}
					]
				: [])
		],
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
			if (filter.flagged) params.set('flagged', filter.flagged);
			if (filter.deleted) params.set('deleted', filter.deleted);
			if (filter.missing) params.set('missing', filter.missing);
			// Use apiRaw to access top-level total/total_pages alongside data.
			const result = await apiRaw<{ data?: MediaItem[]; items?: MediaItem[]; total?: number; total_pages?: number }>(`/admin/media?${params.toString()}`);
			if (result.ok) {
				const d = result.data;
				const items = (d.data ?? d.items ?? []) as MediaItem[];
				for (const r of items) mediaRowMap.set(r.id, r);
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

	// ── Edit metadata (alt_text + caption/title) ──────────────────────────
	let showEditModal = $state(false);
	let editTarget = $state<MediaItem | null>(null);
	let editAltText = $state('');
	let editCaption = $state('');
	let editSaving = $state(false);

	function openEditModal(row: MediaItem) {
		editTarget = row;
		editAltText = row.alt_text ?? '';
		editCaption = row.caption ?? '';
		editSaving = false;
		showEditModal = true;
	}

	async function saveEdit() {
		if (!editTarget) return;
		editSaving = true;
		pageError = '';
		try {
			const result = await mediaAdminApi.update(editTarget.id, {
				alt_text: editAltText,
				caption: editCaption
			});
			if (result.ok) {
				showEditModal = false;
				editTarget = null;
				entityController?.reload();
			} else {
				pageError = result.error ?? t('common.error', 'Failed to update media');
			}
		} catch {
			pageError = t('common.error', 'Failed to update media');
		} finally {
			editSaving = false;
		}
	}

	// ── Image viewer (fullscreen lightbox with zoom/pan) ──────────────────
	let showViewer = $state(false);
	let viewerItem = $state<MediaItem | null>(null);
	let viewerZoom = $state(1);
	let viewerPanX = $state(0);
	let viewerPanY = $state(0);
	let viewerPanning = $state(false);
	let viewerPanStart = $state({ x: 0, y: 0, panX: 0, panY: 0 });

	function openViewer(row: MediaItem) {
		viewerItem = row;
		viewerZoom = 1;
		viewerPanX = 0;
		viewerPanY = 0;
		showViewer = true;
	}

	function closeViewer() {
		showViewer = false;
		viewerItem = null;
	}

	function zoomIn() {
		viewerZoom = Math.min(viewerZoom * 1.25, 8);
	}
	function zoomOut() {
		viewerZoom = Math.max(viewerZoom / 1.25, 0.25);
		if (viewerZoom <= 1) {
			viewerPanX = 0;
			viewerPanY = 0;
		}
	}
	function resetZoom() {
		viewerZoom = 1;
		viewerPanX = 0;
		viewerPanY = 0;
	}

	function onViewerWheel(e: WheelEvent) {
		if (e.deltaY < 0) zoomIn();
		else zoomOut();
	}

	function onViewerPointerDown(e: PointerEvent) {
		if (viewerZoom <= 1) return;
		viewerPanning = true;
		viewerPanStart = { x: e.clientX, y: e.clientY, panX: viewerPanX, panY: viewerPanY };
	}
	function onViewerPointerMove(e: PointerEvent) {
		if (!viewerPanning) return;
		viewerPanX = viewerPanStart.panX + (e.clientX - viewerPanStart.x);
		viewerPanY = viewerPanStart.panY + (e.clientY - viewerPanStart.y);
	}
	function onViewerPointerUp() {
		viewerPanning = false;
	}

	// ── Image resizer (client-side canvas, preview + download) ────────────
	let showResizer = $state(false);
	let resizerItem = $state<MediaItem | null>(null);
	let resizerOriginal = $state<{ w: number; h: number } | null>(null);
	let resizerWidth = $state(0);
	let resizerHeight = $state(0);
	let resizerMaintainAspect = $state(true);
	let resizerQuality = $state(0.9);
	let resizerFormat = $state<'jpeg' | 'png' | 'webp'>('jpeg');
	let resizerPreviewUrl = $state('');
	let resizerError = $state('');
	let resizerCanvas = $state<HTMLCanvasElement | null>(null);

	function openResizer(row: MediaItem) {
		if (!row.mime_type?.startsWith('image/') || !row.url) return;
		resizerItem = row;
		resizerError = '';
		resizerPreviewUrl = '';
		resizerMaintainAspect = true;
		resizerQuality = 0.9;
		resizerFormat = 'jpeg';
		showResizer = true;
		// Load the image to get original dimensions
		const img = new Image();
		img.crossOrigin = 'anonymous';
		img.onload = () => {
			resizerOriginal = { w: img.naturalWidth, h: img.naturalHeight };
			resizerWidth = img.naturalWidth;
			resizerHeight = img.naturalHeight;
			renderResizerPreview(img);
		};
		img.onerror = () => {
			resizerError = 'Failed to load image for resizing';
		};
		img.src = row.url;
	}

	function onResizerWidthChange() {
		if (resizerMaintainAspect && resizerOriginal && resizerWidth > 0) {
			resizerHeight = Math.round((resizerWidth / resizerOriginal.w) * resizerOriginal.h);
		}
		renderResizerFromSource();
	}
	function onResizerHeightChange() {
		if (resizerMaintainAspect && resizerOriginal && resizerHeight > 0) {
			resizerWidth = Math.round((resizerHeight / resizerOriginal.h) * resizerOriginal.w);
		}
		renderResizerFromSource();
	}

	let resizerSourceImg: HTMLImageElement | null = null;

	function renderResizerFromSource() {
		if (!resizerSourceImg || !resizerItem) {
			// Re-load from URL
			if (!resizerItem) return;
			const img = new Image();
			img.crossOrigin = 'anonymous';
			img.onload = () => {
				resizerSourceImg = img;
				renderResizerPreview(img);
			};
			img.src = resizerItem.url ?? '';
			return;
		}
		renderResizerPreview(resizerSourceImg);
	}

	function renderResizerPreview(img: HTMLImageElement) {
		resizerSourceImg = img;
		const w = Math.max(1, resizerWidth);
		const h = Math.max(1, resizerHeight);
		const canvas = document.createElement('canvas');
		canvas.width = w;
		canvas.height = h;
		const ctx = canvas.getContext('2d');
		if (!ctx) {
			resizerError = 'Canvas not supported';
			return;
		}
		ctx.imageSmoothingEnabled = true;
		ctx.imageSmoothingQuality = 'high';
		ctx.drawImage(img, 0, 0, w, h);
		resizerCanvas = canvas;
		const mime = resizerFormat === 'png' ? 'image/png' : resizerFormat === 'webp' ? 'image/webp' : 'image/jpeg';
		resizerPreviewUrl = canvas.toDataURL(mime, resizerQuality);
	}

	function downloadResized() {
		if (!resizerCanvas || !resizerItem) return;
		const mime = resizerFormat === 'png' ? 'image/png' : resizerFormat === 'webp' ? 'image/webp' : 'image/jpeg';
		const ext = resizerFormat === 'png' ? 'png' : resizerFormat === 'webp' ? 'webp' : 'jpg';
		resizerCanvas.toBlob(
			(blob) => {
				if (!blob) return;
				const url = URL.createObjectURL(blob);
				const a = document.createElement('a');
				a.href = url;
				const baseName = resizerItem!.filename.replace(/\.[^/.]+$/, '');
				a.download = `${baseName}_${resizerWidth}x${resizerHeight}.${ext}`;
				a.click();
				URL.revokeObjectURL(url);
			},
			mime,
			resizerQuality
		);
	}

	function closeResizer() {
		showResizer = false;
		resizerItem = null;
		resizerOriginal = null;
		resizerSourceImg = null;
		resizerCanvas = null;
		resizerPreviewUrl = '';
		resizerError = '';
	}

	// ── Page-local upload flow (preserved from legacy) ────────────────────
	// Default user_group_id for admin uploads: the super_admin system group
	// (00000000-0000-0000-0000-000000000010, seeded in migration 000006).
	// Admin-uploaded media defaults to admin-only visibility instead of
	// universal, so an admin's uploads are not exposed to all users unless
	// the admin explicitly chooses "Universal (all users)".
	const SUPER_ADMIN_GROUP_ID = '00000000-0000-0000-0000-000000000010';
	let showUploadModal = $state(false);
	let uploadFiles = $state<File[]>([]);
	let uploadUserGroupId = $state(SUPER_ADMIN_GROUP_ID);
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
		uploadUserGroupId = SUPER_ADMIN_GROUP_ID;
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

	// Clear any pending stock-search debounce when the page unmounts.
	$effect(() => {
		return () => {
			if (stockModalTimer) clearTimeout(stockModalTimer);
		};
	});

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
				const file = await convertImageToModernFormat(uploadFiles[i]);
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

	// ── Stock Providers management tab ────────────────────────────────────
	const MEDIA_TABS = ['media', 'stock-providers'] as const;
	let activeTab = $state<'media' | 'stock-providers'>(initTabParam('media', { valid: MEDIA_TABS }));
	bindTabParam(() => activeTab, (v) => (activeTab = v), { valid: MEDIA_TABS, preserve: [] });
	let stockProviders = $state<StockProviderAdmin[]>([]);
	let stockProvidersLoading = $state(false);
	let stockProvidersError = $state('');
	let stockProvidersLoaded = $state(false);
	let updatingProvider = $state<string | null>(null);

	async function loadStockProviders() {
		stockProvidersLoading = true;
		stockProvidersError = '';
		try {
			const result = await stockAdminApi.listProviders();
			if (result.ok) {
				stockProviders = result.data?.providers ?? [];
				stockProvidersLoaded = true;
			} else {
				stockProvidersError = result.error ?? t('app.media.err_providers', 'Failed to load providers');
			}
		} catch (err) {
			stockProvidersError = err instanceof Error ? err.message : t('app.media.err_providers', 'Failed to load providers');
		}
		stockProvidersLoading = false;
	}

	async function toggleStockProvider(name: string, enabled: boolean) {
		updatingProvider = name;
		const result = await stockAdminApi.patchProvider(name, { enabled });
		if (result.ok) {
			await loadStockProviders();
		} else {
			stockProvidersError = result.error ?? t('app.media.err_provider_update', 'Failed to update provider');
		}
		updatingProvider = null;
	}

	async function updateProviderKey(name: string, apiKey: string) {
		if (!apiKey.trim()) return;
		updatingProvider = name;
		const result = await stockAdminApi.patchProvider(name, { api_key: apiKey.trim() });
		if (result.ok) {
			await loadStockProviders();
		} else {
			stockProvidersError = result.error ?? t('app.media.err_api_key', 'Failed to update API key');
		}
		updatingProvider = null;
	}

	$effect(() => {
		if (activeTab === 'stock-providers' && !stockProvidersLoaded) {
			loadStockProviders();
		}
	});
</script>

<svelte:head>
	<title>{t('app.media.page_title', 'Media Library')} — {siteName()} Admin</title>
</svelte:head>

{#if pageError}
	<div
		class="mb-4 rounded-lg p-3 text-sm"
		style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
	>
		{pageError}
	</div>
{/if}

<!-- Tab switcher: Media Library vs Stock Providers -->
<div class="mb-4 inline-flex h-10 items-center justify-center rounded-lg p-1" style="background-color: var(--color-surface); color: var(--color-muted)">
	<button
		type="button"
		class="inline-flex items-center justify-center whitespace-nowrap rounded-md px-3 py-1.5 text-sm font-medium transition-all"
		style={activeTab === 'media'
			? 'background-color: var(--color-surface); color: var(--color-foreground); box-shadow: 0 1px 2px rgba(0,0,0,0.05)'
			: 'color: var(--color-muted)'}
		onclick={() => (activeTab = 'media')}
	>
		<ImageIcon class="me-2 h-4 w-4" />
		{t('app.media.tab_all', 'All Media')}
	</button>
	<button
		type="button"
		class="inline-flex items-center justify-center whitespace-nowrap rounded-md px-3 py-1.5 text-sm font-medium transition-all"
		style={activeTab === 'stock-providers'
			? 'background-color: var(--color-surface); color: var(--color-foreground); box-shadow: 0 1px 2px rgba(0,0,0,0.05)'
			: 'color: var(--color-muted)'}
		onclick={() => (activeTab = 'stock-providers')}
	>
		<Search class="me-2 h-4 w-4" />
		{t('app.media.tab_stock_providers', 'Stock Providers')}
	</button>
</div>

{#if activeTab === 'media'}
	<EntityPage {config} bind:controller={entityController} />
{:else if activeTab === 'stock-providers'}
	<!-- Stock Providers management tab -->
	<Card>
		<CardContent class="p-0">
			{#if stockProvidersLoading}
				<div class="flex items-center justify-center py-12">
					<RefreshCw class="h-5 w-5 animate-spin" style="color: var(--color-muted)" />
					<span class="ms-2 text-sm" style="color: var(--color-muted)">Loading...</span>
				</div>
			{:else if stockProvidersError}
				<div class="rounded-lg border px-4 py-3 text-sm" style="border-color: var(--color-danger); color: var(--color-danger)">
					{stockProvidersError}
				</div>
			{:else}
				<div class="overflow-x-auto">
					<table class="w-full text-sm">
						<thead class="border-b bg-surface text-start text-xs uppercase" style="color: var(--color-muted)">
							<tr>
								<th class="px-4 py-3 font-medium">{t('app.media.provider', 'Provider')}</th>
								<th class="px-4 py-3 font-medium">{t('app.media.enabled', 'Enabled')}</th>
								<th class="px-4 py-3 font-medium">{t('app.media.api_key', 'API Key')}</th>
								<th class="px-4 py-3 font-medium">{t('app.media.rbac_permission', 'RBAC Permission')}</th>
							</tr>
						</thead>
						<tbody class="divide-y" style="border-color: var(--color-border)">
							{#each stockProviders as p (p.provider_name)}
								<tr class="hover:bg-surface/50">
									<td class="px-4 py-3 font-medium" style="color: var(--color-foreground)">{p.display_name}</td>
									<td class="px-4 py-3">
										<label class="inline-flex cursor-pointer items-center gap-2">
											<input
												type="checkbox"
												checked={p.enabled}
												disabled={updatingProvider === p.provider_name}
												onchange={(e) => toggleStockProvider(p.provider_name, e.currentTarget.checked)}
											/>
											<span class="text-xs" style="color: var(--color-muted)">
												{updatingProvider === p.provider_name
													? t('common.saving', 'Saving...')
													: p.enabled
														? t('app.media.on', 'On')
														: t('app.media.off', 'Off')}
											</span>
										</label>
									</td>
									<td class="px-4 py-3">
										{#if p.provider_name === 'openverse'}
											<span class="text-xs" style="color: var(--color-muted)">{t('app.media.no_key_required', 'No key required')}</span>
										{:else}
											<input
												type="password"
												class="w-full max-w-xs rounded border px-2 py-1 text-xs"
												style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
												placeholder={p.has_api_key ? '•••••••• (enter new key to replace)' : t('app.media.paste_api_key', 'Paste API key...')}
												onkeydown={(e) => {
													if (e.key === 'Enter' && e.currentTarget.value.trim()) {
														updateProviderKey(p.provider_name, e.currentTarget.value);
														e.currentTarget.value = '';
													}
												}}
											/>
										{/if}
									</td>
									<td class="px-4 py-3">
										{#if p.provider_name === 'openverse'}
											<span class="inline-flex items-center rounded-full bg-surface px-2.5 py-0.5 text-xs font-medium" style="color: var(--color-foreground)">media.stock.search</span>
										{:else}
											<span class="inline-flex items-center rounded-full bg-surface px-2.5 py-0.5 text-xs font-medium" style="color: var(--color-foreground)">media.stock.search.{p.provider_name}</span>
										{/if}
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			{/if}
		</CardContent>
	</Card>
{/if}

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
			<Upload class="me-2 h-4 w-4" />
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
			<Search class="me-2 h-4 w-4" />
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
					accept="image/*,.heic,.heif,video/*,audio/*,application/pdf,.doc,.docx"
					onchange={(e) => {
						const files = e.currentTarget.files;
						if (files) uploadFiles = [...uploadFiles, ...Array.from(files)];
					}}
					class="block w-full text-sm text-foreground file:me-4 file:rounded-lg file:border-0 file:bg-primary file:px-4 file:py-2 file:text-sm file:font-semibold file:text-white hover:file:bg-primary/80"
				/>
			</div>
			{#if uploadFiles.length > 0}
				<div class="max-h-40 space-y-1 overflow-y-auto">
					{#each uploadFiles as file, i (i)}
						<div class="flex items-center justify-between rounded-lg bg-surface px-3 py-1.5 text-sm">
							<span class="truncate text-foreground">{file.name}</span>
							<span class="ms-2 shrink-0 text-xs text-muted">{formatFileSize(file.size)}</span>
							<button
								type="button"
								class="ms-2 shrink-0 text-muted hover:text-danger"
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
				<AutocompleteSelect
					id="media-upload-group"
					bind:value={uploadUserGroupId}
					options={[{ value: SUPER_ADMIN_GROUP_ID, label: t('app.media.upload_admin_only', 'Visible to admin') }, { value: '', label: t('app.media.upload_universal', 'Universal (all users)') }, ...userGroups.filter((g) => g.id !== SUPER_ADMIN_GROUP_ID).map((group) => ({ value: group.id, label: group.name }))]}
					class="w-full"
				/>
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
					<span class="ms-2 text-sm" style="color: var(--color-muted)">{t('app.media.loading_providers', 'Loading providers...')}</span>
				</div>
			{:else if stockModalProviders.length === 0}
				<div class="py-8 text-center text-sm" style="color: var(--color-muted)">
					{t('app.media.stock_providers_empty', 'No stock providers available. Enable providers in the Stock Providers admin tab.')}
				</div>
			{:else}
				<!-- Search bar -->
				<div class="relative">
					<Search class="absolute start-3 top-1/2 h-4 w-4 -translate-y-1/2" style="color: var(--color-muted)" />
					<input
						type="text"
						placeholder={t('media.ph_search_stock_photos', 'Search stock photos...')}
						class="w-full rounded-lg border py-2 ps-9 pe-3 text-sm"
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
					>{t('common.all', 'All')}</button>
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
							<span class="ms-2 text-sm" style="color: var(--color-muted)">Searching...</span>
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

<!-- ── Edit metadata modal (alt_text + caption/title) ─────────────────────── -->
<Modal bind:open={showEditModal} title={t('app.media.modal_edit_title', 'Edit Media Details')}>
	{#if editTarget}
		<div class="space-y-4">
			{#if editTarget.mime_type?.startsWith('image/') && editTarget.url}
				<div class="flex h-32 items-center justify-center overflow-hidden rounded-lg" style="background: var(--color-surface)">
					<img src={editTarget.url} alt={editTarget.alt_text || editTarget.filename} class="max-h-full object-contain" />
				</div>
			{/if}
			<div>
				<label for="edit-caption" class="mb-1 block text-sm font-medium text-foreground">
					{t('app.media.field_title', 'Title / Caption')}
				</label>
				<input
					id="edit-caption"
					type="text"
					bind:value={editCaption}
					placeholder={t('app.media.title_placeholder', 'Descriptive title for this media...')}
					class="w-full rounded-lg border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary"
				/>
			</div>
			<div>
				<label for="edit-alt" class="mb-1 block text-sm font-medium text-foreground">
					{t('app.media.field_alt_text', 'Alt Text')}
				</label>
				<textarea
					id="edit-alt"
					bind:value={editAltText}
					rows="3"
					placeholder={t('app.media.alt_placeholder', 'Accessibility description for screen readers...')}
					class="w-full rounded-lg border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary"
				></textarea>
				<p class="mt-1 text-xs text-muted">{t('app.media.alt_hint', 'Used by screen readers and SEO. Describe the image content.')}</p>
			</div>
		</div>
		<div class="mt-4 flex justify-end gap-2">
			<Button variant="ghost" onclick={() => (showEditModal = false)}>{t('common.cancel', 'Cancel')}</Button>
			<Button variant="default" onclick={saveEdit} disabled={editSaving}>
				{#if editSaving}
					<Loader2 class="me-2 h-4 w-4 animate-spin" />
				{:else}
					<Save class="me-2 h-4 w-4" />
				{/if}
				{editSaving ? t('common.saving', 'Saving...') : t('common.save', 'Save')}
			</Button>
		</div>
	{/if}
</Modal>

<!-- ── Image viewer (fullscreen lightbox) ─────────────────────────────────── -->
<svelte:window onkeydown={(e) => { if (e.key === 'Escape' && showViewer) closeViewer(); }} />
{#if showViewer && viewerItem}
	<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/90"
		role="dialog"
		aria-modal="true"
		tabindex="-1"
		aria-label={t('app.media.viewer_aria', 'Image viewer')}
		onclick={closeViewer}
		onwheel={onViewerWheel}
	>
		<!-- Toolbar -->
		<div class="absolute top-0 start-0 end-0 flex items-center justify-between p-4 text-white" onclick={(e) => e.stopPropagation()}>
			<div class="flex items-center gap-2">
				<Button variant="ghost" size="sm" class="text-white hover:bg-white/10" onclick={closeViewer}>
					<X class="h-5 w-5" />
				</Button>
				<span class="truncate text-sm font-medium">{viewerItem.filename}</span>
			</div>
			<div class="flex items-center gap-1">
				<Button variant="ghost" size="sm" class="text-white hover:bg-white/10" onclick={zoomOut}>
					<ZoomOut class="h-5 w-5" />
				</Button>
				<span class="min-w-[3.5rem] text-center text-sm tabular-nums">{Math.round(viewerZoom * 100)}%</span>
				<Button variant="ghost" size="sm" class="text-white hover:bg-white/10" onclick={zoomIn}>
					<ZoomIn class="h-5 w-5" />
				</Button>
				<Button variant="ghost" size="sm" class="text-white hover:bg-white/10" onclick={resetZoom}>
					<Maximize class="h-5 w-5" />
				</Button>
			</div>
		</div>

		<!-- Image -->
		<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
		<div
			class="flex max-h-full max-w-full items-center justify-center overflow-hidden"
			style="cursor: {viewerZoom > 1 ? (viewerPanning ? 'grabbing' : 'grab') : 'default'}"
			onclick={(e) => e.stopPropagation()}
			onpointerdown={onViewerPointerDown}
			onpointermove={onViewerPointerMove}
			onpointerup={onViewerPointerUp}
			onpointerleave={onViewerPointerUp}
		>
			{#if viewerItem.mime_type?.startsWith('image/') && viewerItem.url}
				<img
					src={viewerItem.url}
					alt={viewerItem.alt_text || viewerItem.filename}
					class="max-h-[90vh] max-w-[90vw] select-none object-contain transition-transform"
					style="transform: scale({viewerZoom}) translate({viewerPanX / viewerZoom}px, {viewerPanY / viewerZoom}px);"
					draggable="false"
				/>
			{:else if viewerItem.mime_type?.startsWith('video/') && viewerItem.url}
				<video
					src={viewerItem.url}
					controls
					autoplay
					class="max-h-[90vh] max-w-[90vw]"
				>
					<track kind="captions" />
				</video>
			{:else}
				<div class="text-center text-white/70">
					<FileText class="mx-auto mb-2 h-16 w-16" />
					<p class="text-sm">{t('app.media.no_preview', 'No preview available')}</p>
					<a href={viewerItem.url} download class="mt-2 inline-block text-sm text-primary underline">{t('common.download', 'Download')}</a>
				</div>
			{/if}
		</div>
	</div>
{/if}

<!-- ── Image resizer modal ───────────────────────────────────────────────── -->
<Modal bind:open={showResizer} title={t('app.media.modal_resizer_title', 'Resize Image')}>
	{#if resizerItem}
		<div class="space-y-4">
			{#if resizerError}
				<div class="rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-danger); color: var(--color-danger)">
					{resizerError}
				</div>
			{/if}

			{#if resizerOriginal}
				<div class="text-xs text-muted">
					{t('app.media.original_size', 'Original')}: {resizerOriginal.w} × {resizerOriginal.h}px
				</div>
			{/if}

			<!-- Dimension inputs -->
			<div class="grid grid-cols-2 gap-3">
				<div>
					<label for="resize-width" class="mb-1 block text-sm font-medium text-foreground">{t('app.media.width', 'Width (px)')}</label>
					<input
						id="resize-width"
						type="number"
						min="1"
						bind:value={resizerWidth}
						onchange={onResizerWidthChange}
						oninput={onResizerWidthChange}
						class="w-full rounded-lg border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary"
					/>
				</div>
				<div>
					<label for="resize-height" class="mb-1 block text-sm font-medium text-foreground">{t('app.media.height', 'Height (px)')}</label>
					<input
						id="resize-height"
						type="number"
						min="1"
						bind:value={resizerHeight}
						onchange={onResizerHeightChange}
						oninput={onResizerHeightChange}
						class="w-full rounded-lg border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary"
					/>
				</div>
			</div>

			<!-- Options row -->
			<div class="flex flex-wrap items-center gap-4">
				<label class="inline-flex cursor-pointer items-center gap-2 text-sm text-foreground">
					<input type="checkbox" bind:checked={resizerMaintainAspect} />
					{t('app.media.maintain_aspect', 'Maintain aspect ratio')}
				</label>
				<div class="flex items-center gap-2 text-sm text-foreground">
					<label for="resize-format">{t('app.media.format', 'Format')}</label>
					<AutocompleteSelect
						id="resize-format"
						bind:value={resizerFormat}
						onValueChange={() => renderResizerFromSource()}
						options={[{ value: 'jpeg', label: 'JPEG' }, { value: 'png', label: 'PNG' }, { value: 'webp', label: 'WebP' }]}
					/>
				</div>
				<div class="flex items-center gap-2 text-sm text-foreground">
					<label for="resize-quality">{t('app.media.quality', 'Quality')}</label>
					<input id="resize-quality" type="range" min="0.1" max="1" step="0.05" bind:value={resizerQuality} onchange={renderResizerFromSource} />
					<span class="tabular-nums text-xs text-muted">{Math.round(resizerQuality * 100)}%</span>
				</div>
			</div>

			<!-- Preview -->
			{#if resizerPreviewUrl}
				<div>
					<p class="mb-1 text-xs font-medium uppercase tracking-wider text-muted">{t('app.media.preview', 'Preview')}</p>
					<div class="flex max-h-48 items-center justify-center overflow-hidden rounded-lg border" style="border-color: var(--color-border); background: var(--color-surface)">
						<img src={resizerPreviewUrl} alt={t('app.media.resize_preview_alt', 'Resize preview')} class="max-h-48 object-contain" />
					</div>
					<p class="mt-1 text-xs text-muted">
						{resizerWidth} × {resizerHeight}px
					</p>
				</div>
			{:else if !resizerError}
				<div class="flex items-center justify-center py-8">
					<Loader2 class="h-5 w-5 animate-spin" style="color: var(--color-muted)" />
					<span class="ms-2 text-sm" style="color: var(--color-muted)">{t('common.loading', 'Loading...')}</span>
				</div>
			{/if}
		</div>

		<div class="mt-4 flex justify-end gap-2">
			<Button variant="ghost" onclick={closeResizer}>{t('common.cancel', 'Cancel')}</Button>
			<Button variant="default" onclick={downloadResized} disabled={!resizerCanvas}>
				<Download class="me-2 h-4 w-4" />
				{t('app.media.download_resized', 'Download Resized')}
			</Button>
		</div>
	{/if}
</Modal>

<!-- ── Force Delete confirmation (admin.hard_delete — permanent) ─────────── -->
<Modal bind:open={showForceModal} title={t('common.force_delete', 'Force Delete')} size="sm">
	<p class="text-sm" style="color: var(--color-foreground)">
		{t('common.force_delete_confirm_bulk', 'Permanently delete {n} item(s)? This cannot be undone.').replace('{n}', String(forceTargets.length))}
	</p>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => { showForceModal = false; forceTargets = []; }}>{t('common.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={confirmForceDelete} disabled={bulkActing}>
			{bulkActing ? t('common.deleting', 'Deleting...') : t('common.force_delete', 'Force Delete')}
		</Button>
	</div>
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
			<p class="truncate font-medium text-foreground">{row.caption || row.filename}</p>
			<p class="truncate text-xs text-muted">{row.alt_text || row.filename}</p>
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
				{#if row.mime_type?.startsWith('image/') || row.mime_type?.startsWith('video/')}
					<Button variant="secondary" size="sm" class="h-8 w-8 p-0" onclick={() => openViewer(row)} title={t('app.media.view_full', 'View full image')}>
						<Maximize class="h-4 w-4" />
					</Button>
				{/if}
				<Button variant="secondary" size="sm" class="h-8 w-8 p-0" onclick={() => entityController?.openView(row)} title={t('common.view', 'View')}>
					<Eye class="h-4 w-4" />
				</Button>
				{#if row.mime_type?.startsWith('image/')}
					<Button variant="secondary" size="sm" class="h-8 w-8 p-0" onclick={() => openResizer(row)} title={t('app.media.resize', 'Resize')}>
						<Maximize class="h-4 w-4" />
					</Button>
				{/if}
				{#if canEditMedia}
					<Button variant="secondary" size="sm" class="h-8 w-8 p-0" onclick={() => openEditModal(row)} title={t('common.edit', 'Edit')}>
						<Pencil class="h-4 w-4" />
					</Button>
				{/if}
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
			<p class="truncate text-sm font-medium text-foreground">{row.caption || row.filename}</p>
			{#if row.caption}
				<p class="truncate text-xs text-muted">{row.filename}</p>
			{/if}
			<div class="mt-1 flex items-center justify-between">
				<p class="text-xs text-muted">{formatFileSize(row.file_size)}</p>
				<span class="inline-flex items-center rounded-full px-1.5 py-0.5 text-[10px] font-medium {row.status === 'approved' ? 'bg-success/15 text-success' : row.status === 'pending' ? 'bg-warning/15 text-warning' : row.status === 'quarantined' ? 'bg-danger/15 text-danger' : 'bg-surface text-foreground-alt'}">
					{row.status || 'pending'}
				</span>
			</div>
		</CardContent>
	</Card>
{/snippet}

<!-- View modal body (mirrors the legacy view modal) -->
{#snippet MediaView({ row }: { row: MediaItem })}
	<div class="space-y-4">
		<div class="relative flex aspect-video items-center justify-center rounded-lg" style="background: var(--color-surface)">
			{#if row.mime_type?.startsWith('image/') && row.url}
				<img src={row.url} alt={row.alt_text || row.filename} class="max-h-full rounded-lg object-contain" />
				<!-- Fullscreen viewer button overlay -->
				<button
					type="button"
					class="absolute end-2 top-2 inline-flex h-8 w-8 items-center justify-center rounded-lg bg-black/50 text-white opacity-0 transition-opacity hover:bg-black/70 focus:opacity-100"
					onclick={() => openViewer(row)}
					title={t('app.media.view_full', 'View full image')}
					aria-label={t('app.media.view_full', 'View full image')}
				>
					<Maximize class="h-4 w-4" />
				</button>
			{:else if row.mime_type?.startsWith('video/') && row.url}
				<video src={row.url} controls class="max-h-full rounded-lg">
					<track kind="captions" />
				</video>
			{:else}
				{@const Icon = getIcon(row.mime_type)}
				<Icon class="h-12 w-12" style="color: var(--color-muted)" />
			{/if}
		</div>

		<!-- Action buttons -->
		<div class="flex flex-wrap gap-2">
			{#if row.mime_type?.startsWith('image/') || row.mime_type?.startsWith('video/')}
				<Button variant="secondary" size="sm" onclick={() => openViewer(row)}>
					<Maximize class="me-2 h-4 w-4" />
					{t('app.media.view_full', 'View Full')}
				</Button>
			{/if}
			{#if row.mime_type?.startsWith('image/')}
				<Button variant="secondary" size="sm" onclick={() => openResizer(row)}>
					<Maximize class="me-2 h-4 w-4" />
					{t('app.media.resize', 'Resize')}
				</Button>
			{/if}
			{#if canEditMedia}
				<Button variant="secondary" size="sm" onclick={() => openEditModal(row)}>
					<Pencil class="me-2 h-4 w-4" />
					{t('common.edit', 'Edit')}
				</Button>
			{/if}
			{#if row.url}
				<a href={row.url} download={row.filename} target="_blank" rel="noopener">
					<Button variant="secondary" size="sm">
						<Download class="me-2 h-4 w-4" />
						{t('common.download', 'Download')}
					</Button>
				</a>
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
			{#if row.width || row.height}
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.media.dimensions', 'Dimensions')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.width ?? '—'} × {row.height ?? '—'}px</p>
				</div>
			{/if}
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
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.media.field_title', 'Title / Caption')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.caption || '—'}</p>
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
