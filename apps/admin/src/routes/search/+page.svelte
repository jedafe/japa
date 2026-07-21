<script lang="ts">
	import { api, searchAdminApiExtended } from '$lib/api';
	import { DataTable } from '$lib/components/ui/data-table';
	import { t } from '$lib/stores/i18n.svelte';
	import { RefreshCw } from 'lucide-svelte';

	type SearchAnalytics = {
		query: string;
		search_count: number;
		click_count: number;
		avg_results: number;
		created_at: string;
	};

	let analytics = $state<SearchAnalytics[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');

	const columns = [
		{
			key: 'query',
			label: t('search.query'),
			sortable: true,
			render: (v: string) => `<span class="font-medium">${v}</span>`
		},
		{
			key: 'search_count',
			label: t('search.searches'),
			sortable: true
		},
		{
			key: 'click_count',
			label: t('search.clicks'),
			sortable: true
		},
		{
			key: 'avg_results',
			label: t('search.avg_results'),
			sortable: true,
			render: (v: number) => `<span style="color: var(--color-muted)">${v}</span>`
		}
	];

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const result = await api<SearchAnalytics[]>('/admin/search/analytics');
			if (result.ok) {
				analytics = result.data ?? [];
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
	<title>{t('search.title')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('search.title')}</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('search.description')}</p>

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
	<DataTable {columns} data={analytics} pageSize={25} searchable emptyMessage={t('search.empty')} />
{/if}
