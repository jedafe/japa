<script lang="ts">
	import { onMount } from 'svelte';
	import Modal from '$lib/components/Modal.svelte';
	import { spamApi } from '$lib/api';

	type IpBlock = { id: string; ip_cidr: string; reason: string; created_by: string; expires_at: string };
	type CensoredWord = { id: string; pattern: string; action: string; replacement: string; created_at: string };
	type UserBlock = { id: string; username: string; reason: string; blocked_at: string };

	type Tab = 'ip-blocks' | 'censored-words' | 'user-blocks';

	let activeTab = $state<Tab>('ip-blocks');

	// IP Blocks
	let ipBlocks = $state<IpBlock[]>([]);
	let ipPage = $state(0);
	let ipTotalPages = $state(1);
	let ipLoading = $state(false);

	// Censored Words
	let censoredWords = $state<CensoredWord[]>([]);
	let cwPage = $state(0);
	let cwTotalPages = $state(1);
	let cwLoading = $state(false);

	// User Blocks
	let userBlocks = $state<UserBlock[]>([]);
	let ubPage = $state(0);
	let ubTotalPages = $state(1);
	let ubLoading = $state(false);

	// Modals
	let showIpModal = $state(false);
	let showCwModal = $state(false);
	let showDeleteConfirm = $state(false);
	let deleteTarget = $state<{ type: string; id: string } | null>(null);

	// IP Block form
	let ipForm = $state({ ip_cidr: '', reason: '', expires_at: '' });
	let ipSaving = $state(false);

	// Censored Word form
	let cwForm = $state({ pattern: '', replacement: '' });
	let cwSaving = $state(false);

	let errorMessage = $state('');

	const tabs: { key: Tab; label: string }[] = [
		{ key: 'ip-blocks', label: 'IP Blocks' },
		{ key: 'censored-words', label: 'Censored Words' },
		{ key: 'user-blocks', label: 'User Blocks' }
	];

	async function loadIpBlocks() {
		ipLoading = true;
		const params = new URLSearchParams({ page: String(ipPage) });
		const result = await spamApi.getIpBlocks(params.toString());
		if (result.ok) {
			const data = result.data as { items: IpBlock[]; total_pages: number };
			ipBlocks = data.items ?? [];
			ipTotalPages = data.total_pages ?? 1;
		}
		ipLoading = false;
	}

	async function loadCensoredWords() {
		cwLoading = true;
		const params = new URLSearchParams({ page: String(cwPage) });
		const result = await spamApi.getCensoredWords(params.toString());
		if (result.ok) {
			const data = result.data as { items: CensoredWord[]; total_pages: number };
			censoredWords = data.items ?? [];
			cwTotalPages = data.total_pages ?? 1;
		}
		cwLoading = false;
	}

	async function loadUserBlocks() {
		ubLoading = true;
		const params = new URLSearchParams({ page: String(ubPage) });
		const result = await spamApi.getUserBlocks(params.toString());
		if (result.ok) {
			const data = result.data as { items: UserBlock[]; total_pages: number };
			userBlocks = data.items ?? [];
			ubTotalPages = data.total_pages ?? 1;
		}
		ubLoading = false;
	}

	function switchTab(tab: Tab) {
		activeTab = tab;
		errorMessage = '';
		if (tab === 'ip-blocks') loadIpBlocks();
		else if (tab === 'censored-words') loadCensoredWords();
		else loadUserBlocks();
	}

	async function submitIpBlock() {
		if (!ipForm.ip_cidr.trim()) return;
		ipSaving = true;
		errorMessage = '';
		const body: { ip_cidr: string; reason: string; expires_at?: string } = {
			ip_cidr: ipForm.ip_cidr.trim(),
			reason: ipForm.reason.trim()
		};
		if (ipForm.expires_at) body.expires_at = ipForm.expires_at;
		const result = await spamApi.createIpBlock(body);
		if (result.ok) {
			showIpModal = false;
			ipForm = { ip_cidr: '', reason: '', expires_at: '' };
			loadIpBlocks();
		} else {
			errorMessage = result.error;
		}
		ipSaving = false;
	}

	async function submitCensoredWord() {
		if (!cwForm.pattern.trim()) return;
		cwSaving = true;
		errorMessage = '';
		const body: { pattern: string; action: string; replacement?: string } = {
			pattern: cwForm.pattern.trim(),
			action: 'block'
		};
		if (cwForm.replacement.trim()) body.replacement = cwForm.replacement.trim();
		const result = await spamApi.createCensoredWord(body);
		if (result.ok) {
			showCwModal = false;
			cwForm = { pattern: '', replacement: '' };
			loadCensoredWords();
		} else {
			errorMessage = result.error;
		}
		cwSaving = false;
	}

	function confirmDelete(type: string, id: string) {
		deleteTarget = { type, id };
		showDeleteConfirm = true;
	}

	async function executeDelete() {
		if (!deleteTarget) return;
		errorMessage = '';
		const { type, id } = deleteTarget;
		let result;
		if (type === 'ip-block') result = await spamApi.deleteIpBlock(id);
		else if (type === 'censored-word') result = await spamApi.deleteCensoredWord(id);
		else result = await spamApi.deleteUserBlock(id);

		if (result.ok) {
			showDeleteConfirm = false;
			deleteTarget = null;
			if (type === 'ip-block') loadIpBlocks();
			else if (type === 'censored-word') loadCensoredWords();
			else loadUserBlocks();
		} else {
			errorMessage = result.error;
		}
	}

	onMount(() => loadIpBlocks());
