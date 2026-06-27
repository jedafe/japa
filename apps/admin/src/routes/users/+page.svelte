<script lang="ts">
	import { onMount } from 'svelte';
	import FilterBar from '$lib/components/FilterBar.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import { api } from '$lib/api';

	type User = {
		id: string;
		username: string;
		email: string;
		groups: string[];
		status: string;
		created_at: string;
	};

	let users = $state<User[]>([]);
	let page = $state(0);
	let totalPages = $state(1);
	let loading = $state(true);
	let searchQuery = $state('');
	let filterGroup = $state('');

	// Warning/ban modal
	let showModal = $state(false);
	let modalMode = $state<'warn' | 'ban'>('warn');
	let selectedUser = $state<User | null>(null);
	let warnMessage = $state('');
	let banDuration = $state('7d');

	const columns = [
		{ key: 'username', label: 'Username' },
		{ key: 'email', label: 'Email' },
		{ key: 'groups', label: 'Groups' },
		{ key: 'status', label: 'Status' },
		{ key: 'created_at', label: 'Joined' },
		{ key: 'actions', label: '' }
	];

	const filterDefs = [
		{ key: 'search', label: 'Search', type: 'text' as const },
		{ key: 'group', label: 'Group', type: 'select' as const, options: ['user', 'moderator', 'admin', 'super_admin'] }
	];

	async function load() {
		loading = true;
		const params = new URLSearchParams({ page: String(page) });
		if (searchQuery) params.set('search', searchQuery);
		if (filterGroup) params.set('group', filterGroup);
		try {
			const data = await api<{ users: User[]; page: number; total_pages: number }>(`/admin/users?${params}`);
			users = data.users;
			totalPages = data.total_pages;
		} catch {
			// API not available
		} finally {
			loading = false;
		}
	}

	function handleFilter(values: Record<string, string>) {
		searchQuery = values.search ?? '';
		filterGroup = values.group ?? '';
		page = 0;
		load();
	}

	function openWarn(user: User) {
		selectedUser = user;
		modalMode = 'warn';
		warnMessage = '';
		showModal = true;
	}

	function openBan(user: User) {
		selectedUser = user;
		modalMode = 'ban';
		banDuration = '7d';
		showModal = true;
	}

	async function submitAction() {
		if (!selectedUser) return;
		if (modalMode === 'warn') {
			await api(`/admin/users/${selectedUser.id}/warn`, { method: 'POST', body: { message: warnMessage } });
		} else {
			await api(`/admin/users/${selectedUser.id}/ban`, { method: 'POST', body: { duration: banDuration } });
		}
		showModal = false;
		load();
	}

	function handlePageChange(p: number) {
		page = p;
		load();
	}

	onMount(load);
</script>

<svelte:head>
	<title>Users — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">Users</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">Manage users, warnings, and bans</p>

<FilterBar filters={filterDefs} onApply={handleFilter} />

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">Loading...</p>
{:else}
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-color: var(--color-border)">
					{#each columns as col}
						<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">
							{col.label}
						</th>
					{/each}
				</tr>
			</thead>
			<tbody>
				{#each users as user}
					<tr class="border-t" style="border-color: var(--color-border)">
						<td class="px-4 py-3 font-medium">{user.username}</td>
						<td class="px-4 py-3" style="color: var(--color-muted)">{user.email}</td>
						<td class="px-4 py-3">
							{#each user.groups as g}
								<span class="mr-1 inline-block rounded-full bg-black/5 px-2 py-0.5 text-xs dark:bg-white/10">{g}</span>
							{/each}
						</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs {user.status === 'active' ? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400' : 'bg-red-500/15 text-red-700 dark:text-red-400'}">
								{user.status}
							</span>
						</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{user.created_at}</td>
						<td class="px-4 py-3">
							<div class="flex gap-1">
								<button
									class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
									style="color: var(--color-warning)"
									onclick={() => openWarn(user)}
								>
									Warn
								</button>
								<button
									class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
									style="color: var(--color-danger)"
									onclick={() => openBan(user)}
								>
									Ban
								</button>
							</div>
						</td>
					</tr>
				{:else}
					<tr>
						<td colspan={columns.length} class="px-4 py-8 text-center" style="color: var(--color-muted)">
							No users found
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>

	<div class="mt-3 flex items-center justify-between">
		<p class="text-xs" style="color: var(--color-muted)">Page {page + 1} of {totalPages}</p>
		<div class="flex gap-1">
			<button
				class="rounded p-1 hover:bg-black/5 dark:hover:bg-white/10 disabled:opacity-40"
				disabled={page === 0}
				onclick={() => handlePageChange(page - 1)}
			>&larr;</button>
			<button
				class="rounded p-1 hover:bg-black/5 dark:hover:bg-white/10 disabled:opacity-40"
				disabled={page >= totalPages - 1}
				onclick={() => handlePageChange(page + 1)}
			>&rarr;</button>
		</div>
	</div>
{/if}

<!-- Warn/Ban Modal -->
<Modal bind:open={showModal} title={modalMode === 'warn' ? 'Warn User' : 'Ban User'} onConfirm={submitAction} confirmLabel={modalMode === 'warn' ? 'Send Warning' : 'Apply Ban'}>
	{#if selectedUser}
		<p class="text-sm mb-3">Action for <strong>{selectedUser.username}</strong></p>
		{#if modalMode === 'warn'}
			<textarea
				class="w-full rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-surface)"
				rows="3"
				placeholder="Warning message..."
				bind:value={warnMessage}
			></textarea>
		{:else}
			<label for="ban-duration" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Duration</label>
			<select
				id="ban-duration"
				class="w-full rounded-md border px-3 py-1.5 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-surface)"
				bind:value={banDuration}
			>
				<option value="1d">1 day</option>
				<option value="7d">7 days</option>
				<option value="30d">30 days</option>
				<option value="permanent">Permanent</option>
			</select>
		{/if}
	{/if}
</Modal>
