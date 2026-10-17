<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	import { page } from '$app/state';
	import { api, formsAdminApi } from '$lib/api';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { DataTable } from '$lib/components/ui/data-table';
	import type { ActionItem } from '$lib/components/ui/data-table';
	import { goto } from '$app/navigation';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Plus, Trash2 } from 'lucide-svelte';

	// Seeded form nodes are forms.create/forms.edit/forms.delete —
	// "space.forms.manage" does not exist.
	const canManage = $derived(hasPermission('forms.create'));
	const canDelete = $derived(hasPermission('forms.delete'));

	// GET /forms?space_id returns ListFormsBySpaceRow — full forms row plus
	// submission_count.
	type FormDef = {
		id: string;
		name: string;
		slug?: string;
		description?: string;
		status?: string;
		submission_count?: number;
		created_at: string;
	};

	const spaceId = $derived(page.params.spaceId ?? '');

	let forms = $state<FormDef[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');

	let searchQuery = $state('');

	let showDelete = $state(false);
	let deletingForm = $state<FormDef | null>(null);
	let deleting = $state(false);

	const columns = [
		{ key: 'name', label: t('space_forms.name'), sortable: true },
		{ key: 'description', label: t('space_forms.description_label'), sortable: true, class: 'text-xs max-w-xs truncate', render: (v: string) => `<span style="color: var(--color-muted)">${v || '—'}</span>` },
		{ key: 'submission_count', label: t('space_forms.submissions'), sortable: true, render: (v: number) => `<span class="inline-block rounded-full px-2 py-0.5 text-xs bg-black/5 dark:bg-surface/10">${v ?? 0}</span>` },
		{ key: 'created_at', label: t('space_forms.created'), sortable: true, class: 'text-xs', render: (v: string) => `<span style="color: var(--color-muted)">${v ? new Date(v).toLocaleDateString() : '—'}</span>` }
	];

	const rowActions: ActionItem[] = [
		{ label: t('common.view', 'View'), onclick: (row: FormDef) => goto(`/spaces/${spaceId}/forms/${row.id}`) },
		{ label: t('common.delete', 'Delete'), icon: Trash2, variant: 'destructive', onclick: (row: FormDef) => { deletingForm = row; showDelete = true; }, show: () => canDelete }
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
			// api() unwraps the envelope — data IS the form array.
			const result = await api<FormDef[]>(`/forms?space_id=${encodeURIComponent(spaceId)}`);
			if (result.ok) {
				forms = Array.isArray(result.data) ? result.data : [];
			} else {
				errorMessage = result.error ?? t('space_forms.load_error');
			}
		} catch {
			errorMessage = t('space_forms.load_error');
		} finally {
			loading = false;
		}
	}

	async function deleteForm() {
		if (!deletingForm || deleting) return;
		deleting = true;
		try {
			const result = await formsAdminApi.delete(deletingForm.id);
			if (!result.ok) {
				errorMessage = result.error ?? t('space_forms.delete_error', 'Failed to delete form');
				return;
			}
			showDelete = false;
			deletingForm = null;
			load();
		} catch {
			errorMessage = t('space_forms.delete_error', 'Failed to delete form');
		} finally {
			deleting = false;
		}
	}

	$effect(() => {
		load();
	});
</script>

<svelte:head>
	<title>Space Forms — {siteName()} Admin</title>
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

<!-- Delete Confirmation -->
<Modal bind:open={showDelete} title={t('space_forms.delete_title', 'Delete Form')} size="sm">
	<p class="text-sm">{t('space_forms.delete_text', 'Delete "{name}"? This cannot be undone.').replace('{name}', deletingForm?.name ?? '')}</p>
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => { showDelete = false; }}>{t('common.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={deleteForm} disabled={deleting}>{deleting ? t('common.deleting', 'Deleting...') : t('common.delete', 'Delete')}</Button>
	</div>
</Modal>
