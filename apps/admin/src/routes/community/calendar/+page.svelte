<script lang="ts">
	// /community/calendar — admin HITL review queue for Calendar events with approve/reject.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { calendarAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, CheckCircle, XCircle } from 'lucide-svelte';
	import type { ActionItem } from '$lib/components/ui/data-table';

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
		// ADR-088 AI generation columns
		ai_generated?: boolean;
		ai_model?: string;
		hitl_reviewed_by?: string;
		ai_generated_at?: string;
	};

	const canManage = $derived(hasPermission('calendar.review'));

	// ── State ───────────────────────────────────────────────────────────────
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let acting = $state(false);
	let actionTarget = $state<CalendarItem | null>(null);
	let showRejectModal = $state(false);
	let rejectReason = $state('');

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

	async function approveEvent(item: CalendarItem) {
		acting = true;
		pageError = '';
		try {
			// §86: 'approved' is now 'published' per spec status enum.
			const result = await calendarAdminApi.update(item.id, { status: 'published' });
			if (result.ok) {
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
				onclick: approveEvent,
				show: () => true
			});
			acts.push({
				label: t('app.calendar.reject', 'Reject'),
				icon: XCircle,
				variant: 'destructive',
				onclick: openReject,
				show: () => true
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

	const config = $derived<EntityPageConfig>({
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
				render: (value: string) => value || '—'
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
			if (params.page) queryParts.push(`page=${params.page}`);
			if (params.pageSize) queryParts.push(`page_size=${params.pageSize}`);
			if (params.q) queryParts.push(`q=${encodeURIComponent(params.q)}`);
			if (params.sort) {
				queryParts.push(`sort=${params.sort}`);
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
				return { data, total: data.length, totalPages: 1 };
			}
			throw new Error(result.error ?? t('common.error', 'An error occurred'));
		},
		viewSnippet: CalendarView
	});
</script>

<svelte:head>
	<title>{t('app.calendar.review_queue_page_title', 'Calendar Review Queue — JapaConnect Admin')}</title>
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
	</div>
{/snippet}
