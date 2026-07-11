<script lang="ts">
	import { onMount } from 'svelte';
	import Modal from '$lib/components/Modal.svelte';
	import { spamApi } from '$lib/api';

	// Row shapes mirror the backend JSON (internal/db/gen models); these
	// endpoints return `{"data": [...]}` arrays without pagination.
	type IpBlock = { id: string; cidr: string; block_type: string; reason: string | null; blocked_by?: string; expires_at?: string | null; created_at?: string };
	type CensoredWord = { id: string; pattern: string; action: string; replacement: string | null; created_at?: string };
	type UserBlock = { blocker_id: string; blocked_id: string; created_at?: string };

	type Tab = 'ip-blocks' | 'censored-words' | 'user-blocks';

	let activeTab = $state<Tab>('ip-blocks');

	// IP Blocks
	let ipBlocks = $state<IpBlock[]>([]);
	let ipLoading = $state(false);

	// Censored Words
	let censoredWords = $state<CensoredWord[]>([]);
	let cwLoading = $state(false);

	// User Blocks
	let userBlocks = $state<UserBlock[]>([]);
	let ubLoading = $state(false);

	// Modals
	let showIpModal = $state(false);
	let showCwModal = $state(false);
	let showDeleteConfirm = $state(false);
	// User blocks are keyed by (blocker_id, blocked_id), not a single id.
	let deleteTarget = $state<{ type: string; id: string; secondaryId?: string } | null>(null);

	// IP Block form
	let ipForm = $state({ cidr: '', block_type: 'full', reason: '', expires_at: '' });
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
		const result = await spamApi.getIpBlocks();
		if (result.ok) {
			ipBlocks = (result.data as IpBlock[]) ?? [];
		}
		ipLoading = false;
	}

	async function loadCensoredWords() {
		cwLoading = true;
		const result = await spamApi.getCensoredWords();
		if (result.ok) {
			censoredWords = (result.data as CensoredWord[]) ?? [];
		}
		cwLoading = false;
	}

	async function loadUserBlocks() {
		ubLoading = true;
		const result = await spamApi.getUserBlocks();
		if (result.ok) {
			userBlocks = (result.data as UserBlock[]) ?? [];
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
		if (!ipForm.cidr.trim()) return;
		ipSaving = true;
		errorMessage = '';
		const body: { cidr: string; block_type: string; reason?: string; expires_at?: string } = {
			cidr: ipForm.cidr.trim(),
			block_type: ipForm.block_type
		};
		if (ipForm.reason.trim()) body.reason = ipForm.reason.trim();
		if (ipForm.expires_at) body.expires_at = ipForm.expires_at;
		const result = await spamApi.createIpBlock(body);
		if (result.ok) {
			showIpModal = false;
			ipForm = { cidr: '', block_type: 'full', reason: '', expires_at: '' };
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

	function confirmDelete(type: string, id: string, secondaryId?: string) {
		deleteTarget = { type, id, secondaryId };
		showDeleteConfirm = true;
	}

	async function executeDelete() {
		if (!deleteTarget) return;
		errorMessage = '';
		const { type, id, secondaryId } = deleteTarget;
		let result;
		if (type === 'ip-block') result = await spamApi.deleteIpBlock(id);
		else if (type === 'censored-word') result = await spamApi.deleteCensoredWord(id);
		else result = await spamApi.deleteUserBlock(id, secondaryId ?? '');

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
			onclick={() => { ipForm = { cidr: '', block_type: 'full', reason: '', expires_at: '' }; showIpModal = true; }}
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
						<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Type</th>
						<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Reason</th>
						<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Expires</th>
						<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)"></th>
					</tr>
				</thead>
				<tbody>
					{#each ipBlocks as block}
						<tr class="border-t hover:bg-black/[0.02] dark:hover:bg-white/[0.02]" style="border-color: var(--color-border)">
							<td class="px-4 py-3 font-medium font-mono text-xs">{block.cidr}</td>
							<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{block.block_type}</td>
							<td class="px-4 py-3" style="color: var(--color-muted)">{block.reason ?? '—'}</td>
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
						<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Blocker</th>
						<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Blocked</th>
						<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Blocked At</th>
						<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)"></th>
					</tr>
				</thead>
				<tbody>
					{#each userBlocks as ub}
						<tr class="border-t hover:bg-black/[0.02] dark:hover:bg-white/[0.02]" style="border-color: var(--color-border)">
							<td class="px-4 py-3 font-mono text-xs">{ub.blocker_id}</td>
							<td class="px-4 py-3 font-mono text-xs">{ub.blocked_id}</td>
							<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{ub.created_at ?? '—'}</td>
							<td class="px-4 py-3">
								<button
									class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
									style="color: var(--color-danger)"
									onclick={() => confirmDelete('user-block', ub.blocker_id, ub.blocked_id)}
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
	{/if}
{/if}

<!-- Add IP Block Modal -->
<Modal bind:open={showIpModal} title="Add IP Block" onConfirm={submitIpBlock} confirmLabel={ipSaving ? 'Saving...' : 'Add Block'}>
	<div class="space-y-3">
		<div>
			<label for="ip-cidr" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">IP / CIDR</label>
			<input id="ip-cidr" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" placeholder="192.168.1.0/24" bind:value={ipForm.cidr} />
		</div>
		<div>
			<label for="ip-block-type" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Block Type</label>
			<select id="ip-block-type" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" bind:value={ipForm.block_type}>
				<option value="full">Full</option>
				<option value="registration">Registration</option>
				<option value="posting">Posting</option>
				<option value="api">API</option>
			</select>
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
