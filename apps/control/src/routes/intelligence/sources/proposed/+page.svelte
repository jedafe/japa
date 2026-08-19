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
	import { RefreshCw, Database, Eye, CheckCircle, Search } from 'lucide-svelte';
	import { Dialog } from 'bits-ui';

	interface ProposedSource {
		id: string;
		name: string;
		domain: string;
		source_url: string;
		category: string;
		source_type: string;
		trust_level: number;
		crawl_method: string;
		crawl_frequency: string;
		crawl_frequency_mins: number;
		attribution_required: boolean;
		enabled: boolean;
		status: string;
		proposed_at?: string | null;
		proposed_by?: string | null;
		approved_at?: string | null;
		approved_by?: string | null;
		last_crawled_at?: string | null;
		last_success_at?: string | null;
		last_error_at?: string | null;
		last_error_message?: string | null;
		consecutive_failures?: number;
		created_at?: string;
	}

	let {} = $props();

	const canManage = $derived(hasAnyPermission(['intel.sources.manage', 'intel.sources.*']));

	let sources = $state<ProposedSource[]>([]);
	let loading = $state(true);
	let refreshing = $state(false);
	let errorMessage = $state('');
	let filter = $state('');

	let page = $state(0);
	let pageSize = $state(10);
	let sortKey = $state('proposed_at');
	let sortDirection = $state<'asc' | 'desc'>('desc');
	let selectedIds = $state(new Set<string>());

	let detailOpen = $state(false);
	let detailSource = $state<ProposedSource | null>(null);
	let approving = $state<string | null>(null);
	let bulkApproving = $state(false);

	function trustColor(level: number): string {
		switch (level) {
			case 1: return 'text-emerald-500 bg-emerald-500/10';
			case 2: return 'text-sky-500 bg-sky-500/10';
			case 3: return 'text-amber-500 bg-amber-500/10';
			case 4: return 'text-muted-foreground bg-muted';
			default: return 'text-muted-foreground bg-muted';
		}
	}

	function trustLabel(level: number): string {
		switch (level) {
			case 1: return t('intel.trust_official', 'Official');
			case 2: return t('intel.trust_trusted', 'Trusted');
			case 3: return t('intel.trust_aggregator', 'Aggregator');
			case 4: return t('intel.trust_community', 'Community');
			default: return String(level);
		}
	}

	function formatTime(ts?: string | null): string {
		if (!ts) return '—';
		try {
			return new Date(ts).toLocaleString();
		} catch {
			return ts;
		}
	}

	async function load() {
		refreshing = true;
		errorMessage = '';
		const result = await api<ProposedSource[]>(
			'/api/v1/intel/sources/proposed?page_size=100'
		);
		if (result.ok) {
			sources = Array.isArray(result.data) ? (result.data as ProposedSource[]) : [];
		} else {
			errorMessage = result.error;
		}
		loading = false;
		refreshing = false;
	}

	$effect(() => {
		load();
	});

	let filtered = $derived(
		sources.filter((s) => {
			if (!filter) return true;
			const q = filter.toLowerCase();
			return (
				s.name?.toLowerCase().includes(q) ||
				s.domain?.toLowerCase().includes(q) ||
				s.source_url?.toLowerCase().includes(q) ||
				s.category?.toLowerCase().includes(q)
			);
		})
	);

	async function approveSource(id: string) {
		approving = id;
		const result = await api<{ source_id: string; status: string; enabled: boolean }>(
			`/api/v1/intel/sources/${id}/approve`,
			{ method: 'POST' }
		);
		approving = null;
		if (result.ok) {
			await load();
		} else {
			errorMessage = result.error;
		}
	}

	async function bulkApprove(ids: string[]) {
		bulkApproving = true;
		await Promise.allSettled(ids.map((id) => approveSource(id)));
		selectedIds = new Set();
		bulkApproving = false;
	}

	function openDetail(s: ProposedSource) {
		detailSource = s;
		detailOpen = true;
	}

	function closeDetail() {
		detailOpen = false;
		detailSource = null;
	}

	const columns: DataTableColumn[] = [
		{
			key: 'name',
			label: t('intel.field_name', 'Name'),
			sortable: true,
			render: (_val, row) =>
				`<div class="flex flex-col"><span class="font-medium">${escapeHtml(row.name as string)}</span>` +
				`<span class="text-muted-foreground text-xs font-mono">${escapeHtml((row as unknown as ProposedSource).domain)}</span></div>`
		},
		{
			key: 'category',
			label: t('intel.field_category', 'Category'),
			sortable: true,
			render: (val) => `<span class="text-muted-foreground capitalize">${escapeHtml(val as string)}</span>`
		},
		{
			key: 'trust_level',
			label: t('intel.field_trust_level', 'Trust'),
			sortable: true,
			render: (val) =>
				`<span class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ${trustColor(Number(val))}">${trustLabel(Number(val))}</span>`
		},
		{
			key: 'proposed_at',
			label: t('intel.proposed_at', 'Proposed'),
			sortable: true,
			render: (val) => `<span class="text-muted-foreground text-xs">${formatTime(val as string)}</span>`
		},
		{
			key: 'source_url',
			label: t('intel.source_url', 'URL'),
			sortable: true,
			render: (val) => {
				const url = escapeHtml(val as string);
				return `<a href="${url}" target="_blank" rel="noopener" class="text-primary hover:underline text-xs truncate max-w-[16rem] inline-block">${url}</a>`;
			}
		}
	];

	const rowActions = $derived.by<DataTableRowAction[]>(() => {
		const acts: DataTableRowAction[] = [];
		if (canManage) {
			acts.push({
				label: t('intel.approve', 'Approve'),
				icon: CheckCircle,
				onClick: (row) => approveSource((row as unknown as ProposedSource).id),
				disabled: () => !!approving
			});
		}
		acts.push({
			label: t('common.view', 'View'),
			icon: Eye,
			onClick: (row) => openDetail(row as unknown as ProposedSource)
		});
		return acts;
	});

	const bulkActions = $derived<DataTableBulkAction[]>(
		canManage
			? [
					{
						label: t('intel.approve_selected', 'Approve selected'),
						icon: CheckCircle,
						onClick: (ids) => bulkApprove(ids)
					}
				]
			: []
	);
