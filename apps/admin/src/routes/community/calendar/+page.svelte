<script lang="ts">
	// /community/calendar — admin HITL review queue for Calendar events with approve/reject.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { calendarAdminApi } from '$lib/api';
	import { useCountries } from '$lib/utils/countries.svelte';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, CheckCircle, XCircle, Trash2 } from 'lucide-svelte';
	import type { ActionItem, FilterDef } from '$lib/components/ui/data-table';

	type CalendarItem = {
		id: string;
		title: string;
		occurs_at: string;
		ends_at?: string;
		status: string;
		description?: string;
		category?: string;
		destination_space_id?: string;
		origin_space_id?: string;
		source_url?: string;
		deleted_at?: string;
		// ADR-088 AI generation columns
		ai_generated?: boolean;
		ai_model?: string;
		hitl_reviewed_by?: string;
		hitl_review_notes?: string;
		ai_generated_at?: string;
	};

	const canManage = $derived(hasPermission('calendar.review'));
	// BUG-001/BUG-008: calendar.manage (admin+) gates the soft-delete action.
	const canDelete = $derived(hasPermission('calendar.manage'));
	// Rule 11 amendment: permanent delete requires the dedicated node.
	const canHardDelete = $derived(hasPermission('admin.hard_delete'));

	// ── Country selector (250 primary geo space countries) ───────────────────
	const countryUtils = useCountries();
	$effect(() => {
		countryUtils.load();
	});

	// ── State ───────────────────────────────────────────────────────────────
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let acting = $state(false);
	let actionTarget = $state<CalendarItem | null>(null);
	let showRejectModal = $state(false);
	let rejectReason = $state('');
	// BUG-003 / ADR-088: AI-generated drafts require HITL notes on first publish.
	let showHitlModal = $state(false);
	let hitlNotes = $state('');
	// BUG-008: soft-delete confirm modal state.
	let showDeleteModal = $state(false);
	let deleteTarget = $state<CalendarItem | null>(null);
	// Force-delete (admin.hard_delete) — permanent removal of soft-deleted rows.
	let showForceModal = $state(false);
	let forceTarget = $state<CalendarItem | null>(null);

	function openReject(item: CalendarItem) {
		actionTarget = item;
		rejectReason = '';
		showRejectModal = true;
	}

	async function confirmReject() {
		if (!actionTarget) return;
		acting = true;
		pageError = '';
		try {
			// §86: 'rejected' is not a valid status. Use 'archived' to reject.
			const result = await calendarAdminApi.update(actionTarget.id, { status: 'archived', reason: rejectReason });
			if (result.ok) {
				showRejectModal = false;
				entityController?.reload();
			} else {
				pageError = result.error ?? t('common.error', 'An error occurred');
			}
		} catch {
			pageError = t('common.error', 'An error occurred');
		} finally {
			acting = false;
		}
	}

	function openApprove(item: CalendarItem) {
		// ADR-088 HITL gate: AI-generated content requires HITL notes before
		// first publication. On subsequent re-publishes hitl_reviewed_by is set.
		if (item.ai_generated && !item.hitl_reviewed_by) {
			actionTarget = item;
			hitlNotes = '';
			showHitlModal = true;
			return;
		}
		void approveEvent(item, '');
	}

	async function approveEvent(item: CalendarItem, reason: string) {
		acting = true;
		pageError = '';
		try {
			// §86: 'approved' is now 'published' per spec status enum.
			const result = await calendarAdminApi.update(item.id, { status: 'published', reason });
			if (result.ok) {
				showHitlModal = false;
				entityController?.reload();
			} else {
				// BUG-003: handle server-side HITL gate and source_url errors.
				if (result.error?.includes('HITL review required')) {
					pageError = t('app.calendar.hitl_required', 'Cannot approve AI-generated content without HITL review');
				} else if (result.error?.includes('source_url') || result.error?.includes('source URL')) {
					pageError = t('app.calendar.source_url_required', 'A verifiable official source URL is required before publication');
				} else {
					pageError = result.error ?? t('common.error', 'An error occurred');
				}
			}
		} catch {
			pageError = t('common.error', 'An error occurred');
		} finally {
			acting = false;
		}
	}

	async function confirmApprove() {
		if (!actionTarget) return;
		await approveEvent(actionTarget, hitlNotes);
	}

	// BUG-008: soft-delete entry (calendar.manage, admin+). Per Rule 11, user
	// content is soft-deleted, never hard-deleted.
	function openDelete(item: CalendarItem) {
		deleteTarget = item;
		showDeleteModal = true;
	}

	async function confirmDelete() {
		if (!deleteTarget) return;
		acting = true;
		pageError = '';
		try {
			const result = await calendarAdminApi.delete(deleteTarget.id);
			if (result.ok) {
				showDeleteModal = false;
				entityController?.reload();
			} else {
				pageError = result.error ?? t('common.error', 'An error occurred');
			}
		} catch {
			pageError = t('common.error', 'An error occurred');
		} finally {
			acting = false;
		}
	}

	// Force delete — admin.hard_delete-gated ?force=true on the same endpoint;
	// the backend rejects rows that are not already soft-deleted.
	function openForceDelete(item: CalendarItem) {
		forceTarget = item;
		showForceModal = true;
	}

	async function confirmForceDelete() {
		if (!forceTarget) return;
		acting = true;
		pageError = '';
		try {
			const result = await calendarAdminApi.hardDelete(forceTarget.id);
			if (result.ok) {
				showForceModal = false;
				forceTarget = null;
				entityController?.reload();
			} else {
				pageError = result.error ?? t('common.error', 'An error occurred');
			}
		} catch {
			pageError = t('common.error', 'An error occurred');
		} finally {
			acting = false;
		}
	}

	// ── Row actions ─────────────────────────────────────────────────────────
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row: CalendarItem, ctx) => ctx?.openView(_row) }
		];
		if (canManage) {
			acts.push({
				label: t('app.calendar.approve', 'Approve'),
				icon: CheckCircle,
				onclick: openApprove,
				show: (row: CalendarItem) => !row.deleted_at
			});
			acts.push({
				label: t('app.calendar.reject', 'Reject'),
				icon: XCircle,
				variant: 'destructive',
				onclick: openReject,
				show: (row: CalendarItem) => !row.deleted_at
			});
		}
		// BUG-008: soft-delete (calendar.manage, admin+).
		if (canDelete) {
			acts.push({
				label: t('common.delete', 'Delete'),
				icon: Trash2,
				variant: 'destructive',
				onclick: openDelete,
				show: (row: CalendarItem) => !row.deleted_at
			});
		}
		if (canHardDelete) {
			// Permanent removal — only for already-soft-deleted rows.
			acts.push({
				label: t('common.force_delete', 'Force Delete'),
				icon: Trash2,
				variant: 'destructive',
				onclick: openForceDelete,
				show: (row: CalendarItem) => !!row.deleted_at
			});
		}
		return acts;
	});

	const fields: FieldDef[] = [
		{ key: 'title', label: t('app.calendar.col_title', 'Title'), type: 'text' },
		{ key: 'occurs_at', label: t('app.calendar.col_start_date', 'Start Date'), type: 'datetime-local' },
		{ key: 'ends_at', label: t('app.calendar.col_end_date', 'End Date'), type: 'datetime-local' },
		{ key: 'status', label: t('app.calendar.col_status', 'Status'), type: 'text' },
		{ key: 'category', label: t('app.calendar.col_category', 'Category'), type: 'text' },
		{ key: 'source_url', label: t('app.calendar.col_source', 'Source URL'), type: 'url' },
		{ key: 'description', label: t('app.calendar.detail_description', 'Description'), type: 'textarea' }
	];

	const filters = $derived.by<FilterDef[]>(() => {
		countryUtils.ready;
		return [
			{
				key: 'dest',
				label: t('app.calendar.col_destination', 'Destination'),
				type: 'autocomplete',
				placeholder: t('app.tools.country_placeholder', 'Search country…'),
				emptyMessage: t('common.no_results', 'No results found'),
				fetcher: countryUtils.fetcher,
				initialFetcher: countryUtils.initialFetcher,
				getSelectedLabel: countryUtils.getSelectedLabel
			},
			// Soft-deleted events are hidden by default — this filter surfaces
			// them so the Force Delete action has targets.
			{
				key: 'deleted',
				label: t('common.filter_deleted', 'Deleted'),
				options: [
					{ value: 'only', label: t('common.filter_deleted_only', 'Deleted only') },
					{ value: 'all', label: t('common.filter_deleted_all', 'Include deleted') }
				]
			}
		];
	});

	const config = $derived<EntityPageConfig>({
		filters,
		title: t('app.calendar.review_queue_heading', 'Calendar Review Queue'),
		description: t('app.calendar.review_queue_desc', 'Review and approve pending calendar events'),
		readonly: true,
		pageSize: 25,
		columns: [
			{ key: 'title', label: t('app.calendar.col_title', 'Title'), sortable: true },
			{
				key: 'occurs_at',
				label: t('app.calendar.col_start_date', 'Start Date'),
				sortable: true,
				render: (value: string) => (value ? new Date(value).toLocaleDateString() : '—')
			},
			{
				key: 'ends_at',
				label: t('app.calendar.col_end_date', 'End Date'),
				sortable: true,
				render: (value: string) => (value ? new Date(value).toLocaleDateString() : '—')
			},
			{
				key: 'status',
				label: t('app.calendar.col_status', 'Status'),
				sortable: true,
				render: (value: string, row: CalendarItem) =>
					row.deleted_at
						? `<span class="inline-block rounded-full px-2 py-0.5 text-xs bg-danger/15 text-danger">${t('common.filter_deleted', 'Deleted')}</span>`
						: value || '—'
			},
			{
				key: 'category',
				label: t('app.calendar.col_category', 'Category'),
				sortable: true,
				render: (value: string) => value || '—'
			}
		],
		actions: rowActions,
		fields,
		load: async (params) => {
			// BUG-020: pass pagination params to the API for server-side paging.
			const queryParts: string[] = [];
			queryParts.push(`page=${params.page + 1}`);
			if (params.pageSize) queryParts.push(`page_size=${params.pageSize}`);
			if (params.q) queryParts.push(`q=${encodeURIComponent(params.q)}`);
			if (params.sort) {
				// EntityPage passes sort as "key:dir" (e.g. "title:asc").
				// The backend expects separate sort and dir params.
				const [sortKey, sortDir] = params.sort.split(':');
				queryParts.push(`sort=${sortKey}`);
				// BUG-007: pass sort direction (dir) so column sort toggles
				// can reverse direction.
				if (sortDir) {
					queryParts.push(`dir=${sortDir}`);
				}
			}
			if (params.filter) {
				for (const [k, v] of Object.entries(params.filter)) {
					if (v) queryParts.push(`${k}=${encodeURIComponent(String(v))}`);
				}
			}
			const query = queryParts.length > 0 ? `?${queryParts.join('&')}` : '';
			const result = await calendarAdminApi.list(query);
			if (result.ok) {
				// Backend returns { data, meta: { page, per_page, total, total_pages } }
				const body = result.data as any;
				const data = (body?.data ?? body) as CalendarItem[];
				const meta = body?.meta;
				if (meta) {
					return { data, total: meta.total, totalPages: meta.total_pages };
				}
				const pageSize = params.pageSize || 10;
				const start = params.page * pageSize;
				const total = data.length;
				return { data: data.slice(start, start + pageSize), total, totalPages: Math.max(1, Math.ceil(total / pageSize)) };
			}
			throw new Error(result.error ?? t('common.error', 'An error occurred'));
		},
		viewSnippet: CalendarView
	});
