<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import MetricCard from '$lib/components/MetricCard.svelte';
	import SearchInput from '$lib/components/ui/SearchInput.svelte';
	import ConfirmModal from '$lib/components/ui/ConfirmModal.svelte';
	import Modal from '$lib/components/ui/Modal.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import Tabs from '$lib/components/ui/Tabs.svelte';
	import TabsContent from '$lib/components/ui/TabsContent.svelte';
	import { api } from '$lib/api';
	import { t, translations } from '$lib/stores/i18n.svelte';
	import { escapeHtml } from '$lib/utils';
	import { Plus, ShieldAlert, ShieldCheck, Trash2, CheckCircle } from 'lucide-svelte';
	import type { DataTableColumn, DataTableRowAction, DataTableBulkAction } from '$lib/api/types';

	type ScanResult = {
		id: string;
		severity: 'info' | 'warning' | 'critical';
		title: string;
		description: string;
		created_at: string;
		resolved: boolean;
	};

	type IpBlock = {
		id: string;
		cidr: string;
		block_type: string;
		reason: string | null;
		blocked_by: string;
		expires_at: string | null;
		created_at: string;
	};

	let {} = $props();

	let activeTab = $state('scan-findings');

	let results = $state<ScanResult[]>([]);
	let totalPages = $state(1);
	let totalResults = $state(0);
	let scanPage = $state(0);
	// The backend paginates scan findings at a fixed page size of 20
	// (ListSecurityScanResults) — the page-size selector is locked to it.
	let scanPageSize = $state(20);
	let scanLoading = $state(true);
	let apiAvailable = $state(true);

	let activeIpBlocks = $state(0);
	let overviewLoading = $state(true);

	let filterSeverity = $state('all');
	let searchQuery = $state('');

	let ipBlocks = $state<IpBlock[]>([]);
	let ipBlocksLoading = $state(true);
	let ipSearchQuery = $state('');
	let ipSortKey = $state('');
	let ipSortDirection = $state<'asc' | 'desc'>('asc');
	let ipPage = $state(0);
	let ipPageSize = $state(10);
	// H6: Bulk selection state for IP blocks
	let selectedIpIds = $state(new Set<string>());
	let confirmBulkDeleteIpOpen = $state(false);
	let pendingBulkIpIds = $state<string[]>([]);
	// L4: Bulk selection state for scan findings
	let selectedScanIds = $state(new Set<string>());
	// Scan finding delete confirmation
	let showDeleteScanConfirm = $state(false);
	let deleteScanTarget = $state<ScanResult | null>(null);
	let pendingBulkScanIds = $state<string[]>([]);
	let showBulkDeleteScanConfirm = $state(false);

	let showCreateModal = $state(false);
	let cidr = $state('');
	let blockType = $state('full');
	let reason = $state('');
	let createLoading = $state(false);
	let createError = $state('');

	let showDeleteConfirm = $state(false);
	let deleteTarget = $state<IpBlock | null>(null);
	let deleteLoading = $state(false);
	let deleteError = $state('');

	let scanRunning = $state(false);
	let scanMessage = $state('');
	let scanMessageType = $state<'success' | 'error' | ''>('');
	// Post-scan refresh: a single delayed reload so the user sees results
	// without waiting for the 30s background interval. The background
	// interval handles the case where the Asynq worker is slower than this.
	let scanRefreshTimer: ReturnType<typeof setTimeout> | null = null;
	const SCAN_REFRESH_DELAY_MS = 8000;

	let blockTypeLabels = $derived.by<Record<string, string>>(() => {
		translations();
		return {
		full: t('security.block_full'),
		registration: t('security.block_registration'),
		posting: t('security.block_posting'),
		api: t('security.block_api')
	};

	});

	let severityItems = $derived.by(() => {
		translations();
		return [
		{ value: 'all', label: t('security.severity_all') },
		{ value: 'info', label: t('security.severity_info') },
		{ value: 'warning', label: t('security.severity_warning') },
		{ value: 'critical', label: t('security.severity_critical') }
	];

	});

	let blockTypeItems = $derived.by(() => {
		translations();
		return [
		{ value: 'full', label: t('security.block_full') },
		{ value: 'registration', label: t('security.block_registration') },
		{ value: 'posting', label: t('security.block_posting') },
		{ value: 'api', label: t('security.block_api') }
	];

	});

	const severityClasses: Record<string, string> = {
		critical: 'bg-destructive/15 text-destructive',
		warning: 'bg-amber-500/15 text-amber-500',
		info: 'bg-blue-500/15 text-blue-500'
	};

	let scanColumns = $derived.by<DataTableColumn[]>(() => {
		translations();
		return [
		{
			key: 'severity',
			label: t('security.severity'),
			sortable: true,
			render: (val) => {
				const sev = escapeHtml(val);
				const cls = severityClasses[sev] ?? 'bg-muted text-foreground';
				return `<span class="rounded-full px-2 py-0.5 text-xs font-medium ${cls}">${sev}</span>`;
			}
		},
		{ key: 'title', label: t('security.title_label'), sortable: true, class: 'font-medium' },
		{
			key: 'description',
			label: t('security.description_label'),
			class: 'text-muted-foreground max-w-xs truncate text-xs'
		},
		{
			key: 'created_at',
			label: t('security.found'),
			sortable: true,
			render: (val) => `<span>${escapeHtml(formatDate(String(val)))}</span>`
		},
		{
			key: 'resolved',
			label: t('common.status'),
			render: (val) =>
				val
					? `<span class="bg-emerald-500/15 text-emerald-600 rounded-full px-2 py-0.5 text-xs font-medium">${t('security.resolved')}</span>`
					: `<span class="bg-amber-500/15 text-amber-500 rounded-full px-2 py-0.5 text-xs font-medium">${t('security.open')}</span>`
		}
	];

	});

	let scanRowActions = $derived.by<DataTableRowAction[]>(() => {
		translations();
		return [
		{
			label: t('security.resolve', 'Resolve'),
			icon: CheckCircle,
			variant: 'default',
			onClick: (row) => resolveScanResult((row as unknown as ScanResult).id),
			hidden: (row) => (row as unknown as ScanResult).resolved
		},
		{
			label: t('common.delete', 'Delete'),
			icon: Trash2,
			variant: 'destructive',
			onClick: (row) => openDeleteScanConfirm(row as unknown as ScanResult)
		}
	];
	});

	// L4: Bulk actions for scan findings
	let scanBulkActions = $derived.by<DataTableBulkAction[]>(() => {
		translations();
		return [
		{
			label: t('security.resolve_selected', 'Resolve Selected'),
			icon: CheckCircle,
			onClick: async (ids: string[]) => {
				await Promise.allSettled(ids.map((id) => resolveScanResult(id)));
				selectedScanIds = new Set();
			}
		},
		{
			label: t('security.delete_selected', 'Delete Selected'),
			icon: Trash2,
			variant: 'destructive',
			onClick: (ids: string[]) => {
				pendingBulkScanIds = ids;
				showBulkDeleteScanConfirm = true;
			}
		}
	];
	});

	let ipBlockColumns = $derived.by<DataTableColumn[]>(() => {
		translations();
		return [
		{
			key: 'cidr',
			label: t('security.ip_cidr'),
			sortable: true,
			class: 'font-mono text-xs'
		},
		{
			key: 'block_type',
			label: t('security.block_type'),
			sortable: true,
			render: (val) => {
				const label = escapeHtml(blockTypeLabels[String(val)] ?? String(val));
				return `<span class="bg-destructive/10 text-destructive inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium"><span class="mr-1">⚠</span>${label}</span>`;
			}
		},
		{
			key: 'reason',
			label: t('security.reason'),
			render: (val) => `<span class="text-muted-foreground text-xs">${val ? escapeHtml(val) : '—'}</span>`
		},
		{
			key: 'created_at',
			label: t('security.created'),
			sortable: true,
			render: (val) => `<span class="text-muted-foreground text-xs">${escapeHtml(formatDate(String(val)))}</span>`
		}
	];

	});

	let ipRowActions = $derived.by<DataTableRowAction[]>(() => {
		translations();
		return [
		{
			label: t('common.delete'),
			icon: Trash2,
			variant: 'destructive',
			onClick: (row) => openDeleteConfirm(row as unknown as IpBlock)
		}
	];

	});

	// H6: Bulk actions for IP blocks
	let ipBulkActions = $derived.by<DataTableBulkAction[]>(() => {
		translations();
		return [
		{
			label: t('security.delete_selected', 'Delete Selected'),
			icon: Trash2,
			variant: 'destructive',
			onClick: (ids: string[]) => {
				pendingBulkIpIds = ids;
				confirmBulkDeleteIpOpen = true;
			}
		}
	];

	});

	async function confirmBulkDeleteIp() {
		const ids = pendingBulkIpIds;
		const results = await Promise.allSettled(
			ids.map((id) => api(`/control/security/ip-blocks/${id}`, { method: 'DELETE' }))
		);
		const failed = results.filter((r) => r.status === 'rejected').length;
		if (failed > 0) {
			deleteError = t('security.bulk_delete_partial_error', `{failed} of ${ids.length} deletes failed.`).replace('{failed}', String(failed));
		}
		selectedIpIds = new Set();
		pendingBulkIpIds = [];
		confirmBulkDeleteIpOpen = false;
		await loadIpBlocks();
	}

	let scanSortKey = $state('');
	let scanSortDirection = $state<'asc' | 'desc'>('asc');

	let filteredResults = $derived.by(() => {
		// Severity is filtered server-side; this derivation only applies the
		// within-page search query and column sort on the current page slice.
		let list = results;
		if (searchQuery.trim()) {
			const q = searchQuery.toLowerCase();
			list = list.filter(
				(r) =>
					r.title.toLowerCase().includes(q) ||
					r.description.toLowerCase().includes(q)
			);
		}
		if (scanSortKey) {
			list = [...list].sort((a, b) => {
				const av = (a as Record<string, unknown>)[scanSortKey] ?? '';
				const bv = (b as Record<string, unknown>)[scanSortKey] ?? '';
				const cmp = String(av).localeCompare(String(bv));
				return scanSortDirection === 'asc' ? cmp : -cmp;
			});
		}
		return list;
	});

	let filteredIpBlocks = $derived.by(() => {
		let list = ipBlocks;
		if (ipSearchQuery.trim()) {
			const q = ipSearchQuery.toLowerCase();
			list = list.filter(
				(b) =>
					b.cidr.toLowerCase().includes(q) ||
					b.block_type.toLowerCase().includes(q) ||
					(b.reason ?? '').toLowerCase().includes(q)
			);
		}
		if (ipSortKey) {
			list = [...list].sort((a, b) => {
				const av = (a as Record<string, unknown>)[ipSortKey] ?? '';
				const bv = (b as Record<string, unknown>)[ipSortKey] ?? '';
				const cmp = String(av).localeCompare(String(bv));
				return ipSortDirection === 'asc' ? cmp : -cmp;
			});
		}
		return list;
	});

	async function loadIpBlocks(quiet = false) {
		if (!quiet) ipBlocksLoading = true;
		try {
			const result = await api<IpBlock[]>('/control/security/ip-blocks');
			if (result.ok) {
				ipBlocks = Array.isArray(result.data) ? result.data : [];
				apiAvailable = true;
			} else {
				apiAvailable = false;
			}
		} catch {
			apiAvailable = false;
		} finally {
			ipBlocksLoading = false;
		}
	}

	async function loadOverview(quiet = false) {
		if (!quiet) overviewLoading = true;
		try {
			const result = await api<{ active_ip_blocks: number }>('/control/security');
			if (result.ok && result.data) {
				activeIpBlocks = result.data.active_ip_blocks ?? 0;
				apiAvailable = true;
			} else {
				apiAvailable = false;
			}
		} catch {
			apiAvailable = false;
		} finally {
			overviewLoading = false;
		}
	}

	$effect(() => {
		loadScanResults();
		loadIpBlocks();
		loadOverview();

		// Background refresh every 2h — keeps the page from going stale when
		// left open, without hammering the API on a short interval. The
		// post-scan refresh (scheduleScanRefresh) handles the "Run Scan" case.
		const interval = setInterval(() => {
			if (document.visibilityState === 'visible') {
				loadScanResults(true);
				loadIpBlocks(true);
				loadOverview(true);
			}
		}, 2 * 60 * 60 * 1000);

		const onVisible = () => {
			if (document.visibilityState === 'visible') {
				loadScanResults(true);
				loadIpBlocks(true);
				loadOverview(true);
			}
		};
		document.addEventListener('visibilitychange', onVisible);

		return () => {
			clearInterval(interval);
			document.removeEventListener('visibilitychange', onVisible);
			cancelScanRefresh();
		};
	});

	// Re-fetch scan findings whenever the server-side page or severity
	// filter changes. Tracked separately from the mount effect so paging
	// does not re-create the background-refresh interval.
	let lastScanPage = $state(0);
	let lastScanSeverity = $state('all');
	$effect(() => {
		if (scanPage !== lastScanPage || filterSeverity !== lastScanSeverity) {
			lastScanPage = scanPage;
			lastScanSeverity = filterSeverity;
			loadScanResults();
		}
	});

	// quiet=true refreshes in the background without flashing the table
	// skeleton (used by the 2h interval and visibility-refocus poll).
	async function loadScanResults(quiet = false) {
		if (!quiet) scanLoading = true;
		const sev = filterSeverity !== 'all' ? `&severity=${filterSeverity}` : '';
		try {
			const result = await api<{ results: ScanResult[]; page: number; total?: number; total_pages: number }>(
				`/control/security/scan?page=${scanPage}${sev}`
			);
			if (result.ok) {
				results = result.data.results ?? [];
				totalPages = result.data.total_pages ?? 1;
				// `total` is returned by newer backends; fall back to the
				// page-count upper bound when absent.
				totalResults = result.data.total ?? totalPages * 20;
				apiAvailable = true;
			} else {
				apiAvailable = false;
			}
		} catch {
			apiAvailable = false;
		} finally {
			scanLoading = false;
		}
	}

	function applySeverityFilter() {
		// The page/severity watcher effect performs the refetch — just reset
		// to the first page here.
		scanPage = 0;
	}

	async function triggerScan() {
		scanRunning = true;
		scanMessage = '';
		scanMessageType = '';
		try {
			const result = await api<{ task_id: string; status: string }>('/control/security/scan', {
				method: 'POST'
			});
			if (result.ok) {
				scanMessage = t('security.scan_enqueued', 'Scan enqueued. Results will appear shortly.');
				scanMessageType = 'success';
				// Single delayed refresh — gives the Asynq worker time to process
				// without looping. The 30s background interval covers slower workers.
				scheduleScanRefresh();
			} else {
				scanMessage = result.error || t('security.scan_error', 'Failed to trigger scan');
				scanMessageType = 'error';
			}
		} catch {
			scanMessage = t('security.scan_error', 'Failed to trigger scan');
			scanMessageType = 'error';
		} finally {
			scanRunning = false;
		}
	}

	function scheduleScanRefresh() {
		cancelScanRefresh();
		scanRefreshTimer = setTimeout(() => loadScanResults(true), SCAN_REFRESH_DELAY_MS);
	}

	function cancelScanRefresh() {
		if (scanRefreshTimer !== null) {
			clearTimeout(scanRefreshTimer);
			scanRefreshTimer = null;
		}
	}

	async function resolveScanResult(id: string) {
		try {
			const result = await api(`/control/security/${id}/resolve`, { method: 'POST' });
			if (result.ok) {
				results = results.map((r) => (r.id === id ? { ...r, resolved: true } : r));
			} else {
				scanMessage = result.error || t('security.resolve_error', 'Failed to resolve finding.');
				scanMessageType = 'error';
			}
		} catch {
			scanMessage = t('security.resolve_error', 'Failed to resolve finding.');
			scanMessageType = 'error';
		}
	}

	function openDeleteScanConfirm(finding: ScanResult) {
		deleteScanTarget = finding;
		showDeleteScanConfirm = true;
	}

	function closeDeleteScanConfirm() {
		showDeleteScanConfirm = false;
		deleteScanTarget = null;
	}

	async function deleteScanResult() {
		if (!deleteScanTarget) return;
		try {
			const result = await api(`/control/security/${deleteScanTarget.id}`, { method: 'DELETE' });
			if (result.ok) {
				results = results.filter((r) => r.id !== deleteScanTarget!.id);
				selectedScanIds.delete(deleteScanTarget.id);
				selectedScanIds = new Set(selectedScanIds);
				closeDeleteScanConfirm();
			} else {
				scanMessage = result.error || t('security.delete_finding_error', 'Failed to delete finding.');
				scanMessageType = 'error';
			}
		} catch {
			scanMessage = t('security.delete_finding_error', 'Failed to delete finding.');
			scanMessageType = 'error';
		}
	}

	async function confirmBulkDeleteScan() {
		const ids = pendingBulkScanIds;
		const settled = await Promise.allSettled(
			ids.map((id) => api(`/control/security/${id}`, { method: 'DELETE' }))
		);
		const failed = settled.filter((r) => r.status === 'rejected').length;
		const deletedIds = new Set(
			ids.filter((_, i) => settled[i].status === 'fulfilled' && (settled[i] as PromiseFulfilledResult<{ ok: boolean }>).value.ok)
		);
		results = results.filter((r) => !deletedIds.has(r.id));
		if (failed > 0) {
			scanMessage = t('security.bulk_delete_partial_error', `{failed} of ${ids.length} deletes failed.`)
				.replace('{failed}', String(failed))
				.replace('{count}', String(ids.length));
			scanMessageType = 'error';
		}
		selectedScanIds = new Set();
		pendingBulkScanIds = [];
		showBulkDeleteScanConfirm = false;
	}

	function openCreateModal() {
		cidr = '';
		blockType = 'full';
		reason = '';
		createError = '';
		showCreateModal = true;
	}

	function closeCreateModal() {
		showCreateModal = false;
		createError = '';
	}

	async function createIpBlock() {
		if (!cidr.trim()) {
			createError = t('security.cidr_required', 'CIDR is required.');
			return;
		}
		// Basic CIDR validation
		if (!/^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\/\d{1,2}$/.test(cidr.trim())) {
			createError = t('security.invalid_cidr', 'Please enter a valid CIDR (e.g. 192.168.1.0/24).');
			return;
		}
		createLoading = true;
		createError = '';
		try {
			const result = await api<IpBlock>('/control/security/ip-blocks', {
				method: 'POST',
				body: {
					cidr: cidr.trim(),
					block_type: blockType,
					reason: reason.trim() || undefined
				}
			});
			if (result.ok) {
				ipBlocks = [result.data, ...ipBlocks];
				closeCreateModal();
			} else {
				createError = t('security.create_block_error');
			}
		} catch {
			createError = t('security.create_block_error');
		} finally {
			createLoading = false;
		}
	}

	function openDeleteConfirm(block: IpBlock) {
		deleteTarget = block;
		deleteError = '';
		showDeleteConfirm = true;
	}

	function closeDeleteConfirm() {
		showDeleteConfirm = false;
		deleteTarget = null;
		deleteError = '';
	}

	async function deleteIpBlock() {
		if (!deleteTarget) return;
		deleteLoading = true;
		deleteError = '';
		try {
			const result = await api(`/control/security/ip-blocks/${deleteTarget.id}`, {
				method: 'DELETE'
			});
			if (result.ok) {
				ipBlocks = ipBlocks.filter((b) => b.id !== deleteTarget!.id);
				closeDeleteConfirm();
			} else {
				deleteError = t('security.delete_block_error');
			}
		} catch {
			deleteError = t('security.delete_block_error');
		} finally {
			deleteLoading = false;
		}
	}

	function formatDate(iso: string): string {
		try {
			return new Date(iso).toLocaleDateString('en-US', {
				year: 'numeric',
				month: 'short',
				day: 'numeric'
			});
		} catch {
			return iso;
		}
	}
