<script lang="ts">
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { RefreshCw } from 'lucide-svelte';
	import { DataTable } from '$lib/components/ui/data-table';

	type Deletion = {
		id: string;
		user_id: string;
		content_type: string;
		content_id: string;
		reason: string;
		deleted_at: string;
		hard_deleted: boolean;
	};

	let deletions = $state<Deletion[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');
	let searchQuery = $state('');

	const columns = [
		{ key: 'user_id', label: t('deletions.user'), render: (v: string) => `<span class="font-mono text-xs">${v}</span>` },
		{ key: 'content_type', label: t('deletions.type'), render: (v: string) => `<span class="text-xs">${v}</span>` },
		{ key: 'reason', label: t('deletions.reason'), render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${v || '—'}</span>` },
		{
			key: 'hard_deleted',
			label: t('deletions.hard'),
			render: (v: boolean) => `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${v ? 'bg-danger/15 text-danger dark:text-danger' : 'bg-warning/15 text-warning dark:text-warning'}">${v ? t('deletions.yes') : t('deletions.no')}</span>`
		},
		{
			key: 'deleted_at',
			label: t('deletions.date'),
			sortable: true,
			render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${new Date(v).toLocaleString()}</span>`
		}
	];

	const filteredDeletions = $derived.by(() => {
		let items = deletions;
		if (searchQuery) {
			const q = searchQuery.toLowerCase();
			items = items.filter((d) => d.user_id.toLowerCase().includes(q) || d.content_type.toLowerCase().includes(q) || d.reason?.toLowerCase().includes(q));
		}
		return items;
	});

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const result = await api<Deletion[]>('/admin/deletions');
			if (result.ok) {
				deletions = result.data ?? [];
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		load();
	});
</script>

<svelte:head>
	<title>{t('deletions.title')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('deletions.title')}</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('deletions.description')}</p>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<div class="mb-4 flex justify-end">
	<button class="btn btn-secondary flex items-center gap-1.5 text-xs" onclick={load}>
		<RefreshCw class="h-3 w-3" />
		{t('common.refresh')}
	</button>
</div>

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<DataTable
		columns={columns}
		data={filteredDeletions}
		pageSize={25}
		emptyMessage={t('deletions.empty')}
		searchable
		onsearch={(q) => { searchQuery = q; }}
	/>
{/if}
