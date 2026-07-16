<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import SearchInput from '$lib/components/ui/SearchInput.svelte';
	import ConfirmModal from '$lib/components/ui/ConfirmModal.svelte';
	import { Tabs, Select, Dialog } from 'bits-ui';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n';
	import { Plus, ShieldAlert, ShieldCheck, Trash2 } from 'lucide-svelte';
	import type { PageData } from './$types';
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

	type SecurityPageData = {
		initial?: { results: ScanResult[]; total_pages: number } | null;
		ipBlocks?: IpBlock[];
	};

	let { data = {} as PageData } = $props();

	const pageData = data as SecurityPageData;

	let activeTab = $state('scan-findings');

	let results = $state<ScanResult[]>(pageData.initial?.results ?? []);
	let totalPages = $state(pageData.initial?.total_pages ?? 1);
	let scanPage = $state(0);
	let scanPageSize = $state(10);
	let scanLoading = $state(false);
	let apiAvailable = $state(pageData.initial !== null);

	let filterSeverity = $state('all');
	let searchQuery = $state('');

	let ipBlocks = $state<IpBlock[]>(pageData.ipBlocks ?? []);
	let ipBlocksLoading = $state(false);
	let ipSearchQuery = $state('');

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

	const blockTypeLabels: Record<string, string> = {
		full: t('security.block_full'),
		registration: t('security.block_registration'),
		posting: t('security.block_posting'),
		api: t('security.block_api')
	};

	const severityItems = [
		{ value: 'all', label: t('security.severity_all') },
		{ value: 'info', label: t('security.severity_info') },
		{ value: 'warning', label: t('security.severity_warning') },
		{ value: 'critical', label: t('security.severity_critical') }
	];

	const blockTypeItems = [
		{ value: 'full', label: t('security.block_full') },
		{ value: 'registration', label: t('security.block_registration') },
		{ value: 'posting', label: t('security.block_posting') },
		{ value: 'api', label: t('security.block_api') }
	];

	const severityClasses: Record<string, string> = {
		critical: 'bg-destructive/15 text-destructive',
		warning: 'bg-amber-500/15 text-amber-500',
		info: 'bg-blue-500/15 text-blue-500',
		high: 'bg-destructive/15 text-destructive',
		medium: 'bg-amber-500/15 text-amber-500',
		low: 'bg-blue-500/15 text-blue-500'
	};

	const scanColumns: DataTableColumn[] = [
		{
			key: 'severity',
			label: t('security.severity'),
			sortable: true,
			render: (val) => {
				const sev = String(val);
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
			render: (val) => formatDate(String(val))
		},
		{
			key: 'resolved',
			label: t('common.status'),
			render: (val) =>
				val
					? `<span class="bg-green-500/15 text-green-600 rounded-full px-2 py-0.5 text-xs font-medium">${t('security.resolved')}</span>`
					: `<span class="bg-amber-500/15 text-amber-500 rounded-full px-2 py-0.5 text-xs font-medium">${t('security.open')}</span>`
		}
	];

	const ipBlockColumns: DataTableColumn[] = [
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
				const label = blockTypeLabels[String(val)] ?? String(val);
				return `<span class="bg-destructive/10 text-destructive inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium"><span class="mr-1">⚠</span>${label}</span>`;
			}
		},
		{
			key: 'reason',
			label: t('security.reason'),
			render: (val) => `<span class="text-muted-foreground text-xs">${val ?? '—'}</span>`
		},
		{
			key: 'created_at',
			label: t('security.created'),
			sortable: true,
			render: (val) => `<span class="text-muted-foreground text-xs">${formatDate(String(val))}</span>`
		}
	];

	const ipRowActions: DataTableRowAction[] = [
		{
			label: t('common.delete'),
			icon: Trash2,
			variant: 'destructive',
			onClick: (row) => openDeleteConfirm(row as unknown as IpBlock)
		}
	];

	let filteredResults = $derived(() => {
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

	let filteredIpBlocks = $derived(() => {
		if (!ipSearchQuery.trim()) return ipBlocks;
		const q = ipSearchQuery.toLowerCase();
		return ipBlocks.filter(
			(b) =>
				b.cidr.toLowerCase().includes(q) ||
				b.block_type.toLowerCase().includes(q) ||
				(b.reason ?? '').toLowerCase().includes(q)
		);
	});

	let previousScanPage = $state(0);
	let previousScanPageSize = $state(10);

	$effect(() => {
		if (scanPage !== previousScanPage || scanPageSize !== previousScanPageSize) {
			previousScanPage = scanPage;
			previousScanPageSize = scanPageSize;
			if (apiAvailable) loadScanResults();
		}
	});

	async function loadScanResults() {
		scanLoading = true;
		const sev = filterSeverity !== 'all' ? `&severity=${filterSeverity}` : '';
		try {
			const result = await api<
				| { active_ip_blocks: number }
				| { results: ScanResult[]; page: number; total_pages: number }
			>(`/control/security?page=${scanPage}${sev}`);
			if (result.ok) {
				const raw = result.data;
				if ('active_ip_blocks' in raw) {
					results = [];
					totalPages = 1;
				} else {
					results = raw.results ?? [];
					totalPages = raw.total_pages ?? 1;
				}
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
			rows={filteredResults()}
			totalRows={filteredResults().length}
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
			rows={filteredIpBlocks()}
			totalRows={filteredIpBlocks().length}
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