</script>

<PageHeader title={t('security.title')} description={t('security.description')} />

{#if !apiAvailable}
	<div class="border-tertiary bg-tertiary/10 text-tertiary mb-4 flex items-center gap-2 rounded-lg border px-4 py-3 text-sm">
		{t('security.unavailable')}
	</div>
{/if}

<div class="mb-4 grid gap-3 sm:grid-cols-3">
	<MetricCard label={t('security.active_ip_blocks', 'Active IP Blocks')} value={activeIpBlocks} />
	<MetricCard label={t('security.scan_findings', 'Scan Findings')} value={totalResults} />
	<MetricCard label={t('security.ip_blocks', 'IP Blocks')} value={ipBlocks.length} />
</div>

<Tabs
	bind:value={activeTab}
	tabs={[
		{ value: 'scan-findings', label: t('security.scan_findings') },
		{ value: 'ip-blocks', label: t('security.ip_blocks'), badge: ipBlocks.length }
	]}
>
	<TabsContent value="scan-findings">
		<div class="mb-4 flex items-center gap-3">
			<AutocompleteSelect
				bind:value={filterSeverity}
				options={severityItems}
				onValueChange={() => applySeverityFilter()}
				placeholder={t('security.severity_all')}
				ariaLabel={t('security.filter_severity')}
				class="w-44"
			/>
			<SearchInput bind:value={searchQuery} placeholder={t('security.search_findings', 'Search findings...')} class="w-64" />
			<div class="flex-1"></div>
			<button
				type="button"
				class="bg-foreground text-background flex items-center gap-1.5 rounded-9px px-3 py-1.5 text-xs font-medium transition-colors hover:opacity-90 disabled:opacity-50"
				onclick={triggerScan}
				disabled={scanRunning}
			>
				{scanRunning ? t('common.loading', 'Running...') : t('security.run_scan', 'Run Scan')}
			</button>
		</div>
		{#if scanMessage}
			<div class="mb-4">
				<div class="flex items-center gap-2 rounded-lg border px-4 py-3 text-sm {scanMessageType === 'success' ? 'border-emerald-500 text-emerald-600 bg-emerald-500/10' : 'border-destructive text-destructive bg-destructive/10'}">
					{scanMessage}
				</div>
			</div>
		{/if}

		<DataTable
			columns={scanColumns}
			rows={filteredResults}
			totalRows={totalResults}
			bind:page={scanPage}
			bind:pageSize={scanPageSize}
			pageSizeOptions={[20]}
			bind:sortKey={scanSortKey}
			bind:sortDirection={scanSortDirection}
			loading={scanLoading}
			emptyMessage={t('security.no_findings')}
			emptyIcon={ShieldCheck}
			selectable={true}
			bind:selectedIds={selectedScanIds}
			rowActions={scanRowActions}
			bulkActions={scanBulkActions}
		/>
	</TabsContent>

	<TabsContent value="ip-blocks">
		<div class="mb-4 flex items-center justify-between gap-3">
			<SearchInput bind:value={ipSearchQuery} placeholder={t('security.search_ip_blocks', 'Search IP blocks...')} class="w-64" />
			<button
				type="button"
				class="border-border-input bg-background-alt hover:bg-muted text-foreground flex items-center gap-2 rounded-9px border px-3.5 py-2 text-sm font-medium transition-colors"
				onclick={openCreateModal}
			>
				<Plus size={16} />
				{t('security.add_ip_block')}
			</button>
		</div>

		<DataTable
			columns={ipBlockColumns}
			rows={filteredIpBlocks}
			totalRows={filteredIpBlocks.length}
			bind:page={ipPage}
			bind:pageSize={ipPageSize}
			bind:sortKey={ipSortKey}
			bind:sortDirection={ipSortDirection}
			loading={ipBlocksLoading}
			emptyMessage={t('security.no_ip_blocks')}
			emptyIcon={ShieldCheck}
			rowActions={ipRowActions}
			bulkActions={ipBulkActions}
			selectable={true}
			bind:selectedIds={selectedIpIds}
		/>
	</TabsContent>
</Tabs>

<Modal
	bind:open={showCreateModal}
	onOpenChange={(isOpen) => { if (!isOpen) { cidr = ''; blockType = 'full'; reason = ''; createError = ''; } }}
	title={t('security.add_title')}
>
	{#snippet children(close)}
				{#if createError}
					<div class="border-destructive bg-destructive/10 text-destructive mb-4 rounded-lg border px-3 py-2 text-xs">
						{createError}
					</div>
				{/if}

				<div class="mb-4">
					<label for="cidr-input" class="text-foreground mb-1 block text-sm font-medium">{t('security.cidr_label')}</label>
					<input
						id="cidr-input"
						type="text"
						placeholder={t('security.cidr_placeholder')}
						class="border-border-input bg-background text-foreground placeholder:text-muted-foreground/50 rounded-9px w-full border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
						bind:value={cidr}
					/>
				</div>

				<div class="mb-4">
					<label for="block-type-select" class="text-foreground mb-1 block text-sm font-medium">{t('security.block_type_label')}</label>
					<AutocompleteSelect
						bind:value={blockType}
						options={blockTypeItems}
						placeholder={t('security.block_type')}
						ariaLabel={t('security.block_type_label')}
						class="w-full"
					/>
				</div>

				<div class="mb-6">
					<label for="reason-input" class="text-foreground mb-1 block text-sm font-medium">
						{t('security.reason_label')} <span class="text-muted-foreground">{t('security.reason_optional')}</span>
					</label>
					<input
						id="reason-input"
						type="text"
						placeholder={t('security.reason_placeholder')}
						class="border-border-input bg-background text-foreground placeholder:text-muted-foreground/50 rounded-9px w-full border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
						bind:value={reason}
					/>
				</div>
			<div class="mt-6 flex justify-end gap-3">
				<button
					type="button"
					class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-9px border px-4 py-2 text-sm font-medium transition-colors"
					onclick={close}
				>
					{t('common.cancel')}
				</button>
				<button
					type="button"
					class="bg-foreground text-background rounded-9px px-4 py-2 text-sm font-medium transition-colors disabled:opacity-50"
					disabled={createLoading || !cidr.trim()}
					onclick={createIpBlock}
				>
					{createLoading ? t('security.creating') : t('security.create_block')}
				</button>
			</div>
	{/snippet}
</Modal>

<ConfirmModal
	bind:open={showDeleteConfirm}
	title={t('security.remove_title')}
	consequence={deleteTarget ? t('security.remove_confirm').replace('{cidr}', deleteTarget.cidr) : ''}
	confirmText={deleteTarget?.cidr}
	onConfirm={deleteIpBlock}
	onCancel={closeDeleteConfirm}
/>

<!-- H6: Bulk delete confirmation for IP blocks -->
<ConfirmModal
	bind:open={confirmBulkDeleteIpOpen}
	title={t('security.bulk_delete_title', 'Delete Selected IP Blocks')}
	consequence={t('security.bulk_delete_confirm', 'Are you sure you want to delete the selected IP blocks? This action cannot be undone.')}
	confirmText={t('common.delete', 'Delete')}
	onConfirm={confirmBulkDeleteIp}
	onCancel={() => { confirmBulkDeleteIpOpen = false; pendingBulkIpIds = []; }}
/>

<!-- Single scan finding delete confirmation -->
<ConfirmModal
	bind:open={showDeleteScanConfirm}
	title={t('security.delete_finding_title', 'Delete Finding')}
	consequence={deleteScanTarget ? t('security.delete_finding_confirm', 'Are you sure you want to permanently delete this finding? This action cannot be undone.') : ''}
	confirmText={t('common.delete', 'Delete')}
	onConfirm={deleteScanResult}
	onCancel={closeDeleteScanConfirm}
/>

<!-- Bulk scan finding delete confirmation -->
<ConfirmModal
	bind:open={showBulkDeleteScanConfirm}
	title={t('security.bulk_delete_findings_title', 'Delete Selected Findings')}
	consequence={t('security.bulk_delete_findings_confirm', 'Are you sure you want to permanently delete the selected findings? This action cannot be undone.')}
	confirmText={t('common.delete', 'Delete')}
	onConfirm={confirmBulkDeleteScan}
	onCancel={() => { showBulkDeleteScanConfirm = false; pendingBulkScanIds = []; }}
/>