</script>

<svelte:head>
	<title>{t('app.calendar.review_queue_page_title', 'Calendar Review Queue — {site} Admin')}</title>
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

<!-- ── HITL review notes modal (BUG-003 / ADR-088) ──────────────────────── -->
<Modal bind:open={showHitlModal} title={t('app.calendar.modal_hitl_title', 'HITL Review — Approve AI-Generated Event')}>
	<div class="space-y-4">
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('app.calendar.hitl_confirm', 'This event was generated by AI. Add review notes before approving "{title}".').replace('{title}', actionTarget?.title ?? '')}
		</p>
		<div>
			<label for="hitl-notes" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.calendar.hitl_notes', 'HITL review notes')}</label>
			<textarea
				id="hitl-notes"
				class="w-full rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				rows="3"
				bind:value={hitlNotes}
				placeholder={t('app.calendar.hitl_placeholder', 'Describe what was verified before approving...')}
			></textarea>
		</div>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showHitlModal = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button onclick={confirmApprove} disabled={acting || !hitlNotes.trim()}>
			{acting ? t('app.calendar.approving', 'Approving...') : t('app.calendar.approve', 'Approve')}
		</Button>
	</div>
</Modal>

<!-- ── Reject confirmation modal ─────────────────────────────────────────── -->
<Modal bind:open={showRejectModal} title={t('app.calendar.modal_reject_event', 'Reject Event')}>
	<div class="space-y-4">
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('app.calendar.reject_confirm', 'Are you sure you want to reject "{title}"?').replace('{title}', actionTarget?.title ?? '')}
		</p>
		<div>
			<label for="reject-reason" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.calendar.reason_optional', 'Reason (optional)')}</label>
			<textarea
				id="reject-reason"
				class="w-full rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				rows="3"
				bind:value={rejectReason}
				placeholder={t('app.calendar.reject_placeholder', 'Explain why this event was rejected...')}
			></textarea>
		</div>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showRejectModal = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={confirmReject} disabled={acting}>
			{acting ? t('app.calendar.rejecting', 'Rejecting...') : t('app.calendar.reject', 'Reject')}
		</Button>
	</div>
