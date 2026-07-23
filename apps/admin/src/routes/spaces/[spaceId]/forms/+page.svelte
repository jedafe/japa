<script lang="ts">
	import { page } from '$app/state';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Button } from '$lib/components/ui/button';
	import { goto } from '$app/navigation';

	type FormDef = {
		id: string;
		name: string;
		description?: string;
		submission_count?: number;
		created_at: string;
	};

	const spaceId = $derived(page.params.spaceId);

	let forms = $state<FormDef[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');

	let searchQuery = $state('');

	const columns = [
		{ key: 'name', label: t('space_forms.name'), sortable: true },
		{ key: 'description', label: t('space_forms.description_label'), sortable: true, class: 'text-xs max-w-xs truncate', render: (v: string) => `<span style="color: var(--color-muted)">${v || '—'}</span>` },
		{ key: 'submission_count', label: t('space_forms.submissions'), sortable: true, render: (v: number) => `<span class="inline-block rounded-full px-2 py-0.5 text-xs bg-black/5 dark:bg-surface/10">${v}</span>` },
		{ key: 'created_at', label: t('space_forms.created'), sortable: true, class: 'text-xs', render: (v: string) => `<span style="color: var(--color-muted)">${v}</span>` },
		{ key: 'actions', label: '', class: 'w-20', component: FormActionsCell }
	];

	function filteredForms(): FormDef[] {
		const q = searchQuery.toLowerCase().trim();
		if (!q) return forms;
		return forms.filter(f =>
			f.name.toLowerCase().includes(q) ||
			(f.description ?? '').toLowerCase().includes(q)
		);
	}

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const result = await api<{ data: FormDef[] }>(`/forms?space_id=${spaceId}`);
			if (result.ok) {
				forms = Array.isArray(result.data) ? result.data : (result.data.data ?? []);
			}
		} catch {
			errorMessage = t('space_forms.load_error');
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		load();
	});
</script>

{#snippet FormActionsCell({ value, row }: { value: any; row: FormDef })}
	<Button variant="ghost" size="sm" onclick={() => goto(`/spaces/${spaceId}/forms/${row.id}`)} style="color: var(--color-info)">
		View
	</Button>
{/snippet}

<svelte:head>
	<title>Space Forms — JapaConnect Admin</title>
</svelte:head>

<div class="mb-4 flex items-center justify-between">
	<div>
		<h1 class="text-2xl font-bold tracking-tight">{t('space_forms.title')}</h1>
		<p class="mt-1 text-sm" style="color: var(--color-muted)">{t('space_forms.description')}</p>
	</div>
	<a
		href="/spaces/{spaceId}/forms/new"
		class="inline-flex items-center rounded-md px-4 py-2 text-sm font-medium text-white transition-colors"
		style="background-color: var(--color-primary)"
	>
		{t('space_forms.create')}
	</a>
</div>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<DataTable
	columns={columns}
	data={filteredForms()}
	{loading}
	pageSize={25}
	emptyMessage={t('space_forms.empty')}
	searchable
	searchPlaceholder={t('common.search')}
	onsearch={(q) => { searchQuery = q; }}
/>
