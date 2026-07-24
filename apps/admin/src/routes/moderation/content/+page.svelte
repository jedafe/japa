<script lang="ts">
	import FilterBar from '$lib/components/FilterBar.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import { DataTable } from '$lib/components/ui/data-table';
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
	let loading = $state(true);
	let filterType = $state('');
	let filterStatus = $state('');
	let searchQuery = $state('');
	let errorMessage = $state('');

	let expandedId = $state<string | null>(null);

	let showDeleteModal = $state(false);
	let deleteTarget = $state<ContentItem | null>(null);

	const canModerateContent = $derived(hasPermission('content.moderate'));

	const filterDefs = [
		{ key: 'type', label: t('moderation.content_type'), type: 'select' as const, options: ['post', 'comment', 'blog', 'forum'] },
		{ key: 'status', label: t('moderation.status'), type: 'select' as const, options: ['published', 'draft', 'archived', 'flagged'] }
	];

	function typeBadge(type: string) {
		switch (type) {
			case 'post': return 'bg-info/15 text-info dark:text-info';
			case 'comment': return 'bg-info/15 text-info dark:text-info';
			case 'blog': return 'bg-danger/15 text-danger dark:text-danger';
			case 'forum': return 'bg-warning/15 text-warning dark:text-warning';
			default: return 'bg-muted/15 text-muted dark:text-muted';
		}
	}

	function statusBadge(status: string) {
		switch (status) {
			case 'published': return 'bg-success/15 text-success dark:text-success';
			case 'draft': return 'bg-muted/15 text-muted dark:text-muted';
			case 'flagged': return 'bg-danger/15 text-danger dark:text-danger';
			default: return 'bg-warning/15 text-warning dark:text-warning';
		}
	}

	const columns = [
		{
			key: 'title',
			label: t('moderation.title'),
			sortable: true,
			render: (_: any, row: ContentItem) => `<div><p class="font-medium">${row.title}</p>${row.is_featured ? '<span class="inline-block rounded-full bg-warning/15 px-2 py-0.5 text-xs text-warning dark:text-warning">' + t('moderation.content.featured') + '</span>' : ''}</div>`
		},
		{
			key: 'author',
			label: t('moderation.author'),
			sortable: true,
			render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${v}</span>`
		},
		{
			key: 'type',
			label: t('moderation.content_type'),
			sortable: true,
			render: (v: string) => `<span class="inline-block rounded-full px-2 py-0.5 text-xs ${typeBadge(v)}">${v}</span>`
		},
		{
			key: 'status',
			label: t('moderation.status'),
			sortable: true,
			render: (v: string) => `<span class="inline-block rounded-full px-2 py-0.5 text-xs ${statusBadge(v)}">${v}</span>`
		},
		{
			key: 'reports_count',
			label: t('moderation.reports'),
			sortable: true,
			render: (v: number) => v > 0 ? `<span class="inline-block rounded-full bg-danger/15 px-2 py-0.5 text-xs text-danger dark:text-danger">${v}</span>` : `<span class="text-xs" style="color: var(--color-muted)">0</span>`
		},
		{
			key: 'created_at',
			label: t('moderation.date'),
			sortable: true,
			render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${v}</span>`
		},
		{
			key: 'actions',
			label: '',
			class: 'w-24',
			component: ContentActionsCell
		}
	];

	async function load() {
		loading = true;
		errorMessage = '';
		const params = new URLSearchParams();
		if (filterType) params.set('type', filterType);
		if (filterStatus) params.set('status', filterStatus);
		if (searchQuery) params.set('q', searchQuery);
		try {
			const result = await contentAdminApi.list(params.toString());
			if (result.ok) {
				const d = result.data as { items: ContentItem[] };
				items = d?.items ?? [];
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
		searchQuery = values.search ?? '';
		filterType = values.type ?? '';
		filterStatus = values.status ?? '';
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

	$effect(() => {
		load();
	});
</script>

{#snippet ContentActionsCell({ value, row }: { value: any; row: ContentItem })}
	{#if canModerateContent}
		<button
			class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-surface/10"
			style="color: var(--color-danger)"
			onclick={(e: MouseEvent) => { e.stopPropagation(); confirmDelete(row); }}
		>
			{t('common.delete')}
		</button>
	{/if}
{/snippet}

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
<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('moderation.content.browse_and_manage_platform_content_acros')}</p>

<FilterBar filters={filterDefs} onApply={handleFilter} />

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<DataTable {columns} data={items} pageSize={25} searchable emptyMessage={t('app.moderation.content.empty_message', 'No content found')} onrowclick={(row) => toggleExpand(row.id)} />

	{#if expandedId}
		{@const expanded = items.find((i) => i.id === expandedId)}
		{#if expanded}
			<div class="mt-3 rounded-lg border px-4 py-4" style="border-color: var(--color-border); background-color: var(--color-surface)">
				<div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
					<div>
						<p class="text-xs font-medium" style="color: var(--color-muted)">{t('moderation.content.title')}</p>
						<p class="mt-1 text-sm font-medium">{expanded.title}</p>
					</div>
					<div>
						<p class="text-xs font-medium" style="color: var(--color-muted)">{t('moderation.content.author')}</p>
						<p class="mt-1 text-sm">{expanded.author}</p>
						<p class="text-xs font-mono" style="color: var(--color-muted)">{expanded.author_id.slice(0, 12)}</p>
					</div>
					<div>
						<p class="text-xs font-medium" style="color: var(--color-muted)">{t('moderation.content.created')}</p>
						<p class="mt-1 text-sm">{expanded.created_at}</p>
					</div>
				</div>
				{#if expanded.body_preview}
					<div class="mt-3 rounded-md border p-3 text-sm" style="border-color: var(--color-border); background-color: var(--color-background)">
						<p class="text-xs font-medium mb-1" style="color: var(--color-muted)">{t('moderation.content.content_preview')}</p>
						{expanded.body_preview}
					</div>
				{/if}
				<div class="mt-3 flex gap-2">
					{#if canModerateContent}
						<button
							class="rounded-md px-3 py-1.5 text-xs font-medium text-white"
							style="background-color: var(--color-danger)"
							onclick={() => confirmDelete(expanded)}
						>
							{t('common.delete')}
						</button>
					{/if}
				</div>
			</div>
		{/if}
	{/if}
{/if}

<!-- Delete Confirmation -->
<Modal bind:open={showDeleteModal} title={t('app.moderation.content.confirm_delete', 'Confirm Delete')} onConfirm={executeDelete} confirmLabel={t('common.delete')}>
	<p class="text-sm" style="color: var(--color-muted)">{t('moderation.content.are_you_sure_you_want_to_delete_this_con')}</p>
</Modal>
