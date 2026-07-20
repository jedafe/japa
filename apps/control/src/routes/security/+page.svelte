<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import SearchInput from '$lib/components/ui/SearchInput.svelte';
	import ConfirmModal from '$lib/components/ui/ConfirmModal.svelte';
	import { Tabs, Select, Dialog } from 'bits-ui';
	import { api } from '$lib/api';
	import { t, translations } from '$lib/stores/i18n.svelte';
	import { escapeHtml } from '$lib/utils';
	import { Plus, ShieldAlert, ShieldCheck, Trash2 } from 'lucide-svelte';
	import type { DataTableColumn, DataTableRowAction } from '$lib/api/types';

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
	let scanPage = $state(0);
	let scanPageSize = $state(10);
	let scanLoading = $state(true);
	let apiAvailable = $state(true);

	let filterSeverity = $state('all');
	let searchQuery = $state('');

	let ipBlocks = $state<IpBlock[]>([]);
	let ipBlocksLoading = $state(true);
	let ipSearchQuery = $state('');
	let ipSortKey = $state('');
	let ipSortDirection = $state<'asc' | 'desc'>('asc');
	let ipPage = $state(0);
	let ipPageSize = $state(10);

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
		info: 'bg-blue-500/15 text-blue-500',
		high: 'bg-destructive/15 text-destructive',
		medium: 'bg-amber-500/15 text-amber-500',
		low: 'bg-blue-500/15 text-blue-500'
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

	let filteredResults = $derived.by(() => {
		let list = results;
		if (filterSeverity !== 'all') {
			list = list.filter((r) => r.severity === filterSeverity);
		}
		if (searchQuery.trim()) {
			const q = searchQuery.toLowerCase();
			list = list.filter(
				(r) =>
					r.title.toLowerCase().includes(q) ||
					r.description.toLowerCase().includes(q)
			);
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

	let previousScanPage = $state(0);
	let previousScanPageSize = $state(10);

	async function loadIpBlocks() {
		ipBlocksLoading = true;
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

	$effect(() => {
		loadScanResults();
		loadIpBlocks();

		const interval = setInterval(() => {
			if (document.visibilityState === 'visible') {
				loadScanResults();
				loadIpBlocks();
			}
		}, 60_000);

		const onVisible = () => {
			if (document.visibilityState === 'visible') {
				loadScanResults();
				loadIpBlocks();
			}
		};
		document.addEventListener('visibilitychange', onVisible);

		return () => {
			clearInterval(interval);
			document.removeEventListener('visibilitychange', onVisible);
		};
	});

	async function loadScanResults() {
		scanLoading = true;
		const sev = filterSeverity !== 'all' ? `&severity=${filterSeverity}` : '';
		try {
			const result = await api<{ results: ScanResult[]; page: number; total_pages: number }>(
				`/control/security/scan?page=${scanPage}${sev}`
			);
			if (result.ok) {
				results = result.data.results ?? [];
				totalPages = result.data.total_pages ?? 1;
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
		scanPage = 0;
		loadScanResults();
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
			createError = t('security.cidr_label') + ' is required.';
			return;
		}
		// Basic CIDR validation
		if (!/^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\/\d{1,2}$/.test(cidr.trim())) {
			createError = 'Please enter a valid CIDR (e.g. 192.168.1.0/24).';
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

<Tabs.Root bind:value={activeTab}>
	<Tabs.List class="bg-muted inline-flex gap-1 rounded-9px p-1">
		<Tabs.Trigger value="scan-findings" class="data-[state=active]:bg-background data-[state=active]:shadow-mini rounded-7px px-4 py-2 text-sm font-medium transition-all">
			{t('security.scan_findings')}
		</Tabs.Trigger>
		<Tabs.Trigger value="ip-blocks" class="data-[state=active]:bg-background data-[state=active]:shadow-mini rounded-7px px-4 py-2 text-sm font-medium transition-all">
			{t('security.ip_blocks')}
			<span class="bg-accent text-accent-foreground ml-1.5 rounded-full px-1.5 py-0.5 text-[10px] font-medium">
				{ipBlocks.length}
			</span>
		</Tabs.Trigger>
	</Tabs.List>

	<Tabs.Content value="scan-findings" class="mt-4">
		<div class="mb-4 flex items-center gap-3">
			<Select.Root type="single" bind:value={filterSeverity} items={severityItems}>
				<Select.Trigger
					class="border-border-input bg-background data-placeholder:text-muted-foreground/50 rounded-9px flex h-9 items-center border px-3 text-sm transition-colors hover:border-border-input-hover"
					aria-label={t('security.filter_severity')}
				>
					<Select.Value placeholder={t('security.severity_all')} />
				</Select.Trigger>
				<Select.Portal>
					<Select.Content class="border-border-input bg-background shadow-popover z-50 rounded-xl border px-1 py-1" sideOffset={4}>
						<Select.Viewport class="p-1">
							{#each severityItems as item}
								<Select.Item
									class="data-highlighted:bg-accent rounded-9px flex h-9 cursor-pointer items-center px-3 text-sm outline-none"
									value={item.value}
									label={item.label}
								>
									{#snippet children({ selected })}
										{item.label}
										{#if selected}<span class="ml-auto">✓</span>{/if}
									{/snippet}
								</Select.Item>
							{/each}
						</Select.Viewport>
					</Select.Content>
				</Select.Portal>
			</Select.Root>
			<button
				type="button"
				class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-9px border px-3 py-1.5 text-sm font-medium transition-colors"
				onclick={applySeverityFilter}
			>
				{t('audit.apply_filters', 'Apply')}
			</button>
			<SearchInput bind:value={searchQuery} placeholder={t('security.search_findings', 'Search findings...')} class="w-64" />
		</div>

		<DataTable
			columns={scanColumns}
			rows={filteredResults}
			totalRows={filteredResults.length}
			bind:page={scanPage}
			bind:pageSize={scanPageSize}
			loading={scanLoading}
			emptyMessage={t('security.no_findings')}
			emptyIcon={ShieldCheck}
			selectable={false}
		/>
	</Tabs.Content>

	<Tabs.Content value="ip-blocks" class="mt-4">
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
			selectable={false}
		/>
	</Tabs.Content>
</Tabs.Root>

<Dialog.Root bind:open={showCreateModal}>
	<Dialog.Portal>
		<Dialog.Overlay class="bg-black/50 data-[state=open]:animate-in data-[state=open]:fade-in data-[state=closed]:animate-out data-[state=closed]:fade-out fixed inset-0 z-50" />
		<Dialog.Content
			class="border-border-card bg-background shadow-popover fixed left-1/2 top-1/2 z-50 w-full max-w-md -translate-x-1/2 -translate-y-1/2 rounded-xl border p-0"
			onOpenAutoFocus={(e) => e.preventDefault()}
		>
			<div class="border-border-card flex items-center justify-between border-b px-6 py-4">
				<Dialog.Title class="text-foreground text-lg font-semibold">{t('security.add_title')}</Dialog.Title>
				<Dialog.Close class="hover:bg-dark-04 rounded-9px p-1.5 transition-colors">
					<span class="sr-only">{t('common.close')}</span>
					✕
				</Dialog.Close>
			</div>
			<div class="px-6 py-4">
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
					<Select.Root type="single" bind:value={blockType} items={blockTypeItems}>
						<Select.Trigger
							class="border-border-input bg-background data-placeholder:text-muted-foreground/50 rounded-9px flex h-10 w-full items-center border px-3 text-sm transition-colors hover:border-border-input-hover"
							aria-label={t('security.block_type_label')}
						>
							<Select.Value placeholder={t('security.block_type')} />
						</Select.Trigger>
						<Select.Portal>
							<Select.Content class="border-border-input bg-background shadow-popover z-50 rounded-xl border px-1 py-1" sideOffset={4}>
								<Select.Viewport class="p-1">
									{#each blockTypeItems as item}
										<Select.Item
											class="data-highlighted:bg-accent rounded-9px flex h-9 cursor-pointer items-center px-3 text-sm outline-none"
											value={item.value}
											label={item.label}
										>
											{#snippet children({ selected })}
												{item.label}
												{#if selected}<span class="ml-auto">✓</span>{/if}
											{/snippet}
										</Select.Item>
									{/each}
								</Select.Viewport>
							</Select.Content>
						</Select.Portal>
					</Select.Root>
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
			</div>
			<div class="border-border-card flex justify-end gap-3 border-t px-6 py-4">
				<Dialog.Close class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-9px border px-4 py-2 text-sm font-medium transition-colors">
					{t('common.cancel')}
				</Dialog.Close>
				<button
					type="button"
					class="bg-foreground text-background rounded-9px px-4 py-2 text-sm font-medium transition-colors disabled:opacity-50"
					disabled={createLoading || !cidr.trim()}
					onclick={createIpBlock}
				>
					{createLoading ? t('security.creating') : t('security.create_block')}
				</button>
			</div>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>

<ConfirmModal
	bind:open={showDeleteConfirm}
	title={t('security.remove_title')}
	consequence={deleteTarget ? t('security.remove_confirm').replace('{cidr}', deleteTarget.cidr) : ''}
	confirmText={deleteTarget?.cidr}
	onConfirm={deleteIpBlock}
	onCancel={closeDeleteConfirm}
/>
