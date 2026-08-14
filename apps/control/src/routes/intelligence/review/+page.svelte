<script lang="ts">
	import { api } from '$lib/api';
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
	import { Select } from 'bits-ui';

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
	let filter = $state('');
	let filterEntity = $state('all');

	let page = $state(0);
	let pageSize = $state(10);
	let sortKey = $state('');
	let sortDirection = $state<'asc' | 'desc'>('asc');
	let selectedIds = $state(new Set<string>());

	let actionLoading = $state<string | null>(null);

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

	async function load() {
		refreshing = true;
		errorMessage = '';
		// When status is review_required, use the dedicated review queue
		// endpoint. For other statuses, use the generic records endpoint.
		const endpoint = statusFilter === 'review_required'
			? '/api/v1/intel/review/queue?page_size=100'
			: `/api/v1/intel/records?status=${statusFilter}&page_size=100`;
		const result = await api<IntelRecord[]>(endpoint);
		if (result.ok) {
			records = Array.isArray(result.data) ? (result.data as IntelRecord[]) : [];
		} else {
			errorMessage = result.error;
		}
		loading = false;
		refreshing = false;
	}

	// Reload when the status filter changes (URL query param changes).
	$effect(() => {
		// Track statusFilter so the effect re-runs when it changes.
		statusFilter;
		load();
	});

	const entityItems = $derived([
		{ value: 'all', label: t('intel.filter_all_entities', 'All entity types') },
		{ value: 'scholarship', label: t('intel.category_scholarship', 'Scholarship') },
		{ value: 'job', label: t('intel.category_job', 'Job') },
		{ value: 'visa', label: t('intel.category_visa', 'Visa') }
	]);

	let filtered = $derived(
		records.filter((r) => {
			if (filterEntity !== 'all' && r.entity_type !== filterEntity) return false;
			if (filter) {
				const q = filter.toLowerCase();
				if (
					!r.id?.toLowerCase().includes(q) &&
					!r.external_key?.toLowerCase().includes(q) &&
					!r.entity_type?.toLowerCase().includes(q)
				)
					return false;
			}
			return true;
		})
	);

	let totalRows = $derived(filtered.length);

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
		}
	]);

	const bulkActions = $derived<DataTableBulkAction[]>(
		canApprove && statusIsReviewable
			? [
					{
						label: t('intel.approve_selected', 'Approve selected'),
						icon: CheckCircle2,
						onClick: (ids) => bulkApprove(ids)
					}
				]
			: []
	);

	async function approveRecord(id: string) {
		actionLoading = id;
		const result = await api(`/api/v1/intel/review/${id}/approve`, { method: 'POST' });
		actionLoading = null;
		if (result.ok) {
			await load();
		} else {
			errorMessage = result.error;
		}
	}

	async function rejectRecord(id: string) {
		actionLoading = id;
		const result = await api(`/api/v1/intel/review/${id}/reject`, { method: 'POST' });
		actionLoading = null;
		if (result.ok) {
			await load();
		} else {
			errorMessage = result.error;
		}
	}

	async function bulkApprove(ids: string[]) {
		await Promise.allSettled(ids.map((id) => approveRecord(id)));
		await load();
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
			<Select.Root
				type="single"
				value={filterEntity}
				onValueChange={(v) => { if (v !== undefined) { filterEntity = v; page = 0; } }}
			>
				<Select.Trigger class="border-border-input bg-background text-foreground h-input rounded-9px border px-3 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus">
					<Select.Value placeholder={t('intel.filter_all_entities', 'All entity types')} />
				</Select.Trigger>
				<Select.Portal>
					<Select.Content class="border-border-input bg-background shadow-popover z-50 rounded-xl border">
						<Select.Viewport class="p-1">
							{#each entityItems as e}
								<Select.Item class="data-highlighted:bg-accent rounded-9px flex h-9 cursor-pointer items-center px-3 text-sm outline-none" value={e.value} label={e.label}>
									{e.label}
								</Select.Item>
							{/each}
						</Select.Viewport>
					</Select.Content>
				</Select.Portal>
			</Select.Root>
			<!-- Status filter: updates the URL query param so the page reloads with the new status. -->
			<Select.Root
				type="single"
				value={statusFilter}
				onValueChange={(v) => {
					if (v !== undefined) {
						const url = new URL(window.location.href);
						if (v === 'review_required') {
							url.searchParams.delete('status');
						} else {
							url.searchParams.set('status', v);
						}
						goto(url.pathname + url.search, { replaceState: false });
					}
				}}
			>
				<Select.Trigger class="border-border-input bg-background text-foreground h-input rounded-9px border px-3 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus">
					<span class="truncate">{statusLabels[statusFilter] ?? statusFilter}</span>
				</Select.Trigger>
				<Select.Portal>
					<Select.Content class="border-border-input bg-background shadow-popover z-50 rounded-xl border">
						<Select.Viewport class="p-1">
							{#each Object.entries(statusLabels) as [value, label]}
								<Select.Item class="data-highlighted:bg-accent rounded-9px flex h-9 cursor-pointer items-center px-3 text-sm outline-none" {value} {label}>
									{label}
								</Select.Item>
							{/each}
						</Select.Viewport>
					</Select.Content>
				</Select.Portal>
			</Select.Root>
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

	<DataTable
		{columns}
		rows={filtered}
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
