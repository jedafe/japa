<script lang="ts">
	import { page } from '$app/state';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { DataTable } from '$lib/components/ui/data-table';
	import type { ActionItem } from '$lib/components/ui/data-table';
	import { goto } from '$app/navigation';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Plus } from 'lucide-svelte';

	// Ref: jc-admin-develop — RBAC gate for form management
	const canManage = $derived(hasPermission('space.forms.manage'));

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
		{ key: 'created_at', label: t('space_forms.created'), sortable: true, class: 'text-xs', render: (v: string) => `<span style="color: var(--color-muted)">${v}</span>` }
	];

	const rowActions: ActionItem[] = [
		{ label: t('common.view', 'View'), onclick: (row: FormDef) => goto(`/spaces/${spaceId}/forms/${row.id}`) }
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

<svelte:head>
	<title>Space Forms — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t('space_forms.title')}
	description={t('space_forms.description')}
	actions={canManage ? [{ label: t('space_forms.create'), icon: Plus, onclick: () => goto(`/spaces/${spaceId}/forms/new`) }] : []}
/>

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
	actions={rowActions}
/>
