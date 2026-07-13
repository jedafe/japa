<script lang="ts">
	import { onMount } from 'svelte';
	import FilterBar from '$lib/components/FilterBar.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import { api, setToken, impersonationApi, userManagementApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Users, Shield, UsersRound } from 'lucide-svelte';
	import { page as pageState } from '$app/state';

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
	let groupOptions = $state<string[]>(['user', 'moderator', 'admin', 'super_admin']);

	// Impersonation state
	let impersonating = $state(false);
	let impersonatingUsername = $state('');
	let impersonateTarget = $state<User | null>(null);
	let showImpersonateConfirm = $state(false);
	let impersonateError = $state('');
	
	// Tab navigation - derived from current URL
	let activeTab = $derived.by(() => {
		const path = pageState.url.pathname;
		if (path === '/users') return 'users';
		if (path.startsWith('/users/roles')) return 'roles';
		if (path.startsWith('/users/groups')) return 'groups';
		return 'users';
	});

	const columns = [
		{ key: 'username', label: 'users.username' },
		{ key: 'email', label: 'users.email' },
		{ key: 'groups', label: 'users.groups' },
		{ key: 'status', label: 'users.status' },
		{ key: 'created_at', label: 'users.joined' }
	];

	let filterDefs = $derived([
		{ key: 'search', label: 'users.search', type: 'text' as const },
		{ key: 'group', label: 'users.group_filter', type: 'select' as const, options: groupOptions }
	]);

	async function load() {
		loading = true;
		const params = new URLSearchParams({ page: String(page) });
		if (searchQuery) params.set('search', searchQuery);
		if (filterGroup) params.set('group', filterGroup);
		try {
			const result = await api<{ users: User[]; page: number; total_pages: number }>(`/users?${params}`);
			if (result.ok) {
				users = result.data.users;
				totalPages = result.data.total_pages;
			}
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

	function handlePageChange(p: number) {
		page = p;
		load();
	}

	async function checkImpersonationStatus() {
		const result = await impersonationApi.status();
		if (result.ok) {
			const data = result.data as { impersonating: boolean; username?: string };
			impersonating = data.impersonating;
			impersonatingUsername = data.username ?? '';
		}
	}

	function startImpersonate(user: User) {
		impersonateTarget = user;
		impersonateError = '';
		showImpersonateConfirm = true;
	}

	async function confirmImpersonate() {
		if (!impersonateTarget) return;
		impersonateError = '';
		const result = await impersonationApi.start(impersonateTarget.id);
		if (result.ok) {
			const data = result.data as { token: string };
			setToken(data.token);
			showImpersonateConfirm = false;
			// Redirect to main app
			window.location.href = '/';
		} else {
			impersonateError = result.error;
		}
	}

	async function endImpersonation() {
		const result = await impersonationApi.end();
		if (result.ok) {
			window.location.reload();
		}
	}

	onMount(async () => {
		try {
			const groupsResult = await userManagementApi.listGroups();
			if (groupsResult.ok) {
				groupOptions = groupsResult.data.groups.map((g) => g.name);
			}
		} catch {
			/* group filter options are optional — the list still loads */
		}
		load();
		checkImpersonationStatus();
	});
</script>

<!-- Impersonation Banner -->
{#if impersonating}
	<div class="mb-4 flex items-center justify-between rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-warning); background-color: color-mix(in srgb, var(--color-warning) 10%, transparent)">
		<span style="color: var(--color-warning)">You are impersonating <strong>{impersonatingUsername}</strong></span>
		<button
			class="rounded-md border px-3 py-1 text-xs font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
			style="border-color: var(--color-warning); color: var(--color-warning)"
			onclick={endImpersonation}
		>
			End Impersonation
		</button>
	</div>
{/if}

<svelte:head>
	<title>Users — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('users.title')}</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('users.description')}</p>

<!-- Sub-navigation tabs -->
<div class="mb-6 flex gap-1 rounded-lg border p-1" style="border-color: var(--color-border)">
	<a
		href="/users"
		class="flex items-center gap-2 rounded-md px-4 py-2 text-sm font-medium transition-colors"
		style={activeTab === 'users' ? 'background-color: var(--color-primary); color: white' : ''}
	>
		<Users class="h-4 w-4" />
		{t('users.list')}
	</a>
	<a
		href="/users/roles"
		class="flex items-center gap-2 rounded-md px-4 py-2 text-sm font-medium transition-colors"
		style={activeTab === 'roles' ? 'background-color: var(--color-primary); color: white' : ''}
	>
		<Shield class="h-4 w-4" />
		{t('users.roles.title')}
	</a>
	<a
		href="/users/groups"
		class="flex items-center gap-2 rounded-md px-4 py-2 text-sm font-medium transition-colors"
		style={activeTab === 'groups' ? 'background-color: var(--color-primary); color: white' : ''}
	>
		<UsersRound class="h-4 w-4" />
		{t('users.groups.title')}
	</a>
</div>

{#if impersonateError}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{impersonateError}
	</div>
{/if}

<FilterBar filters={filterDefs} onApply={handleFilter} />

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
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
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Actions</th>
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
							<button
								class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
								style="color: var(--color-warning)"
								onclick={() => startImpersonate(user)}
							>
								Impersonate
							</button>
						</td>
					</tr>
				{:else}
					<tr>
						<td colspan={columns.length + 1} class="px-4 py-8 text-center" style="color: var(--color-muted)">
							{t('users.empty')}
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

<!-- Impersonation Confirmation Modal -->
<Modal bind:open={showImpersonateConfirm} title="Confirm Impersonation" onConfirm={confirmImpersonate} confirmLabel="Impersonate">
	{#if impersonateTarget}
		<p class="text-sm" style="color: var(--color-muted)">
			You are about to impersonate <strong>{impersonateTarget.username}</strong>. You will be redirected to the main app as this user. Your admin session will be preserved.
		</p>
	{/if}
</Modal>


