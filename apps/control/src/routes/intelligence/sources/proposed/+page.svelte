<script lang="ts">
	import { api, apiList } from '$lib/api';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import SearchInput from '$lib/components/ui/SearchInput.svelte';
	import AccessDenied from '$lib/components/AccessDenied.svelte';
	import ConfirmModal from '$lib/components/ui/ConfirmModal.svelte';
	import type { DataTableColumn, DataTableRowAction, DataTableBulkAction } from '$lib/api/types';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasAnyPermission } from '$lib/stores/auth.svelte';
	import { escapeHtml, isSafeUrl } from '$lib/utils';
	import { RefreshCw, Database, Eye, CheckCircle, XCircle } from 'lucide-svelte';
	import Modal from '$lib/components/ui/Modal.svelte';

	// ProbeReport mirrors probe.Report (intel-channels Phase 1, spec §2) —
	// emitted by sourceToFullEntry so reviewers can see what the probe
	// detected before approving the source.
	interface ProbeReport {
		final_url?: string;
		detected_kinds?: string[];
		methods?: { method: string; confidence: number; evidence?: string }[];
		channels?: {
			label: string;
			entry_url: string;
			crawl_method: string;
			entity_types?: string[];
			pipeline?: string;
			confidence?: number;
			relevance_topics?: string[];
		}[];
		jsonld_types?: string[];
		feed_urls?: string[];
		sitemap_urls?: string[];
		has_websub?: boolean;
		requires_js?: boolean;
		robots?: { fetched?: boolean; allowed?: boolean; crawl_delay_secs?: number };
		warnings?: string[];
		probed_at?: string;
	}

	interface ProposedSource {
		id: string;
		name: string;
		domain: string;
		source_url: string;
		category: string;
		categories?: string[] | null;
		keywords?: string[] | null;
		source_kind?: string | null;
		detected_methods?: string[] | null;
		probe_report?: ProbeReport | null;
		last_probed_at?: string | null;
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
		/** Resolved display name for proposed_by (display_name → username → UUID). */
		proposer_name?: string | null;
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
	let noticeMessage = $state('');
	let filter = $state('');

	let page = $state(0);
	let pageSize = $state(10);
	let sortKey = $state('proposed_at');
	let sortDirection = $state<'asc' | 'desc'>('desc');
	let selectedIds = $state(new Set<string>());
	let totalRows = $state(0);

	let detailOpen = $state(false);
	let detailSource = $state<ProposedSource | null>(null);
	let approving = $state<string | null>(null);
	let bulkBusy = $state(false);
	let rejectConfirm = $state<ProposedSource | null>(null);
	let rejectOpen = $state(false);

	function openReject(s: ProposedSource) {
		rejectConfirm = s;
		rejectOpen = true;
	}

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
			case 5: return t('intel.trust_unverified', 'Unverified');
			default: return t('intel.trust_unrated', 'Unrated');
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
		const params = new URLSearchParams();
		params.set('page_size', String(pageSize));
		params.set('page', String(page + 1)); // backend pagination is 1-based
		if (debouncedFilter) params.set('q', debouncedFilter);
		const result = await apiList<ProposedSource[]>(
			`/api/v1/intel/sources/proposed?${params.toString()}`
		);
		if (result.ok) {
			sources = Array.isArray(result.data) ? (result.data as ProposedSource[]) : [];
			totalRows = Number(result.meta?.total ?? sources.length);
		} else {
			errorMessage = result.error;
		}
		loading = false;
		refreshing = false;
	}

	// Server-side list: debounce the search box, reload on page/size/search.
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
		void page;
		void pageSize;
		void debouncedFilter;
		load();
	});

	// doProposedAction is the shared core for row and bulk approve/reject:
	// it returns the backend error string (or null on success) so bulk
	// callers can build a per-item failure summary.
	async function doProposedAction(id: string, action: 'approve' | 'reject'): Promise<string | null> {
		const result = await api<{ source_id: string; status: string; enabled: boolean }>(
			`/api/v1/intel/sources/${id}/${action}`,
			{ method: 'POST' }
		);
		return result.ok ? null : result.error;
	}

	async function approveSource(id: string) {
		approving = id;
		const err = await doProposedAction(id, 'approve');
		approving = null;
		if (!err) {
			await load();
		} else {
			errorMessage = err;
		}
	}

	async function confirmRejectSource() {
		if (!rejectConfirm) return;
		const err = await doProposedAction(rejectConfirm.id, 'reject');
		if (!err) {
			rejectConfirm = null;
			rejectOpen = false;
			await load();
		} else {
			errorMessage = err;
		}
	}

	async function bulkAction(ids: string[], action: 'approve' | 'reject') {
		if (bulkBusy) return;
		bulkBusy = true;
		noticeMessage = '';
		errorMessage = '';
		const results = await Promise.allSettled(ids.map((id) => doProposedAction(id, action)));
		const failures: string[] = [];
		results.forEach((r, i) => {
			if (r.status === 'rejected') {
				failures.push(ids[i].slice(0, 8));
			} else if (r.value) {
				failures.push(`${ids[i].slice(0, 8)}: ${r.value}`);
			}
		});
		const succeeded = ids.length - failures.length;
		if (failures.length === 0) {
			noticeMessage = t('intel.bulk_done', '{done} of {total} sources updated.')
				.replace('{done}', String(succeeded))
				.replace('{total}', String(ids.length));
		} else {
			errorMessage = t('intel.bulk_partial', '{done} of {total} sources updated; {failed} failed: {list}')
				.replace('{done}', String(succeeded))
				.replace('{total}', String(ids.length))
				.replace('{failed}', String(failures.length))
				.replace('{list}', failures.slice(0, 5).join('; '));
		}
		selectedIds = new Set();
		await load();
		bulkBusy = false;
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
				const raw = String(val ?? '');
				if (!isSafeUrl(raw)) return `<span class="font-mono text-xs">${escapeHtml(raw) || '—'}</span>`;
				const url = escapeHtml(raw);
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
				disabled: () => !!approving || bulkBusy
			});
			acts.push({
				label: t('intel.reject', 'Reject'),
				icon: XCircle,
				variant: 'destructive',
				onClick: (row) => openReject(row as unknown as ProposedSource),
				disabled: () => !!approving || bulkBusy
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
						onClick: (ids) => bulkAction(ids, 'approve')
					},
					{
						label: t('intel.reject_selected', 'Reject selected'),
						icon: XCircle,
						variant: 'destructive',
						onClick: (ids) => bulkAction(ids, 'reject')
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
	{#if noticeMessage}
		<div class="bg-emerald-500/10 text-emerald-500 mb-4 rounded-md p-3 text-sm">
			{noticeMessage}
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
		rows={sources}
		{totalRows}
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

	<Modal
		bind:open={detailOpen}
		title={detailSource?.name ?? t('intel.source_detail', 'Source detail')}
		description={detailSource?.domain ?? ''}
		size="max-w-2xl"
	>
		<div>

				{#if detailSource}
					<div class="mt-4 grid grid-cols-2 gap-4 text-sm">
						<div class="text-muted-foreground">{t('intel.field_category', 'Category')}</div>
						<div class="capitalize">
							{detailSource.categories?.length ? detailSource.categories.join(', ') : detailSource.category}
						</div>

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
						<div class="text-xs">
							{#if detailSource.proposer_name}
								{detailSource.proposer_name}
							{:else}
								<span class="font-mono">{detailSource.proposed_by ?? '—'}</span>
							{/if}
						</div>

						<div class="col-span-2 text-muted-foreground">{t('intel.source_url', 'URL')}</div>
						<div class="col-span-2">
							{#if isSafeUrl(detailSource.source_url)}
								<a href={detailSource.source_url} target="_blank" rel="noopener" class="text-primary hover:underline break-all">
									{detailSource.source_url}
								</a>
							{:else}
								<span class="font-mono text-xs break-all">{detailSource.source_url}</span>
							{/if}
						</div>

						{#if detailSource.keywords?.length}
							<div class="col-span-2 text-muted-foreground">{t('intel.keywords', 'Keywords')}</div>
							<div class="col-span-2 flex flex-wrap gap-1">
								{#each detailSource.keywords as kw}
									<span class="bg-muted rounded-full px-2 py-0.5 text-xs">{kw}</span>
								{/each}
							</div>
						{/if}

						{#if detailSource.last_error_message}
							<div class="col-span-2 text-muted-foreground">{t('intel.last_error', 'Last error')}</div>
							<div class="col-span-2 text-destructive text-xs">{detailSource.last_error_message}</div>
						{/if}
					</div>

					<!-- Probe report (intel-channels Phase 1): what the probe
						detected — methods, platform kinds, and the channel
						proposals it would materialize on verify. -->
					{#if detailSource.probe_report || (detailSource.detected_methods?.length ?? 0) > 0}
						{@const report = detailSource.probe_report}
						<div class="border-border-card mt-4 rounded-lg border p-4">
							<div class="flex items-baseline justify-between gap-2">
								<h3 class="text-foreground text-sm font-semibold">{t('intel.probe_report', 'Probe report')}</h3>
								{#if detailSource.last_probed_at}
									<span class="text-muted-foreground text-xs">
										{t('intel.last_probed', 'Last probed')}: {formatTime(detailSource.last_probed_at)}
									</span>
								{/if}
							</div>

							{#if (detailSource.detected_methods?.length ?? 0) > 0}
								<div class="mt-2">
									<span class="text-muted-foreground text-xs">{t('intel.detected_methods', 'Detected methods')}</span>
									<div class="mt-1 flex flex-wrap gap-1">
										{#each detailSource.detected_methods ?? [] as m}
											<span class="bg-muted rounded-full px-2 py-0.5 font-mono text-xs uppercase">{m}</span>
										{/each}
									</div>
								</div>
							{/if}

							{#if report}
								{#if report.detected_kinds?.length}
									<div class="mt-2 flex flex-wrap items-center gap-1">
										<span class="text-muted-foreground text-xs">{t('intel.detected_kinds', 'Platform signatures')}</span>
										{#each report.detected_kinds as k}
											<span class="rounded-full bg-violet-500/10 px-2 py-0.5 text-xs font-medium text-violet-500">{k}</span>
										{/each}
									</div>
								{/if}

								<div class="text-muted-foreground mt-2 flex flex-wrap gap-x-3 gap-y-1 text-xs">
									{#if report.requires_js}
										<span class="text-amber-500">{t('intel.requires_js', 'Requires JS rendering')}</span>
									{/if}
									{#if report.has_websub}
										<span>{t('intel.has_websub', 'WebSub hub advertised')}</span>
									{/if}
									{#if report.robots?.fetched}
										<span class={report.robots.allowed ? '' : 'text-destructive'}>
											{report.robots.allowed
												? t('intel.robots_allowed', 'robots.txt allows crawling')
												: t('intel.robots_disallowed', 'robots.txt disallows crawling')}
										</span>
									{/if}
									{#if report.feed_urls?.length}
										<span>{t('intel.feeds_found', '{n} feed(s) found').replace('{n}', String(report.feed_urls.length))}</span>
									{/if}
								</div>

								{#if report.channels?.length}
									<div class="mt-3">
										<span class="text-muted-foreground text-xs">{t('intel.proposed_channels', 'Proposed channels')}</span>
										<ul class="mt-1 space-y-1.5">
											{#each report.channels as ch}
												<li class="bg-background-alt rounded-md px-2.5 py-1.5 text-xs">
													<div class="flex flex-wrap items-center gap-1">
														<span class="font-medium">{ch.label}</span>
														<span class="bg-muted rounded-full px-1.5 py-0.5 font-mono uppercase">{ch.crawl_method}</span>
														{#if ch.pipeline}
															<span class="rounded-full bg-sky-500/10 px-1.5 py-0.5 text-sky-600">{ch.pipeline}</span>
														{/if}
														{#each ch.entity_types ?? [] as et}
															<span class="bg-muted rounded-full px-1.5 py-0.5">{et}</span>
														{/each}
														{#if ch.confidence != null}
															<span class="text-muted-foreground ms-auto">{Math.round(ch.confidence * 100)}%</span>
														{/if}
													</div>
													<div class="text-muted-foreground mt-0.5 truncate font-mono" title={ch.entry_url}>{ch.entry_url}</div>
												</li>
											{/each}
										</ul>
									</div>
								{/if}

								{#if report.warnings?.length}
									<div class="mt-3">
										<span class="text-muted-foreground text-xs">{t('intel.probe_warnings', 'Probe warnings')}</span>
										<ul class="mt-1 list-inside list-disc space-y-0.5 text-xs text-amber-500">
											{#each report.warnings as w}
												<li>{w}</li>
											{/each}
										</ul>
									</div>
								{/if}
							{/if}
						</div>
					{/if}
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
							class="border-destructive/40 text-destructive hover:bg-destructive/10 rounded-9px border px-4 py-2 text-sm font-medium transition-colors disabled:opacity-50"
							disabled={!!approving}
							onclick={() => { if (detailSource) { detailOpen = false; openReject(detailSource); } }}
						>
							{t('intel.reject', 'Reject')}
						</button>
						<button
							class="bg-foreground text-background hover:bg-foreground/90 rounded-9px px-4 py-2 text-sm font-medium transition-colors disabled:opacity-50"
							disabled={!!approving}
							onclick={() => { if (detailSource) { detailOpen = false; approveSource(detailSource.id); } }}
						>
							{t('intel.approve', 'Approve')}
						</button>
					{/if}
				</div>
		</div>
	</Modal>

	<ConfirmModal
		bind:open={rejectOpen}
		title={t('intel.reject_source', 'Reject Source')}
		consequence={t('intel.reject_consequence', 'This marks the proposal as rejected — it will not be crawled or appear in the directory. The row is kept for audit.')}
		confirmText={rejectConfirm?.name ?? ''}
		onConfirm={confirmRejectSource}
	/>
{/if}
