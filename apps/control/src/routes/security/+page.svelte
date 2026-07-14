<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import { Tabs, Select, Dialog, Tooltip } from 'bits-ui';
	import { api } from '$lib/api';
	import { Plus, Trash2, ShieldAlert, ShieldCheck, X } from 'lucide-svelte';
	import type { PageData } from './$types';

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

	let { data = {} as PageData } = $props();

	// svelte-ignore state_referenced_locally
	const pageData = data as { initial?: any; ipBlocks?: IpBlock[] };

	// --- Scan findings state ---
	let results = $state<ScanResult[]>(pageData.initial?.results ?? []);
	let filterSeverity = $state<string>('all');
	let page = $state(0);
	let totalPages = $state(pageData.initial?.total_pages ?? 1);
	let loading = $state(false);
	let apiAvailable = $state(pageData.initial !== null);
	let errorMessage = $state('');

	// --- IP blocks state ---
	let ipBlocks = $state<IpBlock[]>(pageData.ipBlocks ?? []);
	let showCreateModal = $state(false);
	let cidr = $state('');
	let blockType = $state('full');
	let reason = $state('');
	let createLoading = $state(false);
	let createError = $state('');

	// --- Delete modal state ---
	let showDeleteModal = $state(false);
	let deleteTarget = $state<IpBlock | null>(null);
	let deleteLoading = $state(false);
	let deleteError = $state('');

	const blockTypeLabels: Record<string, string> = {
		full: 'Full Block',
		registration: 'Registration',
		posting: 'Posting',
		api: 'API'
	};

	const severityItems = [
		{ value: 'all', label: 'All' },
		{ value: 'info', label: 'Info' },
		{ value: 'warning', label: 'Warning' },
		{ value: 'critical', label: 'Critical' }
	];

	const blockTypeItems = [
		{ value: 'full', label: 'Full Block' },
		{ value: 'registration', label: 'Registration' },
		{ value: 'posting', label: 'Posting' },
		{ value: 'api', label: 'API' }
	];

	const columns = [
		{ key: 'severity', label: 'Severity' },
		{ key: 'title', label: 'Title' },
		{ key: 'description', label: 'Description' },
		{ key: 'created_at', label: 'Found' },
		{ key: 'resolved', label: 'Status' }
	];

	let activeTab = $state('scan-findings');

	// --- Scan findings functions ---
	async function load() {
		loading = true;
		const sev = filterSeverity !== 'all' ? `&severity=${filterSeverity}` : '';
		try {
			const result = await api<{
				results: ScanResult[];
				page: number;
				total_pages: number;
			}>(`/control/security?page=${page}${sev}`);
			if (result.ok) {
				results = result.data.results;
				totalPages = result.data.total_pages;
				apiAvailable = true;
			} else {
				apiAvailable = false;
			}
		} catch {
			apiAvailable = false;
		} finally {
			loading = false;
		}
	}

	async function resolveFinding(id: string) {
		errorMessage = '';
		try {
			await api(`/control/security/${id}/resolve`, { method: 'POST' });
			await load();
		} catch {
			errorMessage = 'Failed to resolve finding. Please try again.';
		}
	}

	function handlePageChange(p: number) {
		page = p;
		load();
	}

	// --- IP block functions ---
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
			createError = 'CIDR is required.';
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
				const newBlock = result.data;
				ipBlocks = [newBlock, ...ipBlocks];
				closeCreateModal();
			} else {
				createError = 'Failed to create IP block. Please try again.';
			}
		} catch {
			createError = 'Failed to create IP block. Please try again.';
		} finally {
			createLoading = false;
		}
	}

	function openDeleteModal(block: IpBlock) {
		deleteTarget = block;
		deleteError = '';
		showDeleteModal = true;
	}

	function closeDeleteModal() {
		showDeleteModal = false;
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
				closeDeleteModal();
			} else {
				deleteError = 'Failed to delete IP block. Please try again.';
			}
		} catch {
			deleteError = 'Failed to delete IP block. Please try again.';
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

<PageHeader title="Security" description="Scan findings and security posture" />

{#if errorMessage}
	<div class="border-destructive bg-destructive/10 text-destructive mb-4 rounded-lg border px-4 py-3 text-sm">
		{errorMessage}
	</div>
{/if}

{#if !apiAvailable}
	<div class="border-tertiary bg-tertiary/10 text-tertiary mb-4 flex items-center gap-2 rounded-lg border px-4 py-3 text-sm">
		Security analytics API is not available.
	</div>
{/if}

<Tabs.Root bind:value={activeTab}>
	<Tabs.List class="bg-muted inline-flex gap-1 rounded-9px p-1">
		<Tabs.Trigger
			value="scan-findings"
			class="data-[state=active]:bg-background data-[state=active]:shadow-mini rounded-7px px-4 py-2 text-sm font-medium transition-all"
		>
			Scan Findings
		</Tabs.Trigger>
		<Tabs.Trigger
			value="ip-blocks"
			class="data-[state=active]:bg-background data-[state=active]:shadow-mini rounded-7px px-4 py-2 text-sm font-medium transition-all"
		>
			IP Blocks
			<span class="bg-accent text-accent-foreground ml-1.5 rounded-full px-1.5 py-0.5 text-[10px] font-medium">
				{ipBlocks.length}
			</span>
		</Tabs.Trigger>
	</Tabs.List>

	<!-- Scan Findings Tab -->
	<Tabs.Content value="scan-findings" class="mt-4">
		<div class="mb-4 flex items-center gap-3">
			<Select.Root type="single" bind:value={filterSeverity} items={severityItems}>
				<Select.Trigger
					class="border-border-input bg-background data-placeholder:text-muted-foreground/50 rounded-9px flex h-9 items-center border px-3 text-sm transition-colors hover:border-border-input-hover"
					aria-label="Filter severity"
				>
					<Select.Value placeholder="All" />
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
		</div>

		{#if loading}
			<p class="text-muted-foreground py-8 text-center text-sm">Loading...</p>
		{:else}
			<DataTable {columns} rows={results} {page} {totalPages} onPageChange={handlePageChange} />
		{/if}
	</Tabs.Content>

	<!-- IP Blocks Tab -->
	<Tabs.Content value="ip-blocks" class="mt-4">
		<div class="mb-6 flex items-center justify-between">
			<h2 class="text-foreground text-lg font-semibold">IP Block List</h2>
			<div class="flex items-center gap-2">
				<Tooltip.Provider>
					<Tooltip.Root>
						<Tooltip.Trigger>
							<button
								type="button"
								class="border-border-input bg-background-alt hover:bg-muted text-foreground flex items-center gap-2 rounded-9px border px-3.5 py-2 text-sm font-medium transition-colors"
								onclick={openCreateModal}
							>
								<Plus size={16} />
								Add IP Block
							</button>
						</Tooltip.Trigger>
						<Tooltip.Content
							class="border-border-input bg-background shadow-popover rounded-9px border px-3 py-1.5 text-xs"
							sideOffset={8}
						>
							Create a new IP or CIDR block
						</Tooltip.Content>
					</Tooltip.Root>
				</Tooltip.Provider>
			</div>
		</div>

		{#if ipBlocks.length === 0}
			<div class="border-border-card bg-background-alt shadow-card rounded-xl p-8 text-center">
				<ShieldCheck class="text-muted-foreground mx-auto mb-3 size-10 opacity-30" />
				<p class="text-muted-foreground text-sm">No IP blocks configured.</p>
			</div>
		{:else}
			<div class="border-border-card bg-background-alt shadow-card overflow-hidden rounded-xl">
				<div class="overflow-x-auto">
					<table class="w-full text-sm">
						<thead>
							<tr class="border-border-card border-b">
								<th class="text-muted-foreground px-4 py-3 text-left font-medium">IP / CIDR</th>
								<th class="text-muted-foreground px-4 py-3 text-left font-medium">Block Type</th>
								<th class="text-muted-foreground px-4 py-3 text-left font-medium">Reason</th>
								<th class="text-muted-foreground px-4 py-3 text-left font-medium">Created</th>
								<th class="text-muted-foreground px-4 py-3 text-right font-medium">Actions</th>
							</tr>
						</thead>
						<tbody>
							{#each ipBlocks as block (block.id)}
								<tr class="border-border-card hover:bg-dark-04 border-t transition-colors">
									<td class="px-4 py-3 font-mono text-xs">{block.cidr}</td>
									<td class="px-4 py-3">
										<span class="bg-destructive/10 text-destructive inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium">
											<ShieldAlert size={12} class="mr-1" />
											{blockTypeLabels[block.block_type] ?? block.block_type}
										</span>
									</td>
									<td class="text-muted-foreground px-4 py-3 text-xs">
										{block.reason ?? '—'}
									</td>
									<td class="text-muted-foreground px-4 py-3 text-xs">
										{formatDate(block.created_at)}
									</td>
									<td class="px-4 py-3 text-right">
										<Tooltip.Provider>
											<Tooltip.Root>
												<Tooltip.Trigger>
													<button
														type="button"
														class="hover:bg-destructive/10 hover:text-destructive rounded-9px p-1.5 text-muted-foreground transition-colors"
														onclick={() => openDeleteModal(block)}
													>
														<Trash2 size={15} />
													</button>
												</Tooltip.Trigger>
												<Tooltip.Content
													class="border-border-input bg-background shadow-popover rounded-9px border px-3 py-1.5 text-xs"
													sideOffset={8}
												>
													Remove IP block
												</Tooltip.Content>
											</Tooltip.Root>
										</Tooltip.Provider>
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			</div>
		{/if}
	</Tabs.Content>
</Tabs.Root>

<!-- Create IP Block Dialog -->
<Dialog.Root bind:open={showCreateModal}>
	<Dialog.Portal>
		<Dialog.Overlay class="bg-black/50 data-[state=open]:animate-in data-[state=open]:fade-in data-[state=closed]:animate-out data-[state=closed]:fade-out fixed inset-0 z-50" />
		<Dialog.Content
			class="border-border-card bg-background shadow-popover fixed left-1/2 top-1/2 z-50 w-full max-w-md -translate-x-1/2 -translate-y-1/2 rounded-xl border p-0"
			onOpenAutoFocus={(e) => e.preventDefault()}
		>
			<div class="border-border-card flex items-center justify-between border-b px-6 py-4">
				<Dialog.Title class="text-foreground text-lg font-semibold">Add IP Block</Dialog.Title>
				<Dialog.Close class="hover:bg-dark-04 rounded-9px p-1.5 transition-colors">
					<X class="size-4" />
				</Dialog.Close>
			</div>
			<div class="px-6 py-4">
				{#if createError}
					<div class="border-destructive bg-destructive/10 text-destructive mb-4 rounded-lg border px-3 py-2 text-xs">
						{createError}
					</div>
				{/if}

				<div class="mb-4">
					<label for="cidr-input" class="text-foreground mb-1 block text-sm font-medium">CIDR</label>
					<input
						id="cidr-input"
						type="text"
						placeholder="192.168.1.0/24 or 10.0.0.1"
						class="border-border-input bg-background text-foreground placeholder:text-muted-foreground/50 rounded-9px w-full border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
						bind:value={cidr}
					/>
				</div>

				<div class="mb-4">
					<label for="block-type-select" class="text-foreground mb-1 block text-sm font-medium">Block Type</label>
					<Select.Root type="single" bind:value={blockType} items={blockTypeItems}>
						<Select.Trigger
							class="border-border-input bg-background data-placeholder:text-muted-foreground/50 rounded-9px flex h-10 w-full items-center border px-3 text-sm transition-colors hover:border-border-input-hover"
							aria-label="Block type"
						>
							<Select.Value placeholder="Full Block" />
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
						Reason <span class="text-muted-foreground">(optional)</span>
					</label>
					<input
						id="reason-input"
						type="text"
						placeholder="Why this IP is blocked"
						class="border-border-input bg-background text-foreground placeholder:text-muted-foreground/50 rounded-9px w-full border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
						bind:value={reason}
					/>
				</div>
			</div>
			<div class="border-border-card flex justify-end gap-3 border-t px-6 py-4">
				<Dialog.Close class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-9px border px-4 py-2 text-sm font-medium transition-colors">
					Cancel
				</Dialog.Close>
				<button
					type="button"
					class="bg-foreground text-background rounded-9px px-4 py-2 text-sm font-medium transition-colors disabled:opacity-50"
					disabled={createLoading || !cidr.trim()}
					onclick={createIpBlock}
				>
					{createLoading ? 'Creating...' : 'Create Block'}
				</button>
			</div>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>

<!-- Delete Confirm Dialog -->
<Dialog.Root bind:open={showDeleteModal}>
	<Dialog.Portal>
		<Dialog.Overlay class="bg-black/50 data-[state=open]:animate-in data-[state=open]:fade-in data-[state=closed]:animate-out data-[state=closed]:fade-out fixed inset-0 z-50" />
		<Dialog.Content
			class="border-border-card bg-background shadow-popover fixed left-1/2 top-1/2 z-50 w-full max-w-md -translate-x-1/2 -translate-y-1/2 rounded-xl border p-0"
			onOpenAutoFocus={(e) => e.preventDefault()}
		>
			<div class="border-border-card flex items-center justify-between border-b px-6 py-4">
				<Dialog.Title class="text-foreground text-lg font-semibold">Remove IP Block</Dialog.Title>
				<Dialog.Close class="hover:bg-dark-04 rounded-9px p-1.5 transition-colors">
					<X class="size-4" />
				</Dialog.Close>
			</div>
			<div class="px-6 py-4">
				{#if deleteError}
					<div class="border-destructive bg-destructive/10 text-destructive mb-4 rounded-lg border px-3 py-2 text-xs">
						{deleteError}
					</div>
				{/if}
				{#if deleteTarget}
					<div class="bg-destructive/10 border-destructive/20 rounded-lg border p-3">
						<p class="text-destructive text-sm">
							Remove block for <strong class="font-mono">{deleteTarget.cidr}</strong>? This will unblock this IP/CIDR.
						</p>
					</div>
				{/if}
			</div>
			<div class="border-border-card flex justify-end gap-3 border-t px-6 py-4">
				<Dialog.Close class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-9px border px-4 py-2 text-sm font-medium transition-colors">
					Cancel
				</Dialog.Close>
				<button
					type="button"
					class="bg-destructive text-white rounded-9px px-4 py-2 text-sm font-medium transition-colors disabled:opacity-50"
					disabled={deleteLoading}
					onclick={deleteIpBlock}
				>
					{deleteLoading ? 'Removing...' : 'Remove'}
				</button>
			</div>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>