</Modal>

<!-- ── Delete confirmation modal (BUG-008: soft-delete, calendar.manage) ─── -->
<Modal bind:open={showDeleteModal} title={t('app.calendar.modal_delete_event', 'Delete Event')}>
	<div class="space-y-4">
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('app.calendar.delete_confirm', 'Are you sure you want to delete "{title}"? This will hide it from the public calendar.').replace('{title}', deleteTarget?.title ?? '')}
		</p>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showDeleteModal = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={confirmDelete} disabled={acting}>
			{acting ? t('app.calendar.deleting', 'Deleting...') : t('common.delete', 'Delete')}
		</Button>
	</div>
</Modal>

<!-- ── Force Delete confirmation (admin.hard_delete — permanent) ─────────── -->
<Modal bind:open={showForceModal} title={t('common.force_delete', 'Force Delete')} size="sm">
	<div class="space-y-4">
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('common.force_delete_confirm', 'Permanently delete "{name}"? This cannot be undone.').replace('{name}', forceTarget?.title ?? '')}
		</p>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => { showForceModal = false; forceTarget = null; }}>{t('common.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={confirmForceDelete} disabled={acting}>
			{acting ? t('common.deleting', 'Deleting...') : t('common.force_delete', 'Force Delete')}
		</Button>
	</div>
</Modal>

<!-- ── View slide-over body ──────────────────────────────────────────────── -->
{#snippet CalendarView({ row }: { row: CalendarItem })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.calendar.col_title', 'Title')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.title}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.calendar.col_start_date', 'Start Date')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.occurs_at ? new Date(row.occurs_at).toLocaleString() : '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.calendar.col_end_date', 'End Date')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.ends_at ? new Date(row.ends_at).toLocaleString() : '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.calendar.col_status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.calendar.col_category', 'Category')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.category || '—'}</p>
		</div>
		{#if row.source_url}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.calendar.col_source', 'Source URL')}</p>
				<a href={row.source_url} target="_blank" rel="noopener noreferrer nofollow" class="mt-1 text-sm font-medium text-blue-500 hover:underline" style="color: var(--color-primary)">{row.source_url}</a>
			</div>
		{/if}
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.calendar.detail_description', 'Description')}</p>
			<p class="mt-1 whitespace-pre-wrap text-sm" style="color: var(--color-foreground)">{row.description || '—'}</p>
		</div>
		{#if row.ai_generated}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.common.ai_generated', 'AI Generated')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{t('common.yes', 'Yes')} ({row.ai_model ?? '—'})</p>
			</div>
		{/if}
		{#if row.hitl_reviewed_by}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.common.hitl_reviewed_by', 'HITL Reviewed By')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.hitl_reviewed_by}</p>
			</div>
		{/if}
		{#if row.hitl_review_notes}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.common.hitl_review_notes', 'HITL Review Notes')}</p>
				<p class="mt-1 whitespace-pre-wrap text-sm" style="color: var(--color-foreground)">{row.hitl_review_notes}</p>
			</div>
		{/if}
	</div>
{/snippet}
