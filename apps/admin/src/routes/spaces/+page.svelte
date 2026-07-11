<script lang="ts">
	import { onMount } from 'svelte';
	import FilterBar from '$lib/components/FilterBar.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

	type Space = {
		id: string;
		name: string;
		owner: string;
		owner_id: string;
		member_count: number;
		status: string;
		space_type: string;
		created_at: string;
	};

	let spaces = $state<Space[]>([]);
	let page = $state(0);
	let totalPages = $state(1);
	let loading = $state(true);
	let filterStatus = $state('');
	let errorMessage = $state('');

	// Suspend modal
	let showSuspendModal = $state(false);
	let suspendTarget = $state<Space | null>(null);
	let suspendReason = $state('');
	let acting = $state(false);

	// Assign moderator modal
	let showModModal = $state(false);
	let modTarget = $state<Space | null>(null);
	let modUsername = $state('');

	const filterDefs = [
		{ key: 'status', label: t('spaces.status'), type: 'select' as const, options: ['active', 'suspended', 'archived'] }
	];

	async function load() {
		loading = true;
		errorMessage = '';
		const params = new URLSearchParams();
		if (filterStatus) params.set('status', filterStatus);
		try {
			const result = await api<{ data: Space[] }>(`/spaces?${params}`);
			if (result.ok) {
				const list = Array.isArray(result.data) ? result.data : (result.data.data ?? []);
				spaces = filterStatus ? list.filter((s) => s.status === filterStatus) : list;
				totalPages = 1;
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	function handleFilter(values: Record<string, string>) {
		filterStatus = values.status ?? '';
		page = 0;
		load();
	}

	function openSuspend(space: Space) {
		suspendTarget = space;
		suspendReason = '';
		showSuspendModal = true;
	}

	async function executeSuspend() {
		if (!suspendTarget) return;
		acting = true;
		errorMessage = '';
		try {
			const result = await api(`/admin/spaces/${suspendTarget.id}/status`, {
				method: 'PATCH',
				body: { status: 'suspended', reason: suspendReason.trim() }
			});
			if (result.ok) {
				showSuspendModal = false;
				load();
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			acting = false;
		}
	}

	function openAssignMod(space: Space) {
		modTarget = space;
		modUsername = '';
		showModModal = true;
	}

	async function executeAssignMod() {
		if (!modTarget || !modUsername.trim()) return;
		acting = true;
		errorMessage = '';
		try {
			const result = await api(`/admin/spaces/${modTarget.id}/moderators`, {
				method: 'POST',
				body: { username: modUsername.trim() }
			});
			if (result.ok) {
				showModModal = false;
				load();
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			acting = false;
		}
	}

	function handlePageChange(p: number) {
		page = p;
		load();
	}

	function statusBadge(status: string) {
		switch (status) {
			case 'active': return 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400';
			case 'suspended': return 'bg-red-500/15 text-red-700 dark:text-red-400';
			default: return 'bg-slate-500/15 text-slate-700 dark:text-slate-400';
		}
	}

	onMount(load);
</script>

<svelte:head>
	<title>{t('spaces.title')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('spaces.title')}</h1>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}
<p class="mb-4 text-sm" style="color: var(--color-muted)">Manage spaces, members, and moderation settings</p>

<FilterBar filters={filterDefs} onApply={handleFilter} />

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-color: var(--color-border)">
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('spaces.name')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('spaces.owner')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('spaces.members')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('spaces.type')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('spaces.status')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('spaces.created')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)"></th>
				</tr>
			</thead>
			<tbody>
				{#each spaces as space}
					<tr class="border-t hover:bg-black/[0.02] dark:hover:bg-white/[0.02]" style="border-color: var(--color-border)">
						<td class="px-4 py-3">
							<a href="/spaces/{space.id}" class="font-medium hover:underline">{space.name}</a>
						</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{space.owner}</td>
						<td class="px-4 py-3 text-xs font-medium">{space.member_count.toLocaleString()}</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{space.space_type || '—'}</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs {statusBadge(space.status)}">
								{space.status}
							</span>
						</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{space.created_at}</td>
						<td class="px-4 py-3">
							<div class="flex items-center gap-1">
								{#if space.status === 'active'}
									<button
										class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
										style="color: var(--color-warning)"
										onclick={() => openSuspend(space)}
									>
										Suspend
									</button>
								{/if}
								<button
									class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
									style="color: var(--color-info)"
									onclick={() => openAssignMod(space)}
								>
									Moderator
								</button>
								<a
									href="/spaces/{space.id}/rules"
									class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
									style="color: var(--color-muted)"
								>
									{t('spaces.view_members')}
								</a>
							</div>
						</td>
					</tr>
				{:else}
					<tr>
						<td colspan="7" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							No spaces found
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>

	<div class="mt-3 flex items-center justify-between">
		<p class="text-xs" style="color: var(--color-muted)">Page {page + 1} of {totalPages}</p>
		<div class="flex gap-1">
			<button class="rounded p-1 hover:bg-black/5 dark:hover:bg-white/10 disabled:opacity-40" disabled={page === 0} onclick={() => handlePageChange(page - 1)}>&larr;</button>
			<button class="rounded p-1 hover:bg-black/5 dark:hover:bg-white/10 disabled:opacity-40" disabled={page >= totalPages - 1} onclick={() => handlePageChange(page + 1)}>&rarr;</button>
		</div>
	</div>
{/if}

<!-- Suspend Modal -->
<Modal bind:open={showSuspendModal} title="Suspend Space" onConfirm={executeSuspend} confirmLabel={acting ? t('common.saving') : 'Suspend'}>
	<p class="text-sm mb-3" style="color: var(--color-muted)">
		This will suspend <strong>{suspendTarget?.name}</strong> and prevent new content from being posted.
	</p>
	<textarea
		class="w-full rounded-md border px-3 py-2 text-sm"
		style="border-color: var(--color-border); background-color: var(--color-surface)"
		rows="3"
		placeholder="Suspension reason (required)"
		bind:value={suspendReason}
	></textarea>
</Modal>

<!-- Assign Moderator Modal -->
<Modal bind:open={showModModal} title="Assign Moderator" onConfirm={executeAssignMod} confirmLabel={acting ? t('common.saving') : 'Assign'}>
	<p class="text-sm mb-3" style="color: var(--color-muted)">
		Assign a moderator to <strong>{modTarget?.name}</strong>.
	</p>
	<div>
		<label for="mod-username" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Username</label>
		<input
			id="mod-username"
			type="text"
			class="w-full rounded-md border px-3 py-2 text-sm"
			style="border-color: var(--color-border); background-color: var(--color-surface)"
			placeholder="Enter username"
			bind:value={modUsername}
		/>
	</div>
</Modal>
