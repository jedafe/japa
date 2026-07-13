<script lang="ts">
	import { onMount } from 'svelte';
	import FilterBar from '$lib/components/FilterBar.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import { api, contentAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

	type ContentItem = {
		id: string;
		title: string;
		content_type: string;
		status: string;
		author_id: string;
		author_name: string;
		space_name?: string;
		created_at: string;
		removed_at?: string;
	};

	let items = $state<ContentItem[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');
	let activeTab = $state<'all' | 'removed'>('all');

	let showRemoveModal = $state(false);
	let removeTarget = $state<ContentItem | null>(null);
	let removeReason = $state('');
	let removing = $state(false);

	let showRestoreModal = $state(false);
	let restoreTarget = $state<ContentItem | null>(null);
	let restoring = $state(false);

	const filterDefs = [
		{ key: 'status', label: 'content.status', type: 'select' as const, options: ['published', 'draft', 'removed', 'archived'] },
		{ key: 'content_type', label: 'content.type', type: 'select' as const, options: ['post', 'comment', 'blog', 'forum', 'page'] }
	];

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const result = await api<ContentItem[]>('/admin/content');
			if (result.ok) {
				items = result.data ?? [];
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	function filteredItems() {
		if (activeTab === 'removed') {
			return items.filter((i) => i.status === 'removed' || i.removed_at);
		}
		return items.filter((i) => i.status !== 'removed' && !i.removed_at);
	}

	function handleFilter(values: Record<string, string>) {
		load();
	}

	function openRemove(item: ContentItem) {
		removeTarget = item;
		removeReason = '';
		showRemoveModal = true;
	}

	async function confirmRemove() {
		if (!removeTarget) return;
		removing = true;
		errorMessage = '';
		try {
			const result = await api(`/admin/content/${removeTarget.id}/remove`, { method: 'POST', body: { reason: removeReason } });
			if (result.ok) {
				showRemoveModal = false;
				load();
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			removing = false;
		}
	}

	function openRestore(item: ContentItem) {
		restoreTarget = item;
		showRestoreModal = true;
	}

	async function confirmRestore() {
		if (!restoreTarget) return;
		restoring = true;
		errorMessage = '';
		try {
			const result = await api(`/admin/content/${restoreTarget.id}/restore`, { method: 'POST', body: {} });
			if (result.ok) {
				showRestoreModal = false;
				load();
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			restoring = false;
		}
	}

	function statusColor(status: string) {
		switch (status) {
			case 'published':
				return 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400';
			case 'draft':
				return 'bg-amber-500/15 text-amber-700 dark:text-amber-400';
			case 'removed':
				return 'bg-red-500/15 text-red-700 dark:text-red-400';
			case 'archived':
				return 'bg-slate-500/15 text-slate-700 dark:text-slate-400';
			default:
				return 'bg-slate-500/15 text-slate-700 dark:text-slate-400';
		}
	}

	onMount(load);
</script>

<svelte:head>
	<title>{t('content.title')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('content.title')}</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('content.description')}</p>

<!-- Tabs -->
<div class="mb-6 flex gap-1 rounded-lg border p-1" style="border-color: var(--color-border)">
	<button
		class="rounded-md px-4 py-2 text-sm font-medium transition-colors"
		style={activeTab === 'all' ? 'background-color: var(--color-primary); color: white' : ''}
		onclick={() => { activeTab = 'all'; }}
	>
		{t('content.all')}
	</button>
	<button
		class="rounded-md px-4 py-2 text-sm font-medium transition-colors"
		style={activeTab === 'removed' ? 'background-color: var(--color-primary); color: white' : ''}
		onclick={() => { activeTab = 'removed'; }}
	>
		{t('content.removed')}
	</button>
</div>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-bottom: 1px solid var(--color-border)">
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('content.title')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('content.type')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('content.author')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('content.space')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('content.status')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('content.created')}</th>
					<th class="px-4 py-3 text-right font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('common.actions')}</th>
				</tr>
			</thead>
			<tbody>
				{#each filteredItems() as item (item.id)}
					<tr class="border-t" style="border-color: var(--color-border)">
						<td class="px-4 py-3 font-medium">{item.title || item.id.slice(0, 12)}</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs" style="background-color: var(--color-surface); color: var(--color-muted)">
								{item.content_type}
							</span>
						</td>
						<td class="px-4 py-3 text-xs">{item.author_name || item.author_id.slice(0, 8)}</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{item.space_name || '—'}</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium {statusColor(item.status)}">
								{item.status}
							</span>
						</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{new Date(item.created_at).toLocaleDateString()}</td>
						<td class="px-4 py-3 text-right">
							{#if item.status === 'removed' || item.removed_at}
								<button
									class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
									style="color: var(--color-success)"
									onclick={() => openRestore(item)}
								>
									{t('content.restore')}
								</button>
							{:else}
								<button
									class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
									style="color: var(--color-warning)"
									onclick={() => openRemove(item)}
								>
									{t('content.remove')}
								</button>
							{/if}
						</td>
					</tr>
				{:else}
					<tr>
						<td colspan="7" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							{activeTab === 'removed' ? t('content.no_removed') : t('content.empty')}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}

<!-- Remove Modal -->
<Modal bind:open={showRemoveModal} title={t('content.remove_title')} onConfirm={confirmRemove} confirmLabel={removing ? t('common.saving') : t('content.remove')}>
	<div class="space-y-3">
		<p class="text-sm" style="color: var(--color-muted)">
			{t('content.remove_confirm')}
		</p>
		{#if removeTarget}
			<div class="rounded-md border p-3 text-xs" style="border-color: var(--color-border); background-color: var(--color-surface)">
				<span class="font-medium">{removeTarget.title || removeTarget.id.slice(0, 12)}</span> &middot; {removeTarget.content_type}
			</div>
		{/if}
		<div>
			<label for="remove-reason" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('content.reason')}</label>
			<textarea
				id="remove-reason"
				bind:value={removeReason}
				rows="3"
				class="w-full rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-surface)"
				placeholder={t('content.remove_reason_placeholder')}
			></textarea>
		</div>
	</div>
</Modal>

<!-- Restore Modal -->
<Modal bind:open={showRestoreModal} title={t('content.restore_title')} onConfirm={confirmRestore} confirmLabel={restoring ? t('common.saving') : t('content.restore')}>
	<p class="text-sm" style="color: var(--color-muted)">
		{t('content.restore_confirm')}
	</p>
</Modal>
