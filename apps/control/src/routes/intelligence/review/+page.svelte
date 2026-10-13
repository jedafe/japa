<script lang="ts">
	import { api, apiList } from '$lib/api';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import SearchInput from '$lib/components/ui/SearchInput.svelte';
	import AccessDenied from '$lib/components/AccessDenied.svelte';
	import type { DataTableColumn, DataTableRowAction, DataTableBulkAction } from '$lib/api/types';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasAnyPermission } from '$lib/stores/auth.svelte';
	import { escapeHtml } from '$lib/utils';
	import { goto } from '$app/navigation';
	import { page as pageStore } from '$app/state';
	import { RefreshCw, Eye, CheckCircle2, XCircle, ClipboardList } from 'lucide-svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';

	// The review record shape mirrors the IntelRecord row returned by
	// /api/v1/intel/review/queue. JSON blob fields are kept as-is.
	interface IntelRecord {
		id: string;
		source_id: string;
		raw_document_id: string;
		entity_type: string;
		external_key: string;
		extracted?: unknown;
		confidence?: unknown;
		confidence_overall?: number;
		status: string;
		freshness_score?: number;
		extracted_at?: string;
		last_verified_at?: string;
		[hash: string]: unknown;
	}

	let {} = $props();

	const canView = $derived(hasAnyPermission(['intel.review.view', 'intel.review.*']));
	const canApprove = $derived(hasAnyPermission(['intel.review.approve', 'intel.review.*']));
	const canReject = $derived(hasAnyPermission(['intel.review.reject', 'intel.review.*']));

	let records = $state<IntelRecord[]>([]);
	let loading = $state(true);
	let refreshing = $state(false);
	let errorMessage = $state('');
	let noticeMessage = $state('');
	let filter = $state('');
	let filterEntity = $state('all');

	let page = $state(0);
	let pageSize = $state(10);
	let sortKey = $state('');
	let sortDirection = $state<'asc' | 'desc'>('asc');
	let selectedIds = $state(new Set<string>());
	let totalRows = $state(0);

	let actionLoading = $state<string | null>(null);
	let bulkBusy = $state(false);

	// Status filter: read from URL query param ?status=. Defaults to
	// 'review_required' (the standard review queue). Supported values:
	// review_required, approved, published, rejected, extracted, stale.
	const statusFilter = $derived(pageStore.url.searchParams.get('status') || 'review_required');

	const statusLabels: Record<string, string> = {
		review_required: 'Review Required',
		approved: 'Approved',
		published: 'Published',
		rejected: 'Rejected',
		extracted: 'Extracted',
		stale: 'Stale'
	};

	const statusIsReviewable = $derived(statusFilter === 'review_required');
	const statusIsApproved = $derived(statusFilter === 'approved');

	async function load() {
		refreshing = true;
		errorMessage = '';
		const params = new URLSearchParams();
		params.set('page_size', String(pageSize));
		params.set('page', String(page + 1)); // backend pagination is 1-based
		if (debouncedFilter) params.set('q', debouncedFilter);
		if (filterEntity !== 'all') params.set('entity_type', filterEntity);
		if (sortKey) {
			params.set('sort', sortKey);
			params.set('dir', sortDirection);
		}
		// When status is review_required, use the dedicated review queue
		// endpoint. For other statuses, use the generic records endpoint —
		// both honor the same filter/sort/page params server-side.
		const endpoint = statusFilter === 'review_required'
			? `/api/v1/intel/review/queue?${params.toString()}`
			: `/api/v1/intel/records?status=${statusFilter}&${params.toString()}`;
		const result = await apiList<IntelRecord[]>(endpoint);
		if (result.ok) {
			records = Array.isArray(result.data) ? (result.data as IntelRecord[]) : [];
			totalRows = Number(result.meta?.total ?? records.length);
		} else {
			errorMessage = result.error;
		}
		loading = false;
		refreshing = false;
	}

	// Server-side list: reload whenever page/page-size/sort/entity/status
	// or the debounced search text changes.
	let filterDebounce: ReturnType<typeof setTimeout> | null = null;
	let debouncedFilter = $state('');

	$effect(() => {
		void filter;
		if (filterDebounce) clearTimeout(filterDebounce);
		filterDebounce = setTimeout(() => {
			if (debouncedFilter !== filter) page = 0;
			debouncedFilter = filter;
		}, 300);
	});

	$effect(() => {
		// Track every server-side list parameter so a change re-runs load().
		void statusFilter;
		void page;
		void pageSize;
		void sortKey;
		void sortDirection;
		void filterEntity;
		void debouncedFilter;
		load();
	});

	const entityItems = $derived([
		{ value: 'all', label: t('intel.filter_all_entities', 'All entity types') },
		{ value: 'scholarship', label: t('intel.category_scholarship', 'Scholarship') },
		{ value: 'job', label: t('intel.category_job', 'Job') },
		{ value: 'visa', label: t('intel.category_visa', 'Visa') },
		{ value: 'housing', label: t('intel.category_housing', 'Housing') },
		{ value: 'remittance', label: t('intel.category_remittance', 'Remittance') },
		{ value: 'cost_of_living', label: t('intel.category_cost_of_living', 'Cost of Living') },
		{ value: 'guide', label: t('intel.category_guide', 'Guide') },
		{ value: 'news', label: t('intel.category_news', 'News') },
		{ value: 'community', label: t('intel.category_community', 'Community') },
		{ value: 'mentorship', label: t('intel.category_mentorship', 'Mentorship') }
	]);

	function formatTime(ts?: string): string {
		if (!ts) return '—';
		try {
			return new Date(ts).toLocaleString();
		} catch {
			return ts;
		}
	}

	function confidencePct(v?: number): string {
		if (v == null) return '—';
		return (v * 100).toFixed(0) + '%';
	}

	const columns: DataTableColumn[] = [
		{
			key: 'external_key',
			label: t('intel.external_key', 'External Key'),
			sortable: true,
			render: (val) => `<span class="font-mono text-xs text-muted-foreground">${escapeHtml(val)}</span>`
		},
		{
			key: 'entity_type',
			label: t('intel.field_category', 'Type'),
			sortable: true,
			render: (val) => `<span class="text-muted-foreground capitalize">${escapeHtml(val)}</span>`
		},
		{
			key: 'confidence_overall',
			label: t('intel.confidence', 'Confidence'),
			sortable: true,
			render: (val) => `<span class="font-mono text-xs">${confidencePct(Number(val))}</span>`
		},
		{
			key: 'freshness_score',
			label: t('intel.freshness', 'Freshness'),
			sortable: true,
			render: (val) => `<span class="font-mono text-xs text-muted-foreground">${val ?? '—'}</span>`
		},
		{
			key: 'extracted_at',
			label: t('intel.extracted_at', 'Extracted At'),
			sortable: true,
			render: (val) => `<span class="text-muted-foreground text-xs">${formatTime(val as string)}</span>`
		},
		{
			key: 'status',
			label: t('intel.status', 'Status'),
			sortable: true,
			render: (val) =>
				`<span class="inline-flex items-center gap-1 rounded-full bg-amber-500/10 px-2 py-0.5 text-xs font-medium text-amber-500">${escapeHtml(val)}</span>`
		}
	];

	const rowActions = $derived<DataTableRowAction[]>([
		{
			label: t('intel.review_record', 'Review'),
			icon: Eye,
			onClick: (row) => goto(`/intelligence/review/${(row as unknown as IntelRecord).id}`)
		},
		{
			label: t('intel.approve', 'Approve'),
			icon: CheckCircle2,
			hidden: () => !canApprove || !statusIsReviewable,
			onClick: (row) => approveRecord((row as unknown as IntelRecord).id)
		},
		{
			label: t('intel.reject', 'Reject'),
			icon: XCircle,
			variant: 'destructive',
			hidden: () => !canReject || !statusIsReviewable,
			onClick: (row) => rejectRecord((row as unknown as IntelRecord).id)
		},
		{
			// Recovery path for records stuck in 'approved' when the promote
			// enqueue failed after approval — re-queues intel:promote.
			label: t('intel.retry_promote', 'Retry promote'),
			icon: RefreshCw,
			hidden: () => !canApprove || !statusIsApproved,
			onClick: (row) => repromoteRecord((row as unknown as IntelRecord).id)
		}
	]);

	const bulkActions = $derived<DataTableBulkAction[]>(
		statusIsReviewable
			? [
					...(canApprove
						? [{
								label: t('intel.approve_selected', 'Approve selected'),
								icon: CheckCircle2,
								onClick: (ids: string[]) => bulkApprove(ids)
							}]
						: []),
					...(canReject
						? [{
								label: t('intel.reject_selected', 'Reject selected'),
								icon: XCircle,
								onClick: (ids: string[]) => bulkReject(ids)
							}]
						: [])
				]
			: []
	);

	// doReviewAction is the shared core for row and bulk actions: it returns
	// the backend error string (or null on success) plus the promote_queued
	// flag the approve endpoint reports, so bulk callers can build a
	// per-item failure summary and approves can surface a queue outage.
	async function doReviewAction(
		id: string,
		action: 'approve' | 'reject' | 'promote'
	): Promise<{ err: string | null; promoteQueued: boolean }> {
		const path = action === 'promote'
			? `/api/v1/intel/records/${id}/promote`
			: `/api/v1/intel/review/${id}/${action}`;
		const result = await api<{ promote_queued?: boolean }>(path, { method: 'POST' });
		if (!result.ok) return { err: result.error, promoteQueued: false };
		return { err: null, promoteQueued: result.data?.promote_queued !== false };
	}

	async function approveRecord(id: string) {
		actionLoading = id;
		noticeMessage = '';
		const r = await doReviewAction(id, 'approve');
		actionLoading = null;
		if (!r.err) {
			if (!r.promoteQueued) {
				noticeMessage = t('intel.promote_not_queued', 'Approved, but the promotion queue is unreachable — use Retry promote once it recovers.');
			}
			await load();
		} else {
			errorMessage = r.err;
		}
	}

	async function rejectRecord(id: string) {
		actionLoading = id;
		const r = await doReviewAction(id, 'reject');
		actionLoading = null;
		if (!r.err) {
			await load();
		} else {
			errorMessage = r.err;
		}
	}

	async function repromoteRecord(id: string) {
		actionLoading = id;
		noticeMessage = '';
		const r = await doReviewAction(id, 'promote');
		actionLoading = null;
		if (!r.err) {
			noticeMessage = t('intel.repromote_queued', 'Promotion re-queued.');
			await load();
		} else {
			errorMessage = r.err;
		}
	}

	async function bulkReview(ids: string[], action: 'approve' | 'reject') {
		if (bulkBusy) return;
		bulkBusy = true;
		noticeMessage = '';
		errorMessage = '';
		const results = await Promise.allSettled(ids.map((id) => doReviewAction(id, action)));
		const failures: string[] = [];
		let unqueued = 0;
		results.forEach((r, i) => {
			if (r.status === 'rejected') {
				failures.push(ids[i]);
			} else if (r.value.err) {
				failures.push(`${ids[i].slice(0, 8)}: ${r.value.err}`);
			} else if (!r.value.promoteQueued) {
				unqueued++;
			}
		});
		const succeeded = ids.length - failures.length;
		if (failures.length === 0) {
			noticeMessage = t('intel.bulk_done', '{done} of {total} records updated.')
				.replace('{done}', String(succeeded))
				.replace('{total}', String(ids.length));
			if (unqueued > 0) {
				noticeMessage += ' ' + t('intel.promote_not_queued_bulk', '{n} approved record(s) could not reach the promotion queue — retry them from the Approved view.')
					.replace('{n}', String(unqueued));
			}
		} else {
			errorMessage = t('intel.bulk_partial', '{done} of {total} records updated; {failed} failed: {list}')
				.replace('{done}', String(succeeded))
				.replace('{total}', String(ids.length))
				.replace('{failed}', String(failures.length))
				.replace('{list}', failures.slice(0, 5).join('; '));
		}
		selectedIds = new Set();
		await load();
		bulkBusy = false;
	}

	async function bulkApprove(ids: string[]) {
		await bulkReview(ids, 'approve');
	}

	async function bulkReject(ids: string[]) {
		await bulkReview(ids, 'reject');
	}
