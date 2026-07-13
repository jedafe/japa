<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import ConfirmModal from '$lib/components/ui/ConfirmModal.svelte';
	import { api } from '$lib/api';
	import { Plus, Trash2, ShieldAlert, ShieldCheck } from 'lucide-svelte';
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

	const columns = [
		{ key: 'severity', label: 'Severity' },
		{ key: 'title', label: 'Title' },
		{ key: 'description', label: 'Description' },
		{ key: 'created_at', label: 'Found' },
		{ key: 'resolved', label: 'Status' }
	];

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
			const result = await api<{ data: IpBlock }>('/control/security/ip-blocks', {
				method: 'POST',
				body: {
					cidr: cidr.trim(),
					block_type: blockType,
					reason: reason.trim() || undefined
				}
			});
			if (result.ok) {
				const newBlock = (result.data as any).data ?? result.data;
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
	<div
		class="mb-4 rounded-md border px-4 py-3 text-sm"
		style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)"
	>
		{errorMessage}
	</div>
{/if}

{#if !apiAvailable}
	<div
		class="mb-4 rounded-md border px-4 py-3 text-sm"
		style="border-color: var(--color-warning); background-color: color-mix(in srgb, var(--color-warning) 8%, transparent); color: var(--color-warning)"
	>
		Security analytics API is not available.
	</div>
{/if}

<!-- Scan Findings Section -->
<div class="mb-4 flex items-center gap-3">
	<label for="severity-filter" class="text-xs font-medium" style="color: var(--color-muted)">Filter severity:</label>
	<select
		id="severity-filter"
		class="rounded-md border px-3 py-1.5 text-sm"
		style="border-color: var(--color-border); background-color: var(--color-surface)"
		bind:value={filterSeverity}
		onchange={() => { page = 0; load(); }}
	>
		<option value="all">All</option>
		<option value="info">Info</option>
		<option value="warning">Warning</option>
		<option value="critical">Critical</option>
	</select>
</div>

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">Loading...</p>
{:else}
	<DataTable {columns} rows={results} {page} {totalPages} onPageChange={handlePageChange} />
{/if}

<!-- IP Blocks Section -->
<div class="mt-12 border-t pt-8" style="border-color: var(--color-border)">
	<div class="mb-6 flex items-center justify-between">
		<div class="flex items-center gap-3">
			<h2 class="text-lg font-semibold" style="color: var(--color-text)">IP Block List</h2>
			<span
				class="inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium"
				style="background-color: color-mix(in srgb, var(--color-primary) 15%, transparent); color: var(--color-primary)"
			>
				{ipBlocks.length}
			</span>
		</div>
		<button
			type="button"
			class="inline-flex items-center gap-2 rounded-md px-3.5 py-2 text-sm font-medium text-white transition-colors"
			style="background-color: var(--color-primary)"
			onclick={openCreateModal}
		>
			<Plus size={16} />
			Add IP Block
		</button>
	</div>

	{#if ipBlocks.length === 0}
		<div
			class="rounded-lg border p-8 text-center"
			style="border-color: var(--color-border); background-color: var(--color-surface)"
		>
			<ShieldCheck size={40} class="mx-auto mb-3 opacity-30" style="color: var(--color-muted)" />
			<p class="text-sm" style="color: var(--color-muted)">No IP blocks configured.</p>
		</div>
	{:else}
		<div
			class="overflow-hidden rounded-lg border"
			style="border-color: var(--color-border); background-color: var(--color-surface)"
		>
			<table class="w-full text-left text-sm">
				<thead>
					<tr style="border-bottom: 1px solid var(--color-border); background-color: var(--color-background)">
						<th class="px-4 py-3 font-medium" style="color: var(--color-muted)">IP / CIDR</th>
						<th class="px-4 py-3 font-medium" style="color: var(--color-muted)">Block Type</th>
						<th class="px-4 py-3 font-medium" style="color: var(--color-muted)">Reason</th>
						<th class="px-4 py-3 font-medium" style="color: var(--color-muted)">Created</th>
						<th class="px-4 py-3 font-medium text-right" style="color: var(--color-muted)">Actions</th>
					</tr>
				</thead>
				<tbody>
					{#each ipBlocks as block (block.id)}
						<tr style="border-bottom: 1px solid var(--color-border)">
							<td class="px-4 py-3 font-mono text-xs" style="color: var(--color-text)">{block.cidr}</td>
							<td class="px-4 py-3">
								<span
									class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium"
									style="background-color: color-mix(in srgb, var(--color-danger) 12%, transparent); color: var(--color-danger)"
								>
									<ShieldAlert size={12} class="mr-1" />
									{blockTypeLabels[block.block_type] ?? block.block_type}
								</span>
							</td>
							<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">
								{block.reason ?? '—'}
							</td>
							<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">
								{formatDate(block.created_at)}
							</td>
							<td class="px-4 py-3 text-right">
								<button
									type="button"
									class="inline-flex items-center justify-center rounded p-1.5 transition-colors"
									style="color: var(--color-muted); background-color: transparent"
									onmouseenter={(e) => (e.currentTarget.style.color = 'var(--color-danger)')}
									onmouseleave={(e) => (e.currentTarget.style.color = 'var(--color-muted)')}
									title="Remove IP block"
									onclick={() => openDeleteModal(block)}
								>
									<Trash2 size={15} />
								</button>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>

<!-- Create IP Block Modal -->
{#if showCreateModal}
	<div class="fixed inset-0 z-50 flex items-center justify-center">
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div class="absolute inset-0 bg-black/50" onclick={closeCreateModal}></div>
		<div
			class="relative z-10 w-full max-w-md rounded-lg border p-6 shadow-xl"
			style="border-color: var(--color-border); background-color: var(--color-surface)"
		>
			<h3 class="mb-4 text-lg font-semibold" style="color: var(--color-text)">Add IP Block</h3>

			{#if createError}
				<div
					class="mb-4 rounded-md border px-3 py-2 text-xs"
					style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)"
				>
					{createError}
				</div>
			{/if}

			<div class="mb-4">
				<label for="cidr-input" class="mb-1 block text-sm font-medium" style="color: var(--color-text)">CIDR</label>
				<input
					id="cidr-input"
					type="text"
					placeholder="192.168.1.0/24 or 10.0.0.1"
					class="w-full rounded-md border px-3 py-2 text-sm outline-none focus:ring-2"
					style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-text); --tw-ring-color: var(--color-primary)"
					bind:value={cidr}
				/>
			</div>

			<div class="mb-4">
				<label for="block-type-select" class="mb-1 block text-sm font-medium" style="color: var(--color-text)">Block Type</label>
				<select
					id="block-type-select"
					class="w-full rounded-md border px-3 py-2 text-sm outline-none focus:ring-2"
					style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-text); --tw-ring-color: var(--color-primary)"
					bind:value={blockType}
				>
					<option value="full">Full Block</option>
					<option value="registration">Registration</option>
					<option value="posting">Posting</option>
					<option value="api">API</option>
				</select>
			</div>

			<div class="mb-6">
				<label for="reason-input" class="mb-1 block text-sm font-medium" style="color: var(--color-text)">Reason <span style="color: var(--color-muted)">(optional)</span></label>
				<input
					id="reason-input"
					type="text"
					placeholder="Why this IP is blocked"
					class="w-full rounded-md border px-3 py-2 text-sm outline-none focus:ring-2"
					style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-text); --tw-ring-color: var(--color-primary)"
					bind:value={reason}
				/>
			</div>

			<div class="flex justify-end gap-3">
				<button
					type="button"
					class="rounded-md border px-4 py-2 text-sm font-medium transition-colors"
					style="border-color: var(--color-border); color: var(--color-text)"
					onclick={closeCreateModal}
				>
					Cancel
				</button>
				<button
					type="button"
					class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors disabled:opacity-50"
					style="background-color: var(--color-primary)"
					disabled={createLoading || !cidr.trim()}
					onclick={createIpBlock}
				>
					{createLoading ? 'Creating...' : 'Create Block'}
				</button>
			</div>
		</div>
	</div>
{/if}

<!-- Delete Confirm Modal -->
<ConfirmModal
	bind:open={showDeleteModal}
	title="Remove IP Block"
	consequence={deleteTarget ? `Remove block for ${deleteTarget.cidr}? This will unblock this IP/CIDR.` : ''}
	confirmText="REMOVE"
	onConfirm={deleteIpBlock}
	onCancel={closeDeleteModal}
/>

{#if deleteError}
	<div class="fixed bottom-4 right-4 z-50">
		<div
			class="rounded-md border px-4 py-3 text-sm shadow-lg"
			style="border-color: var(--color-danger); background-color: var(--color-surface); color: var(--color-danger)"
		>
			{deleteError}
		</div>
	</div>
{/if}