</script>

<PageHeader
	title={t('intel.proposed_sources_title', 'Proposed Sources')}
	description={t('intel.proposed_sources_desc', 'Sources discovered by the agent awaiting admin review and approval.')}
/>

{#if !canManage}
	<AccessDenied message={t('intel.proposed_sources_access_denied', 'You need intel.sources.manage permission to review proposed sources.')} />
{:else}
	{#if errorMessage}
		<div class="bg-destructive/10 text-destructive mb-4 rounded-md p-3 text-sm">
			{errorMessage}
		</div>
	{/if}

	<div class="mb-4 flex flex-wrap items-center justify-between gap-3">
		<div class="flex flex-1 flex-wrap items-center gap-2">
			<SearchInput bind:value={filter} placeholder={t('intel.search_proposed', 'Search proposed sources...')} class="w-64" />
		</div>
		<div class="flex items-center gap-2">
			<button
				class="border-border-input bg-background-alt hover:bg-muted text-foreground flex items-center gap-1.5 rounded-9px border px-3 py-2 text-sm font-medium transition-colors disabled:opacity-50"
				disabled={refreshing}
				onclick={load}
			>
				<RefreshCw class="size-4 {refreshing ? 'animate-spin' : ''}" />
				{t('common.refresh', 'Refresh')}
			</button>
		</div>
	</div>

	<DataTable
		{columns}
		rows={filtered}
		totalRows={filtered.length}
		bind:page
		bind:pageSize
		bind:sortKey
		bind:sortDirection
		bind:selectedIds
		{rowActions}
		{bulkActions}
		{loading}
		emptyMessage={t('intel.no_proposed_sources', 'No proposed sources. Discover sources with the source.discover agent tool.')}
		emptyIcon={Database}
	/>

	<Dialog.Root bind:open={detailOpen}>
		<Dialog.Portal>
			<Dialog.Overlay class="bg-black/50 fixed inset-0 z-40" />
			<Dialog.Content class="bg-background border-border-card fixed top-1/2 left-1/2 z-50 w-full max-w-2xl -translate-x-1/2 -translate-y-1/2 rounded-xl border p-6 shadow-lg max-h-[90vh] overflow-y-auto">
				<Dialog.Title class="text-foreground text-lg font-semibold">
					{detailSource?.name ?? t('intel.source_detail', 'Source detail')}
				</Dialog.Title>
				<Dialog.Description class="text-muted-foreground mt-1 text-sm">
					{detailSource?.domain ?? ''}
				</Dialog.Description>

				{#if detailSource}
					<div class="mt-4 grid grid-cols-2 gap-4 text-sm">
						<div class="text-muted-foreground">{t('intel.field_category', 'Category')}</div>
						<div class="capitalize">{detailSource.category}</div>

						<div class="text-muted-foreground">{t('intel.field_source_type', 'Type')}</div>
						<div class="capitalize">{detailSource.source_type}</div>

						<div class="text-muted-foreground">{t('intel.field_trust_level', 'Trust')}</div>
						<div class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium w-fit {trustColor(detailSource.trust_level)}">
							{trustLabel(detailSource.trust_level)}
						</div>

						<div class="text-muted-foreground">{t('intel.field_crawl_method', 'Method')}</div>
						<div class="font-mono text-xs uppercase">{detailSource.crawl_method}</div>

						<div class="text-muted-foreground">{t('intel.field_crawl_frequency', 'Frequency')}</div>
						<div class="capitalize">{detailSource.crawl_frequency}</div>

						<div class="text-muted-foreground">{t('intel.proposed_at', 'Proposed')}</div>
						<div>{formatTime(detailSource.proposed_at)}</div>

						<div class="text-muted-foreground">{t('intel.proposed_by', 'Proposed by')}</div>
						<div class="font-mono text-xs">{detailSource.proposed_by ?? '—'}</div>

						<div class="col-span-2 text-muted-foreground">{t('intel.source_url', 'URL')}</div>
						<div class="col-span-2">
							<a href={detailSource.source_url} target="_blank" rel="noopener" class="text-primary hover:underline break-all">
								{detailSource.source_url}
							</a>
						</div>

						{#if detailSource.last_error_message}
							<div class="col-span-2 text-muted-foreground">{t('intel.last_error', 'Last error')}</div>
							<div class="col-span-2 text-destructive text-xs">{detailSource.last_error_message}</div>
						{/if}
					</div>
				{/if}

				<div class="mt-6 flex justify-end gap-2">
					<button
						class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-9px border px-4 py-2 text-sm font-medium transition-colors"
						onclick={closeDetail}
					>
						{t('common.close', 'Close')}
					</button>
					{#if canManage && detailSource}
						<button
							class="bg-foreground text-background hover:bg-foreground/90 rounded-9px px-4 py-2 text-sm font-medium transition-colors disabled:opacity-50"
							disabled={!!approving}
							onclick={() => { if (detailSource) { detailOpen = false; approveSource(detailSource.id); } }}
						>
							{t('intel.approve', 'Approve')}
						</button>
					{/if}
				</div>
			</Dialog.Content>
		</Dialog.Portal>
	</Dialog.Root>
{/if}