</script>

<PageHeader
	title={statusFilter === 'review_required' ? t('intel.review_title', 'Review Queue') : `${statusLabels[statusFilter] ?? statusFilter} Records`}
	description={statusFilter === 'review_required'
		? t('intel.review_desc', 'Records extracted by the pipeline awaiting human moderation before publication.')
		: `Intelligence records with status: ${statusLabels[statusFilter] ?? statusFilter}`}
/>

{#if !canView}
	<AccessDenied message={t('intel.review_access_denied', 'You need intel.review.view permission to access the review queue.')} />
{:else}
	{#if errorMessage}
		<div class="bg-destructive/10 text-destructive mb-4 rounded-md p-3 text-sm">
			{errorMessage}
		</div>
	{/if}

	<div class="mb-4 flex flex-wrap items-center justify-between gap-3">
		<div class="flex flex-1 flex-wrap items-center gap-2">
			<SearchInput bind:value={filter} placeholder={t('intel.search_records', 'Search records...')} class="w-64" />
			<AutocompleteSelect
				value={filterEntity}
				onValueChange={(v) => { filterEntity = v; page = 0; }}
				options={entityItems}
				placeholder={t('intel.filter_all_entities', 'All entity types')}
				class="w-44"
			/>
			<!-- Status filter: updates the URL query param so the page reloads with the new status. -->
			<AutocompleteSelect
				value={statusFilter}
				onValueChange={(v) => {
					page = 0;
					const url = new URL(window.location.href);
					if (v === 'review_required') {
						url.searchParams.delete('status');
					} else {
						url.searchParams.set('status', v);
					}
					goto(url.pathname + url.search, { replaceState: false });
				}}
				options={Object.entries(statusLabels).map(([value, label]) => ({ value, label }))}
				class="w-44"
			/>
		</div>
		<button
			class="border-border-input bg-background-alt hover:bg-muted text-foreground flex items-center gap-1.5 rounded-9px border px-3 py-2 text-sm font-medium transition-colors disabled:opacity-50"
			disabled={refreshing}
			onclick={load}
		>
			<RefreshCw class="size-4 {refreshing ? 'animate-spin' : ''}" />
			{t('common.refresh', 'Refresh')}
		</button>
	</div>

	{#if noticeMessage}
		<div class="bg-emerald-500/10 text-emerald-500 mb-3 flex items-center gap-2 rounded-md p-2.5 text-xs">
			<CheckCircle2 class="size-3.5 shrink-0" />
			{noticeMessage}
		</div>
	{/if}

	<DataTable
		{columns}
		rows={records}
		{totalRows}
		bind:page
		bind:pageSize
		bind:sortKey
		bind:sortDirection
		bind:selectedIds
		{rowActions}
		{bulkActions}
		{loading}
		emptyMessage={t('intel.no_review_records', 'No records awaiting review')}
		emptyIcon={ClipboardList}
		onRowClick={(row) => goto(`/intelligence/review/${(row as unknown as IntelRecord).id}`)}
	/>
{/if}
