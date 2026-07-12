<script lang="ts">
	import { onMount } from 'svelte';
	import FilterBar from '$lib/components/FilterBar.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

	type Appeal = {
		id: string;
		user_id: string;
		username: string;
		original_action: string;
		original_reason: string;
		appeal_reason: string;
		status: string;
		created_at: string;
		reviewed_at?: string;
	};

	let appeals = $state<Appeal[]>([]);
	let page = $state(0);
	let totalPages = $state(1);
	let loading = $state(true);
	let filterStatus = $state('');
	let errorMessage = $state('');

	// Expandable detail
	let expandedId = $state<string | null>(null);

	// Action modal
	let showActionModal = $state(false);
	let actionAppeal = $state<Appeal | null>(null);
	let actionType = $state<'approve' | 'deny'>('approve');
	let actionNote = $state('');
	let acting = $state(false);

	const filterDefs = [
		{ key: 'status', label: t('moderation.status'), type: 'select' as const, options: ['pending', 'approved', 'denied'] }
	];

	async function load() {
		loading = true;
		errorMessage = '';
		const params = new URLSearchParams({ page: String(page) });
		if (filterStatus) params.set('status', filterStatus);
		try {
			const result = await api<{ items: Appeal[]; page: number; total_pages: number }>(`/admin/appeals?${params}`);
			if (result.ok) {
				appeals = result.data.items;
				totalPages = result.data.total_pages;
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

	function toggleExpand(id: string) {
		expandedId = expandedId === id ? null : id;
	}

	function openAction(appeal: Appeal, type: 'approve' | 'deny') {
		actionAppeal = appeal;
		actionType = type;
		actionNote = '';
		showActionModal = true;
	}

	async function executeAction() {
		if (!actionAppeal) return;
		acting = true;
		errorMessage = '';
		try {
			const endpoint = `/admin/appeals/${actionAppeal.id}`;
			const body: Record<string, unknown> = { status: actionType === 'approve' ? 'approved' : 'denied' };
			if (actionNote.trim()) body.note = actionNote.trim();
			const result = await api(endpoint, { method: 'PATCH', body });
			if (result.ok) {
				showActionModal = false;
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
			case 'pending': return 'bg-amber-500/15 text-amber-700 dark:text-amber-400';
			case 'approved': return 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400';
			case 'denied': return 'bg-red-500/15 text-red-700 dark:text-red-400';
			default: return 'bg-slate-500/15 text-slate-700 dark:text-slate-400';
		}
	}

	function actionBadgeColor(action: string) {
		switch (action) {
			case 'ban': return 'bg-red-500/15 text-red-700 dark:text-red-400';
			case 'suspend': return 'bg-amber-500/15 text-amber-700 dark:text-amber-400';
			case 'remove_content': return 'bg-orange-500/15 text-orange-700 dark:text-orange-400';
			default: return 'bg-slate-500/15 text-slate-700 dark:text-slate-400';
		}
	}

	onMount(load);
</script>

<svelte:head>
	<title>{t('moderation.appeals')} — JapaConnect Admin</title>
</svelte:head>

<div class="mb-4">
	<a href="/moderation" class="text-xs hover:underline" style="color: var(--color-muted)">&larr; {t('moderation.back')}</a>
</div>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('moderation.appeals')}</h1>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}
<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('moderation.appeals_description')}</p>

<FilterBar filters={filterDefs} onApply={handleFilter} />

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-color: var(--color-border)">
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('moderation.user')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('moderation.original_action')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('moderation.appeal_reason')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('moderation.status')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('moderation.date')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)"></th>
				</tr>
			</thead>
			<tbody>
				{#each appeals as appeal}
					<tr
						class="border-t cursor-pointer hover:bg-black/[0.02] dark:hover:bg-white/[0.02]"
						style="border-color: var(--color-border)"
						onclick={() => toggleExpand(appeal.id)}
					>
						<td class="px-4 py-3">
							<p class="font-medium">{appeal.username}</p>
							<p class="text-xs" style="color: var(--color-muted)">{appeal.user_id.slice(0, 12)}</p>
						</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs {actionBadgeColor(appeal.original_action)}">
								{appeal.original_action}
							</span>
						</td>
						<td class="px-4 py-3 text-xs max-w-xs truncate">{appeal.appeal_reason}</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs {statusBadge(appeal.status)}">
								{appeal.status}
							</span>
						</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{appeal.created_at}</td>
						<td class="px-4 py-3" onclick={(e) => e.stopPropagation()}>
							{#if appeal.status === 'pending'}
								<div class="flex items-center gap-1">
									<button
										class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
										style="color: var(--color-success)"
										onclick={() => openAction(appeal, 'approve')}
									>
										{t('moderation.approve')}
									</button>
									<button
										class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
										style="color: var(--color-danger)"
										onclick={() => openAction(appeal, 'deny')}
									>
										{t('moderation.deny')}
									</button>
								</div>
							{/if}
						</td>
					</tr>
					{#if expandedId === appeal.id}
						<tr style="border-color: var(--color-border)">
							<td colspan="6" class="px-4 py-4" style="background-color: var(--color-surface)">
								<div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
									<div>
										<p class="text-xs font-medium" style="color: var(--color-muted)">Original Action</p>
										<p class="mt-1">
											<span class="inline-block rounded-full px-2 py-0.5 text-xs {actionBadgeColor(appeal.original_action)}">
												{appeal.original_action}
											</span>
										</p>
										<p class="mt-1 text-sm" style="color: var(--color-muted)">{appeal.original_reason || 'No reason provided'}</p>
									</div>
									<div>
										<p class="text-xs font-medium" style="color: var(--color-muted)">Appeal Reason</p>
										<p class="mt-1 text-sm">{appeal.appeal_reason}</p>
									</div>
								</div>
								{#if appeal.reviewed_at}
									<p class="mt-3 text-xs" style="color: var(--color-muted)">Reviewed at: {appeal.reviewed_at}</p>
								{/if}
								{#if appeal.status === 'pending'}
									<div class="mt-3 flex gap-2">
										<button
											class="rounded-md px-3 py-1.5 text-xs font-medium text-white"
											style="background-color: var(--color-success)"
											onclick={() => openAction(appeal, 'approve')}
										>
											Approve Appeal (Reverse Action)
										</button>
										<button
											class="rounded-md px-3 py-1.5 text-xs font-medium text-white"
											style="background-color: var(--color-danger)"
											onclick={() => openAction(appeal, 'deny')}
										>
											Deny Appeal
										</button>
									</div>
								{/if}
							</td>
						</tr>
					{/if}
				{:else}
					<tr>
						<td colspan="6" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							No appeals found
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

<!-- Action Modal -->
<Modal
	bind:open={showActionModal}
	title={actionType === 'approve' ? 'Approve Appeal' : 'Deny Appeal'}
	onConfirm={executeAction}
	confirmLabel={acting ? t('common.saving') : actionType === 'approve' ? 'Approve' : 'Deny'}
>
	<p class="text-sm mb-3" style="color: var(--color-muted)">
		{actionType === 'approve'
			? 'This will reverse the original action and restore the user\'s access.'
			: 'This will deny the appeal and keep the original action in place.'}
	</p>
	<textarea
		class="w-full rounded-md border px-3 py-2 text-sm"
		style="border-color: var(--color-border); background-color: var(--color-surface)"
		rows="3"
		placeholder="Note (optional)"
		bind:value={actionNote}
	></textarea>
</Modal>
