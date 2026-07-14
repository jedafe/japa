<script lang="ts">
	import { onMount } from 'svelte';
	import { RefreshCw, Plus, Trash2, Ban, MessageSquareOff, Users } from 'lucide-svelte';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent } from '$lib/components/ui/card';
	import { Tabs } from '$lib/components/ui/tabs';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Modal } from '$lib/components/ui/modal';
	import { spamApi } from '$lib/api';

	type IpBlock = {
		id: string;
		cidr: string;
		block_type: string;
		reason: string | null;
		blocked_by?: string;
		expires_at?: string | null;
		created_at?: string;
	};
	type CensoredWord = {
		id: string;
		pattern: string;
		action: string;
		replacement: string | null;
		created_at?: string;
	};
	type UserBlock = {
		blocker_id: string;
		blocked_id: string;
		created_at?: string;
	};

	type Tab = 'ip-blocks' | 'censored-words' | 'user-blocks';

	let activeTab = $state<Tab>('ip-blocks');
	let loading = $state(false);
	let errorMessage = $state('');

	// IP Blocks
	let ipBlocks = $state<IpBlock[]>([]);
	let showIpModal = $state(false);
	let ipForm = $state({ cidr: '', block_type: 'full', reason: '', expires_at: '' });
	let ipSaving = $state(false);

	// Censored Words
	let censoredWords = $state<CensoredWord[]>([]);
	let showCwModal = $state(false);
	let cwForm = $state({ pattern: '', replacement: '' });
	let cwSaving = $state(false);

	// User Blocks
	let userBlocks = $state<UserBlock[]>([]);

	// Delete confirmation
	let showDeleteConfirm = $state(false);
	let deleteTarget = $state<{ type: string; id: string; secondaryId?: string } | null>(null);

	const tabs: { value: Tab; label: string; icon: any }[] = [
		{ value: 'ip-blocks', label: 'IP Blocks', icon: Ban },
		{ value: 'censored-words', label: 'Censored Words', icon: MessageSquareOff },
		{ value: 'user-blocks', label: 'User Blocks', icon: Users }
	];

	const ipColumns = [
		{
			key: 'cidr',
			label: 'IP / CIDR',
			sortable: true,
			render: (value: string) => `<span class="font-mono text-sm text-gray-900">${value}</span>`
		},
		{
			key: 'block_type',
			label: 'Type',
			sortable: true,
			render: (value: string) => {
				const cls =
					value === 'full'
						? 'bg-danger-100 text-danger-800'
						: 'bg-warning-100 text-warning-800';
				return `<span class="inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${cls}">${value}</span>`;
			}
		},
		{
			key: 'reason',
			label: 'Reason',
			render: (value: string) => `<span class="text-sm text-gray-600">${value ?? '—'}</span>`
		},
		{
			key: 'expires_at',
			label: 'Expires',
			render: (value: string) =>
				`<span class="text-sm text-gray-500">${value || 'Never'}</span>`
		},
		{
			key: 'actions',
			label: '',
			class: 'w-24',
			render: (_: any, row: IpBlock) => `
				<button onclick="window.spamDeleteIp('${row.id}')" class="inline-flex items-center justify-center rounded-md p-1.5 text-danger-700 hover:bg-danger-50 transition-colors" title="Delete">
					<svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"></path></svg>
				</button>`
		}
	];

	const cwColumns = [
		{
			key: 'pattern',
			label: 'Pattern',
			sortable: true,
			render: (value: string) =>
				`<span class="font-mono text-sm text-gray-900">${value}</span>`
		},
		{
			key: 'replacement',
			label: 'Replacement',
			render: (value: string) =>
				`<span class="text-sm text-gray-600">${value || '—'}</span>`
		},
		{
			key: 'created_at',
			label: 'Created',
			render: (value: string) =>
				`<span class="text-sm text-gray-500">${value ?? '—'}</span>`
		},
		{
			key: 'actions',
			label: '',
			class: 'w-24',
			render: (_: any, row: CensoredWord) => `
				<button onclick="window.spamDeleteCw('${row.id}')" class="inline-flex items-center justify-center rounded-md p-1.5 text-danger-700 hover:bg-danger-50 transition-colors" title="Delete">
					<svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"></path></svg>
				</button>`
		}
	];

	const ubColumns = [
		{
			key: 'blocker_id',
			label: 'Blocker',
			render: (value: string) =>
				`<span class="font-mono text-xs text-gray-700">${value}</span>`
		},
		{
			key: 'blocked_id',
			label: 'Blocked',
			render: (value: string) =>
				`<span class="font-mono text-xs text-gray-700">${value}</span>`
		},
		{
			key: 'created_at',
			label: 'Blocked At',
			render: (value: string) =>
				`<span class="text-sm text-gray-500">${value ?? '—'}</span>`
		},
		{
			key: 'actions',
			label: '',
			class: 'w-24',
			render: (_: any, row: UserBlock) => `
				<button onclick="window.spamDeleteUb('${row.blocker_id}','${row.blocked_id}')" class="inline-flex items-center justify-center rounded-md p-1.5 text-danger-700 hover:bg-danger-50 transition-colors" title="Unblock">
					<svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M18.364 18.364A9 9 0 005.636 5.636m12.728 12.728A9 9 0 015.636 5.636m12.728 12.728L5.636 5.636"></path></svg>
				</button>`
		}
	];

	async function loadIpBlocks() {
		loading = true;
		errorMessage = '';
		const result = await spamApi.getIpBlocks();
		if (result.ok) {
			ipBlocks = (result.data as IpBlock[]) ?? [];
		} else {
			errorMessage = result.error;
		}
		loading = false;
	}

	async function loadCensoredWords() {
		loading = true;
		errorMessage = '';
		const result = await spamApi.getCensoredWords();
		if (result.ok) {
			censoredWords = (result.data as CensoredWord[]) ?? [];
		} else {
			errorMessage = result.error;
		}
		loading = false;
	}

	async function loadUserBlocks() {
		loading = true;
		errorMessage = '';
		const result = await spamApi.getUserBlocks();
		if (result.ok) {
			userBlocks = (result.data as UserBlock[]) ?? [];
		} else {
			errorMessage = result.error;
		}
		loading = false;
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

	$effect(() => {
		(window as any).spamDeleteIp = (id: string) => confirmDelete('ip-block', id);
		(window as any).spamDeleteCw = (id: string) => confirmDelete('censored-word', id);
		(window as any).spamDeleteUb = (blockerId: string, blockedId: string) =>
			confirmDelete('user-block', blockerId, blockedId);
	});

	onMount(() => loadIpBlocks());
</script>

<svelte:head>
	<title>Spam Management — JapaConnect Admin</title>
</svelte:head>

<PageHeader title="Spam Management" description="Manage IP blocks, censored words, and user blocks" />

<div class="space-y-6">
	{#if errorMessage}
		<div class="rounded-lg border border-danger-200 bg-danger-50 px-4 py-3 text-sm text-danger-800">
			{errorMessage}
		</div>
	{/if}

	<Tabs tabs={[]} bind:value={activeTab}>
		{#snippet children()}
			<!-- Custom tab bar since Tabs component expects tabs prop -->
		{/snippet}
	</Tabs>

	<!-- Tab navigation using the same visual pattern as Tabs component -->
	<div class="flex gap-1 rounded-lg bg-gray-100 p-1">
		{#each tabs as tab}
			{@const Icon = tab.icon}
			<button
				class="inline-flex items-center justify-center whitespace-nowrap rounded-md px-3 py-1.5 text-sm font-medium transition-all disabled:pointer-events-none disabled:opacity-50 {activeTab === tab.value
					? 'bg-white text-gray-950 shadow-sm'
					: 'text-gray-500 hover:text-gray-900'}"
				onclick={() => switchTab(tab.value)}
			>
				<Icon class="mr-2 h-4 w-4" />
				{tab.label}
			</button>
		{/each}
	</div>

	<!-- IP Blocks Tab -->
	{#if activeTab === 'ip-blocks'}
		<div class="flex items-center justify-between">
			<p class="text-sm text-gray-500">{ipBlocks.length} IP blocks configured</p>
			<Button
				onclick={() => {
					ipForm = { cidr: '', block_type: 'full', reason: '', expires_at: '' };
					showIpModal = true;
				}}
			>
				<Plus class="h-4 w-4 mr-2" />
				Add IP Block
			</Button>
		</div>

		{#if loading}
			<Card>
				<CardContent class="py-12">
					<div class="flex items-center justify-center text-gray-500">
						<RefreshCw class="h-5 w-5 mr-2 animate-spin" />
						Loading...
					</div>
				</CardContent>
			</Card>
		{:else}
			<Card>
				<CardContent class="p-0">
					<DataTable
						columns={ipColumns}
						data={ipBlocks}
						pageSize={20}
						emptyMessage="No IP blocks configured"
					/>
				</CardContent>
			</Card>
		{/if}
	{/if}

	<!-- Censored Words Tab -->
	{#if activeTab === 'censored-words'}
		<div class="flex items-center justify-between">
			<p class="text-sm text-gray-500">{censoredWords.length} patterns configured</p>
			<Button
				onclick={() => {
					cwForm = { pattern: '', replacement: '' };
					showCwModal = true;
				}}
			>
				<Plus class="h-4 w-4 mr-2" />
				Add Pattern
			</Button>
		</div>

		{#if loading}
			<Card>
				<CardContent class="py-12">
					<div class="flex items-center justify-center text-gray-500">
						<RefreshCw class="h-5 w-5 mr-2 animate-spin" />
						Loading...
					</div>
				</CardContent>
			</Card>
		{:else}
			<Card>
				<CardContent class="p-0">
					<DataTable
						columns={cwColumns}
						data={censoredWords}
						pageSize={20}
						emptyMessage="No censored words configured"
					/>
				</CardContent>
			</Card>
		{/if}
	{/if}

	<!-- User Blocks Tab -->
	{#if activeTab === 'user-blocks'}
		{#if loading}
			<Card>
				<CardContent class="py-12">
					<div class="flex items-center justify-center text-gray-500">
						<RefreshCw class="h-5 w-5 mr-2 animate-spin" />
						Loading...
					</div>
				</CardContent>
			</Card>
		{:else}
			<Card>
				<CardContent class="p-0">
					<DataTable
						columns={ubColumns}
						data={userBlocks}
						pageSize={20}
						emptyMessage="No user blocks"
					/>
				</CardContent>
			</Card>
		{/if}
	{/if}
</div>

<!-- Add IP Block Modal -->
<Modal
	bind:open={showIpModal}
	title="Add IP Block"
	description="Block an IP address or CIDR range from accessing the platform."
	size="default"
>
	<div class="space-y-4">
		<div>
			<label for="ip-cidr" class="block text-sm font-medium text-gray-700">IP / CIDR</label>
			<input
				id="ip-cidr"
				type="text"
				class="mt-1 block w-full rounded-lg border border-gray-300 px-3 py-2 text-sm placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500"
				placeholder="192.168.1.0/24"
				bind:value={ipForm.cidr}
			/>
		</div>
		<div>
			<label for="ip-block-type" class="block text-sm font-medium text-gray-700">Block Type</label>
			<select
				id="ip-block-type"
				class="mt-1 block w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary-500"
				bind:value={ipForm.block_type}
			>
				<option value="full">Full</option>
				<option value="registration">Registration</option>
				<option value="posting">Posting</option>
				<option value="api">API</option>
			</select>
		</div>
		<div>
			<label for="ip-reason" class="block text-sm font-medium text-gray-700">Reason</label>
			<input
				id="ip-reason"
				type="text"
				class="mt-1 block w-full rounded-lg border border-gray-300 px-3 py-2 text-sm placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500"
				placeholder="Spam source"
				bind:value={ipForm.reason}
			/>
		</div>
		<div>
			<label for="ip-expires" class="block text-sm font-medium text-gray-700"
				>Expires At (optional)</label
			>
			<input
				id="ip-expires"
				type="datetime-local"
				class="mt-1 block w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary-500"
				bind:value={ipForm.expires_at}
			/>
		</div>
	</div>

	<div class="mt-6 flex justify-end gap-3">
		<Button variant="outline" onclick={() => (showIpModal = false)}>Cancel</Button>
		<Button onclick={submitIpBlock} disabled={ipSaving}>
			{#if ipSaving}
				<RefreshCw class="h-4 w-4 mr-2 animate-spin" />
				Saving...
			{:else}
				Add Block
			{/if}
		</Button>
	</div>
</Modal>

<!-- Add Censored Word Modal -->
<Modal
	bind:open={showCwModal}
	title="Add Censored Pattern"
	description="Add a word or phrase to be automatically blocked or censored."
	size="default"
>
	<div class="space-y-4">
		<div>
			<label for="cw-word" class="block text-sm font-medium text-gray-700">Pattern</label>
			<input
				id="cw-word"
				type="text"
				class="mt-1 block w-full rounded-lg border border-gray-300 px-3 py-2 text-sm placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500"
				placeholder="badword"
				bind:value={cwForm.pattern}
			/>
		</div>
		<div>
			<label for="cw-replacement" class="block text-sm font-medium text-gray-700"
				>Replacement (optional)</label
			>
			<input
				id="cw-replacement"
				type="text"
				class="mt-1 block w-full rounded-lg border border-gray-300 px-3 py-2 text-sm placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500"
				placeholder="***"
				bind:value={cwForm.replacement}
			/>
		</div>
	</div>

	<div class="mt-6 flex justify-end gap-3">
		<Button variant="outline" onclick={() => (showCwModal = false)}>Cancel</Button>
		<Button onclick={submitCensoredWord} disabled={cwSaving}>
			{#if cwSaving}
				<RefreshCw class="h-4 w-4 mr-2 animate-spin" />
				Saving...
			{:else}
				Add Pattern
			{/if}
		</Button>
	</div>
</Modal>

<!-- Delete Confirmation Modal -->
<Modal
	bind:open={showDeleteConfirm}
	title="Confirm Delete"
	description="This action cannot be undone."
	size="sm"
>
	<p class="text-sm text-gray-500">
		Are you sure you want to delete this item?
	</p>

	<div class="mt-6 flex justify-end gap-3">
		<Button variant="outline" onclick={() => (showDeleteConfirm = false)}>Cancel</Button>
		<Button variant="destructive" onclick={executeDelete}>
			<Trash2 class="h-4 w-4 mr-2" />
			Delete
		</Button>
	</div>
</Modal>