</script>

<svelte:head>
	<title>Spam Management — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">Spam Management</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">Manage IP blocks, censored words, and user blocks</p>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<!-- Tabs -->
<div class="mb-4 flex gap-1 border-b" style="border-color: var(--color-border)">
	{#each tabs as tab}
		<button
			class="px-4 py-2 text-sm font-medium transition-colors border-b-2 -mb-px"
			style="border-color: {activeTab === tab.key ? 'var(--color-primary)' : 'transparent'}; color: {activeTab === tab.key ? 'var(--color-primary)' : 'var(--color-muted)'}"
			onclick={() => switchTab(tab.key)}
		>
			{tab.label}
		</button>
	{/each}
</div>

<!-- IP Blocks Tab -->
{#if activeTab === 'ip-blocks'}
	<div class="mb-4 flex justify-end">
		<button
			class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors"
			style="background-color: var(--color-primary)"
			onclick={() => { ipForm = { ip_cidr: '', reason: '', expires_at: '' }; showIpModal = true; }}
		>
			Add IP Block
		</button>
	</div>

	{#if ipLoading}
		<p class="py-8 text-center text-sm" style="color: var(--color-muted)">Loading...</p>
	{:else}
		<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
			<table class="w-full text-sm">
				<thead>
					<tr style="border-color: var(--color-border)">
						<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">IP / CIDR</th>
						<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Reason</th>
						<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Created By</th>
						<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Expires</th>
						<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)"></th>
					</tr>
				</thead>
				<tbody>
					{#each ipBlocks as block}
						<tr class="border-t hover:bg-black/[0.02] dark:hover:bg-white/[0.02]" style="border-color: var(--color-border)">
							<td class="px-4 py-3 font-medium font-mono text-xs">{block.ip_cidr}</td>
							<td class="px-4 py-3" style="color: var(--color-muted)">{block.reason}</td>
							<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{block.created_by}</td>
							<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{block.expires_at || 'Never'}</td>
							<td class="px-4 py-3">
								<button
									class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
									style="color: var(--color-danger)"
									onclick={() => confirmDelete('ip-block', block.id)}
								>
									Delete
								</button>
							</td>
						</tr>
					{:else}
						<tr>
							<td colspan="5" class="px-4 py-8 text-center" style="color: var(--color-muted)">No IP blocks configured</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>

		<div class="mt-3 flex items-center justify-between">
			<p class="text-xs" style="color: var(--color-muted)">Page {ipPage + 1} of {ipTotalPages}</p>
			<div class="flex gap-1">
				<button class="rounded p-1 hover:bg-black/5 dark:hover:bg-white/10 disabled:opacity-40" disabled={ipPage === 0} onclick={() => { ipPage--; loadIpBlocks(); }}>&larr;</button>
				<button class="rounded p-1 hover:bg-black/5 dark:hover:bg-white/10 disabled:opacity-40" disabled={ipPage >= ipTotalPages - 1} onclick={() => { ipPage++; loadIpBlocks(); }}>&rarr;</button>
			</div>
		</div>
	{/if}
{/if}

<!-- Censored Words Tab -->
{#if activeTab === 'censored-words'}
	<div class="mb-4 flex justify-end">
		<button
			class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors"
			style="background-color: var(--color-primary)"
			onclick={() => { cwForm = { pattern: '', replacement: '' }; showCwModal = true; }}
		>
			Add Pattern
		</button>
	</div>

	{#if cwLoading}
		<p class="py-8 text-center text-sm" style="color: var(--color-muted)">Loading...</p>
	{:else}
		<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
			<table class="w-full text-sm">
				<thead>
					<tr style="border-color: var(--color-border)">
						<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Pattern</th>
						<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Replacement</th>
						<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Created</th>
						<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)"></th>
					</tr>
				</thead>
				<tbody>
					{#each censoredWords as cw}
						<tr class="border-t hover:bg-black/[0.02] dark:hover:bg-white/[0.02]" style="border-color: var(--color-border)">
							<td class="px-4 py-3 font-medium font-mono text-xs">{cw.pattern}</td>
							<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{cw.replacement || '—'}</td>
							<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{cw.created_at}</td>
							<td class="px-4 py-3">
								<button
									class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
									style="color: var(--color-danger)"
									onclick={() => confirmDelete('censored-word', cw.id)}
								>
									Delete
								</button>
							</td>
						</tr>
					{:else}
						<tr>
							<td colspan="4" class="px-4 py-8 text-center" style="color: var(--color-muted)">No censored words configured</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>

		<div class="mt-3 flex items-center justify-between">
			<p class="text-xs" style="color: var(--color-muted)">Page {cwPage + 1} of {cwTotalPages}</p>
			<div class="flex gap-1">
				<button class="rounded p-1 hover:bg-black/5 dark:hover:bg-white/10 disabled:opacity-40" disabled={cwPage === 0} onclick={() => { cwPage--; loadCensoredWords(); }}>&larr;</button>
				<button class="rounded p-1 hover:bg-black/5 dark:hover:bg-white/10 disabled:opacity-40" disabled={cwPage >= cwTotalPages - 1} onclick={() => { cwPage++; loadCensoredWords(); }}>&rarr;</button>
			</div>
		</div>
	{/if}
{/if}

<!-- User Blocks Tab -->
{#if activeTab === 'user-blocks'}
	{#if ubLoading}
		<p class="py-8 text-center text-sm" style="color: var(--color-muted)">Loading...</p>
	{:else}
		<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
			<table class="w-full text-sm">
				<thead>
					<tr style="border-color: var(--color-border)">
						<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Username</th>
						<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Reason</th>
						<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Blocked At</th>
						<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)"></th>
					</tr>
				</thead>
				<tbody>
					{#each userBlocks as ub}
						<tr class="border-t hover:bg-black/[0.02] dark:hover:bg-white/[0.02]" style="border-color: var(--color-border)">
							<td class="px-4 py-3 font-medium">{ub.username}</td>
							<td class="px-4 py-3" style="color: var(--color-muted)">{ub.reason}</td>
							<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{ub.blocked_at}</td>
							<td class="px-4 py-3">
								<button
									class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
									style="color: var(--color-danger)"
									onclick={() => confirmDelete('user-block', ub.id)}
								>
									Unblock
								</button>
							</td>
						</tr>
					{:else}
						<tr>
							<td colspan="4" class="px-4 py-8 text-center" style="color: var(--color-muted)">No user blocks</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>

		<div class="mt-3 flex items-center justify-between">
			<p class="text-xs" style="color: var(--color-muted)">Page {ubPage + 1} of {ubTotalPages}</p>
			<div class="flex gap-1">
				<button class="rounded p-1 hover:bg-black/5 dark:hover:bg-white/10 disabled:opacity-40" disabled={ubPage === 0} onclick={() => { ubPage--; loadUserBlocks(); }}>&larr;</button>
				<button class="rounded p-1 hover:bg-black/5 dark:hover:bg-white/10 disabled:opacity-40" disabled={ubPage >= ubTotalPages - 1} onclick={() => { ubPage++; loadUserBlocks(); }}>&rarr;</button>
			</div>
		</div>
	{/if}
{/if}

<!-- Add IP Block Modal -->
<Modal bind:open={showIpModal} title="Add IP Block" onConfirm={submitIpBlock} confirmLabel={ipSaving ? 'Saving...' : 'Add Block'}>
	<div class="space-y-3">
		<div>
			<label for="ip-cidr" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">IP / CIDR</label>
			<input id="ip-cidr" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" placeholder="192.168.1.0/24" bind:value={ipForm.ip_cidr} />
		</div>
		<div>
			<label for="ip-reason" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Reason</label>
			<input id="ip-reason" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" placeholder="Spam source" bind:value={ipForm.reason} />
		</div>
		<div>
			<label for="ip-expires" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Expires At (optional)</label>
			<input id="ip-expires" type="datetime-local" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" bind:value={ipForm.expires_at} />
		</div>
	</div>
</Modal>

<!-- Add Censored Word Modal -->
<Modal bind:open={showCwModal} title="Add Censored Pattern" onConfirm={submitCensoredWord} confirmLabel={cwSaving ? 'Saving...' : 'Add Pattern'}>
	<div class="space-y-3">
		<div>
			<label for="cw-word" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Pattern</label>
			<input id="cw-word" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" placeholder="badword" bind:value={cwForm.pattern} />
		</div>
		<div>
			<label for="cw-replacement" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Replacement (optional)</label>
			<input id="cw-replacement" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" placeholder="***" bind:value={cwForm.replacement} />
		</div>
	</div>
</Modal>

<!-- Delete Confirmation Modal -->
<Modal bind:open={showDeleteConfirm} title="Confirm Delete" onConfirm={executeDelete} confirmLabel="Delete">
	<p class="text-sm" style="color: var(--color-muted)">Are you sure you want to delete this item? This action cannot be undone.</p>
</Modal>
