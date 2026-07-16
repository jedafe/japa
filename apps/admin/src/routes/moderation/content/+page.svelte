<script lang="ts">
	import FilterBar from '$lib/components/FilterBar.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import { contentAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';

	type ContentItem = {
		id: string;
		title: string;
		author: string;
		author_id: string;
		type: 'post' | 'comment' | 'blog' | 'forum';
		status: string;
		reports_count: number;
		is_featured: boolean;
		created_at: string;
		body_preview?: string;
	};

	let items = $state<ContentItem[]>([]);
	let page = $state(0);
	let totalPages = $state(1);
	let loading = $state(true);
	let filterType = $state('');
	let filterStatus = $state('');
	let errorMessage = $state('');

	// Expandable detail
	let expandedId = $state<string | null>(null);

	// Delete modal
	let showDeleteModal = $state(false);
	let deleteTarget = $state<ContentItem | null>(null);

	const canModerateContent = $derived(hasPermission('content.moderate'));

	const filterDefs = [
		{ key: 'type', label: t('moderation.content_type'), type: 'select' as const, options: ['post', 'comment', 'blog', 'forum'] },
		{ key: 'status', label: t('moderation.status'), type: 'select' as const, options: ['published', 'draft', 'archived', 'flagged'] }
	];

	async function load() {
		loading = true;
		errorMessage = '';
		const params = new URLSearchParams({ page: String(page) });
		if (filterType) params.set('type', filterType);
		if (filterStatus) params.set('status', filterStatus);
		try {
			const result = await contentAdminApi.list(params.toString());
			if (result.ok) {
				const d = result.data as { items: ContentItem[]; total_pages: number };
				items = d.items;
				totalPages = d.total_pages;
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
		filterType = values.type ?? '';
		filterStatus = values.status ?? '';
		page = 0;
		load();
	}

	function toggleExpand(id: string) {
		expandedId = expandedId === id ? null : id;
	}

	function confirmDelete(item: ContentItem) {
		deleteTarget = item;
		showDeleteModal = true;
	}

	async function executeDelete() {
		if (!deleteTarget) return;
		errorMessage = '';
		try {
			const result = await contentAdminApi.remove(deleteTarget.id);
			if (result.ok) {
				showDeleteModal = false;
				load();
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		}
	}

	function handlePageChange(p: number) {
		page = p;
		load();
	}

	function typeBadge(type: string) {
		switch (type) {
			case 'post': return 'bg-indigo-500/15 text-indigo-700 dark:text-indigo-400';
			case 'comment': return 'bg-cyan-500/15 text-cyan-700 dark:text-cyan-400';
			case 'blog': return 'bg-rose-500/15 text-rose-700 dark:text-rose-400';
			case 'forum': return 'bg-orange-500/15 text-orange-700 dark:text-orange-400';
			default: return 'bg-slate-500/15 text-slate-700 dark:text-slate-400';
		}
	}

	function statusBadge(status: string) {
		switch (status) {
			case 'published': return 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400';
			case 'draft': return 'bg-slate-500/15 text-slate-700 dark:text-slate-400';
			case 'flagged': return 'bg-red-500/15 text-red-700 dark:text-red-400';
			default: return 'bg-amber-500/15 text-amber-700 dark:text-amber-400';
		}
	}

	$effect(() => {
		load();
	});
</script>

<svelte:head>
	<title>{t('moderation.content_admin')} — JapaConnect Admin</title>
</svelte:head>

<div class="mb-4">
	<a href="/moderation" class="text-xs hover:underline" style="color: var(--color-muted)">&larr; {t('moderation.back')}</a>
</div>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('moderation.content_admin')}</h1>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}
<p class="mb-4 text-sm" style="color: var(--color-muted)">Browse and manage platform content across all spaces</p>

<FilterBar filters={filterDefs} onApply={handleFilter} />

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-color: var(--color-border)">
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('moderation.title')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('moderation.author')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('moderation.content_type')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('moderation.status')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('moderation.reports')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('moderation.date')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)"></th>
				</tr>
			</thead>
			<tbody>
				{#each items as item}
					<tr
						class="border-t cursor-pointer hover:bg-black/[0.02] dark:hover:bg-white/[0.02]"
						style="border-color: var(--color-border)"
						onclick={() => toggleExpand(item.id)}
					>
						<td class="px-4 py-3">
							<p class="font-medium">{item.title}</p>
							{#if item.is_featured}
								<span class="inline-block rounded-full bg-yellow-500/15 px-2 py-0.5 text-xs text-yellow-700 dark:text-yellow-400">Featured</span>
							{/if}
						</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{item.author}</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs {typeBadge(item.type)}">
								{item.type}
							</span>
						</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs {statusBadge(item.status)}">
								{item.status}
							</span>
						</td>
						<td class="px-4 py-3">
							{#if item.reports_count > 0}
								<span class="inline-block rounded-full bg-red-500/15 px-2 py-0.5 text-xs text-red-700 dark:text-red-400">
									{item.reports_count}
								</span>
							{:else}
								<span class="text-xs" style="color: var(--color-muted)">0</span>
							{/if}
						</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{item.created_at}</td>
						<td class="px-4 py-3" onclick={(e) => e.stopPropagation()}>
							<div class="flex items-center gap-1">
								{#if canModerateContent}
									<button
										class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
										style="color: var(--color-danger)"
										onclick={() => confirmDelete(item)}
									>
										{t('common.delete')}
									</button>
								{/if}
							</div>
						</td>
					</tr>
					{#if expandedId === item.id}
						<tr style="border-color: var(--color-border)">
							<td colspan="7" class="px-4 py-4" style="background-color: var(--color-surface)">
								<div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
									<div>
										<p class="text-xs font-medium" style="color: var(--color-muted)">Title</p>
										<p class="mt-1 text-sm font-medium">{item.title}</p>
									</div>
									<div>
										<p class="text-xs font-medium" style="color: var(--color-muted)">Author</p>
										<p class="mt-1 text-sm">{item.author}</p>
										<p class="text-xs font-mono" style="color: var(--color-muted)">{item.author_id.slice(0, 12)}</p>
									</div>
									<div>
										<p class="text-xs font-medium" style="color: var(--color-muted)">Created</p>
										<p class="mt-1 text-sm">{item.created_at}</p>
									</div>
								</div>
								{#if item.body_preview}
									<div class="mt-3 rounded-md border p-3 text-sm" style="border-color: var(--color-border); background-color: var(--color-background)">
										<p class="text-xs font-medium mb-1" style="color: var(--color-muted)">Content Preview</p>
										{item.body_preview}
									</div>
								{/if}
								<div class="mt-3 flex gap-2">
									{#if canModerateContent}
										<button
											class="rounded-md px-3 py-1.5 text-xs font-medium text-white"
											style="background-color: var(--color-danger)"
											onclick={() => confirmDelete(item)}
										>
											{t('common.delete')}
										</button>
									{/if}
								</div>
							</td>
						</tr>
					{/if}
				{:else}
					<tr>
						<td colspan="7" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							No content found
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

<!-- Delete Confirmation -->
<Modal bind:open={showDeleteModal} title="Confirm Delete" onConfirm={executeDelete} confirmLabel={t('common.delete')}>
	<p class="text-sm" style="color: var(--color-muted)">Are you sure you want to delete this content? This action cannot be undone.</p>
</Modal>
