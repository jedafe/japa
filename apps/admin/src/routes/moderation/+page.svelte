<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { RefreshCw } from 'lucide-svelte';
	import FilterBar from '$lib/components/FilterBar.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

	type ModItem = {
		id: string;
		type: 'user_report' | 'ai_flag';
		content_type: 'post' | 'comment' | 'blog' | 'forum' | 'message';
		content_id: string;
		reporter_id: string;
		reporter_name?: string;
		reason: string;
		status: string;
		created_at: string;
		content_preview?: string;
	};

	let items = $state<ModItem[]>([]);
	let page = $state(0);
	let totalPages = $state(1);
	let loading = $state(true);
	let filterType = $state('');
	let filterStatus = $state('');
	let filterContentType = $state('');
	let selectedIds = $state<Set<string>>(new Set());
	let errorMessage = $state('');

	// Polling state
	let lastCount = $state(0);
	let newItemsCount = $state(0);
	let isPolling = $state(false);
	let pollingInterval: ReturnType<typeof setInterval> | undefined;

	// Expandable detail
	let expandedId = $state<string | null>(null);

	// Action modal
	let showActionModal = $state(false);
	let actionItem = $state<ModItem | null>(null);
	let actionType = $state<'approve' | 'reject' | 'delete'>('approve');
	let actionReason = $state('');
	let acting = $state(false);

	const filterDefs = [
		{ key: 'type', label: t('moderation.source'), type: 'select' as const, options: ['user_report', 'ai_flag'] },
		{ key: 'content_type', label: t('moderation.content_type'), type: 'select' as const, options: ['post', 'comment', 'blog', 'forum', 'message'] },
		{ key: 'status', label: t('moderation.status'), type: 'select' as const, options: ['pending', 'reviewed', 'resolved', 'dismissed'] }
	];

	async function load() {
		loading = true;
		errorMessage = '';
		const params = new URLSearchParams({ page: String(page) });
		if (filterType) params.set('type', filterType);
		if (filterStatus) params.set('status', filterStatus);
		if (filterContentType) params.set('content_type', filterContentType);
		try {
			const result = await api<{ items: ModItem[]; page: number; total_pages: number }>(`/admin/moderation-queue?${params}`);
			if (result.ok) {
				items = result.data.items;
				totalPages = result.data.total_pages;
				lastCount = items.length;
				newItemsCount = 0;
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	function startPolling() {
		pollingInterval = setInterval(async () => {
			const params = new URLSearchParams({ page: '0' });
			if (filterType) params.set('type', filterType);
			if (filterStatus) params.set('status', filterStatus);
			if (filterContentType) params.set('content_type', filterContentType);
			try {
				const result = await api<{ items: ModItem[]; page: number; total_pages: number }>(`/admin/moderation-queue?${params}`);
				if (result.ok) {
					const newCount = result.data.items.length;
					if (lastCount > 0 && newCount > lastCount) {
						newItemsCount = newCount - lastCount;
					}
					lastCount = newCount;
				}
			} catch {
				// Silently fail polling — user can manually refresh
			}
		}, 30_000);
		isPolling = true;
	}

	function stopPolling() {
		if (pollingInterval !== undefined) {
			clearInterval(pollingInterval);
			pollingInterval = undefined;
		}
		isPolling = false;
	}

	function refreshList() {
		newItemsCount = 0;
		load();
	}

	function handleFilter(values: Record<string, string>) {
		filterType = values.type ?? '';
		filterContentType = values.content_type ?? '';
		filterStatus = values.status ?? '';
		page = 0;
		load();
	}

	function toggleExpand(id: string) {
		expandedId = expandedId === id ? null : id;
	}

	function toggleSelect(id: string) {
		const next = new Set(selectedIds);
		if (next.has(id)) next.delete(id);
		else next.add(id);
		selectedIds = next;
	}

	function toggleAll() {
		if (selectedIds.size === items.length) {
			selectedIds = new Set();
		} else {
			selectedIds = new Set(items.map((i) => i.id));
		}
	}

	function openAction(item: ModItem, action: 'approve' | 'reject' | 'delete') {
		actionItem = item;
		actionType = action;
		actionReason = '';
		showActionModal = true;
	}

	async function executeAction() {
		if (!actionItem) return;
		acting = true;
		errorMessage = '';
		const endpoint = `/admin/moderation-queue/${actionItem.id}`;
		try {
			const body: Record<string, unknown> = { action: actionType };
			if (actionReason.trim()) body.reason = actionReason.trim();
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

	async function bulkAction(action: 'approve' | 'reject') {
		const ids = Array.from(selectedIds);
		errorMessage = '';
		try {
			await api('/admin/moderation-queue/bulk', { method: 'POST', body: { ids, action } });
			selectedIds = new Set();
			load();
		} catch {
			errorMessage = t('moderation.bulk_error');
		}
	}

	function handlePageChange(p: number) {
		page = p;
		load();
	}

	function typeBadge(type: string) {
		return type === 'ai_flag'
			? 'bg-purple-500/15 text-purple-700 dark:text-purple-400'
			: 'bg-blue-500/15 text-blue-700 dark:text-blue-400';
	}

	function statusBadge(status: string) {
		switch (status) {
			case 'pending': return 'bg-amber-500/15 text-amber-700 dark:text-amber-400';
			case 'reviewed': return 'bg-blue-500/15 text-blue-700 dark:text-blue-400';
			case 'resolved': return 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400';
			default: return 'bg-slate-500/15 text-slate-700 dark:text-slate-400';
		}
	}

	function contentTypeBadge(ct: string) {
		switch (ct) {
			case 'post': return 'bg-indigo-500/15 text-indigo-700 dark:text-indigo-400';
			case 'comment': return 'bg-cyan-500/15 text-cyan-700 dark:text-cyan-400';
			case 'blog': return 'bg-rose-500/15 text-rose-700 dark:text-rose-400';
			case 'forum': return 'bg-orange-500/15 text-orange-700 dark:text-orange-400';
			default: return 'bg-slate-500/15 text-slate-700 dark:text-slate-400';
		}
	}

	onMount(() => {
		load();
		// Initialize lastCount after first load completes
		setTimeout(() => {
			lastCount = items.length;
			startPolling();
		}, 1000);
	});

	onDestroy(() => {
		stopPolling();
	});
</script>

<svelte:head>
	<title>{t('moderation.title')} — JapaConnect Admin</title>
</svelte:head>

<div class="mb-1 flex items-center gap-2">
	<h1 class="text-2xl font-bold tracking-tight">{t('moderation.title')}</h1>
	{#if isPolling}
		<span class="relative flex h-2.5 w-2.5" title="Auto-refresh active">
			<span class="absolute inline-flex h-full w-full animate-ping rounded-full opacity-75" style="background-color: var(--color-success)"></span>
			<span class="relative inline-flex h-2.5 w-2.5 rounded-full" style="background-color: var(--color-success)"></span>
		</span>
	{/if}
</div>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

{#if newItemsCount > 0}
	<div class="mb-4 flex items-center gap-3 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-info); background-color: color-mix(in srgb, var(--color-info) 8%, transparent); color: var(--color-info)">
		<span class="font-medium">{newItemsCount} new {newItemsCount === 1 ? 'item' : 'items'}</span>
		<button
			class="inline-flex items-center gap-1.5 rounded px-2.5 py-1 text-xs font-medium text-white transition-opacity hover:opacity-90"
			style="background-color: var(--color-info)"
			onclick={refreshList}
		>
			<RefreshCw class="h-3 w-3" />
			Show
		</button>
	</div>
{/if}

<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('moderation.description')}</p>

<FilterBar filters={filterDefs} onApply={handleFilter} />

<!-- Bulk actions -->
{#if selectedIds.size > 0}
	<div class="mb-3 flex items-center gap-2 rounded-md border p-2" style="border-color: var(--color-border); background-color: var(--color-surface)">
		<span class="text-xs font-medium" style="color: var(--color-muted)">
			{selectedIds.size} {selectedIds.size === 1 ? 'item' : 'items'} selected
		</span>
		<button
			class="rounded px-2 py-1 text-xs font-medium hover:bg-black/5 dark:hover:bg-white/10"
			style="color: var(--color-success)"
			onclick={() => bulkAction('approve')}
		>
			{t('moderation.resolve')}
		</button>
		<button
			class="rounded px-2 py-1 text-xs font-medium hover:bg-black/5 dark:hover:bg-white/10"
			style="color: var(--color-warning)"
			onclick={() => bulkAction('reject')}
		>
			{t('moderation.dismiss')}
		</button>
	</div>
{/if}

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-color: var(--color-border)">
					<th class="px-4 py-3 text-left" style="background-color: var(--color-surface)">
						<input type="checkbox" checked={selectedIds.size === items.length && items.length > 0} onchange={toggleAll} />
					</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('moderation.source')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('moderation.content_type')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('moderation.content')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('moderation.reason')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('moderation.status')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('moderation.date')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)"></th>
				</tr>
			</thead>
			<tbody>
				{#each items as item}
					<!-- Main row -->
					<tr
						class="border-t cursor-pointer hover:bg-black/[0.02] dark:hover:bg-white/[0.02]"
						style="border-color: var(--color-border)"
						onclick={() => toggleExpand(item.id)}
					>
						<td class="px-4 py-3" onclick={(e) => e.stopPropagation()}>
							<input type="checkbox" checked={selectedIds.has(item.id)} onchange={() => toggleSelect(item.id)} />
						</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs {typeBadge(item.type)}">
								{item.type === 'ai_flag' ? 'AI Flag' : 'User Report'}
							</span>
						</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs {contentTypeBadge(item.content_type)}">
								{item.content_type}
							</span>
						</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{item.content_id.slice(0, 12)}</td>
						<td class="px-4 py-3 text-xs">{item.reason}</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs {statusBadge(item.status)}">
								{item.status}
							</span>
						</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{item.created_at}</td>
						<td class="px-4 py-3" onclick={(e) => e.stopPropagation()}>
							<div class="flex items-center gap-1">
								{#if item.status === 'pending'}
									<button
										class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
										style="color: var(--color-success)"
										onclick={() => openAction(item, 'approve')}
									>
										{t('moderation.approve')}
									</button>
									<button
										class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
										style="color: var(--color-warning)"
										onclick={() => openAction(item, 'reject')}
									>
										{t('moderation.reject')}
									</button>
								{/if}
								<button
									class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
									style="color: var(--color-danger)"
									onclick={() => openAction(item, 'delete')}
								>
									{t('common.delete')}
								</button>
							</div>
						</td>
					</tr>
					<!-- Expanded detail row -->
					{#if expandedId === item.id}
						<tr style="border-color: var(--color-border)">
							<td colspan="8" class="px-4 py-4" style="background-color: var(--color-surface)">
								<div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
									<div>
										<p class="text-xs font-medium" style="color: var(--color-muted)">Reporter</p>
										<p class="mt-1 text-sm">{item.reporter_name ?? item.reporter_id}</p>
									</div>
									<div>
										<p class="text-xs font-medium" style="color: var(--color-muted)">Content Type</p>
										<p class="mt-1 text-sm">{item.content_type}</p>
									</div>
									<div>
										<p class="text-xs font-medium" style="color: var(--color-muted)">Content ID</p>
										<p class="mt-1 text-sm font-mono text-xs">{item.content_id}</p>
									</div>
								</div>
								{#if item.content_preview}
									<div class="mt-3 rounded-md border p-3 text-sm" style="border-color: var(--color-border); background-color: var(--color-background)">
										<p class="text-xs font-medium mb-1" style="color: var(--color-muted)">Content Preview</p>
										{item.content_preview}
									</div>
								{/if}
								<div class="mt-3 flex gap-2">
									{#if item.status === 'pending'}
										<button
											class="rounded-md px-3 py-1.5 text-xs font-medium text-white"
											style="background-color: var(--color-success)"
											onclick={() => openAction(item, 'approve')}
										>
											{t('moderation.approve')}
										</button>
										<button
											class="rounded-md px-3 py-1.5 text-xs font-medium"
											style="background-color: var(--color-warning); color: white"
											onclick={() => openAction(item, 'reject')}
										>
											{t('moderation.reject')}
										</button>
									{/if}
									<button
										class="rounded-md px-3 py-1.5 text-xs font-medium"
										style="background-color: var(--color-danger); color: white"
										onclick={() => openAction(item, 'delete')}
									>
										{t('common.delete')}
									</button>
								</div>
							</td>
						</tr>
					{/if}
				{:else}
					<tr>
						<td colspan="8" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							{t('moderation.empty')}
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
	title={actionType === 'approve' ? t('moderation.approve_title') : actionType === 'reject' ? t('moderation.reject_title') : t('moderation.delete_title')}
	onConfirm={executeAction}
	confirmLabel={acting ? t('common.saving') : actionType === 'delete' ? t('common.delete') : t('common.confirm')}
>
	<p class="text-sm mb-3" style="color: var(--color-muted)">
		{actionType === 'approve'
			? 'Approve this content and mark the report as resolved.'
			: actionType === 'reject'
				? 'Reject this content and dismiss the report.'
				: 'Permanently delete this content. This cannot be undone.'}
	</p>
	{#if actionItem}
		<div class="mb-3 rounded-md border p-3 text-xs" style="border-color: var(--color-border); background-color: var(--color-surface)">
			<span class="font-medium">{actionItem.content_type}</span> &middot; {actionItem.reason}
		</div>
	{/if}
	{#if actionType !== 'approve'}
		<textarea
			class="w-full rounded-md border px-3 py-2 text-sm"
			style="border-color: var(--color-border); background-color: var(--color-surface)"
			rows="3"
			placeholder={actionType === 'reject' ? 'Rejection reason (optional)' : 'Deletion reason (optional)'}
			bind:value={actionReason}
		></textarea>
	{/if}
</Modal>
