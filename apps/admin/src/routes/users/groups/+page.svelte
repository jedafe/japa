<script lang="ts">
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent } from '$lib/components/ui/card';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Button } from '$lib/components/ui/button';
	import Pagination from '$lib/components/ui/Pagination.svelte';
	import { TableSkeleton } from '$lib/components/ui/skeleton';
	import Modal from '$lib/components/Modal.svelte';
	import { Plus, RefreshCw, Trash2 } from 'lucide-svelte';
	import { userManagementApi, permissionsApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import type { UserGroup } from '$lib/api';
	import GroupActionsCell from './GroupActionsCell.svelte';

	let groups = $state<UserGroup[]>([]);
	let loading = $state(true);
	let currentPage = $state(0);
	let totalItems = $state(0);
	let pageSize = $state(25);
	let totalPages = $state(1);

	let showModal = $state(false);
	let formName = $state('');
	let formDescription = $state('');
	let formPermissions = $state<string[]>([]);
	let saveError = $state('');
	let saving = $state(false);

	let selectedIds = $state<Set<string>>(new Set());
	let bulkDeleting = $state(false);

	let allPermissions = $state<string[]>([]);
	let permSearch = $state('');
	let searchQuery = $state("");

	const canManageGroups = $derived(hasPermission('admin.rbac.manage'));

	const filteredPermissions = $derived(
		permSearch
			? allPermissions.filter((p) => p.toLowerCase().includes(permSearch.toLowerCase()))
			: allPermissions
	);

	async function load() {
		loading = true;
		try {
			const params = new URLSearchParams({
				page: String(currentPage + 1),
				limit: String(pageSize)
			});
			const [groupsResult, permsResult] = await Promise.all([
				userManagementApi.listGroups(params.toString()),
				permissionsApi.list()
			]);
			if (groupsResult.ok) {
				const d = groupsResult.data;
				groups = d.groups ?? [];
				totalItems = d.total ?? 0;
				totalPages = Math.ceil(totalItems / pageSize) || 1;
			}
			if (searchQuery) {
				const q = searchQuery.toLowerCase();
				groups = groups.filter((g: any) => g.name?.toLowerCase().includes(q) || g.description?.toLowerCase().includes(q));
			}
			if (permsResult.ok) allPermissions = permsResult.data;
		} catch { /* ignore */ }
		finally { loading = false; }
	}

	function openCreate() {
		formName = '';
		formDescription = '';
		formPermissions = [];
		saveError = '';
		showModal = true;
	}

	async function handleSave() {
		if (!formName.trim()) { saveError = t('users.groups.name_required', 'Name is required'); return; }
		saving = true;
		saveError = '';
		const body = { name: formName, description: formDescription, permissions: formPermissions };
		const result = await userManagementApi.createGroup(body);
		saving = false;
		if (result.ok) {
			showModal = false;
			load();
		} else {
			saveError = result.error ?? t('users.groups.save_error', 'Failed to save group');
		}
	}

	async function bulkDelete() {
		if (selectedIds.size === 0) return;
		bulkDeleting = true;
		try {
			for (const id of selectedIds) {
				await userManagementApi.deleteGroup(id);
			}
			selectedIds = new Set();
			load();
		} catch { /* ignore */ }
		bulkDeleting = false;
	}

	const columns = [
		{ key: 'name', label: t('users.groups.name', 'Name'), sortable: true },
		{ key: 'description', label: t('users.groups.description_label', 'Description') },
		{
			key: 'member_count',
			label: t('users.groups.members', 'Members'),
			sortable: true,
			render: (v: number) => `<span class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-semibold" style="background-color: color-mix(in srgb, var(--color-primary) 10%, transparent); color: var(--color-primary)">${v}</span>`
		},
		{
			key: 'permissions',
			label: t('users.groups.permissions', 'Permissions'),
			render: (v: string[]) => {
				if (!v || !Array.isArray(v)) return '';
				const shown = v.slice(0, 3);
				const rest = v.length > 3 ? `<span class="inline-flex items-center rounded-full px-2 py-0.5 text-xs" style="background-color: var(--color-surface); color: var(--color-muted)">+${v.length - 3}</span>` : '';
				return `<div class="flex flex-wrap gap-1">${shown.map((p) => `<span class="inline-flex items-center rounded-full px-2 py-0.5 text-xs" style="background-color: var(--color-surface); color: var(--color-muted)">${p}</span>`).join('')}${rest}</div>`;
			}
		},
		{
			key: 'actions',
			label: t('common.actions', 'Actions'),
			class: 'w-24',
			component: GroupActionsCell,
			componentProps: { onchange: load }
		}
	];

	$effect(() => {
		void currentPage;
		void pageSize;
		load();
	});
</script>

<svelte:head>
	<title>{t('users.groups.title', 'User Groups')} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t('users.groups.title', 'User Groups')}
	description={t('users.groups.description', 'Manage user groups and their permissions')}
	actions={[
		...(canManageGroups ? [{ label: t('users.groups.create', 'Create Group'), icon: Plus, onclick: openCreate }] : []),
		{ label: t('common.refresh', 'Refresh'), icon: RefreshCw, variant: 'ghost', onclick: load }
	]}
/>

<Card>
	<CardContent>
		{#if loading}
			<TableSkeleton rows={5} columns={4} />
		{:else}
			<DataTable
				{columns}
				data={groups}
				emptyMessage={t('users.groups.empty', 'No groups found')}
				searchable
				onsearch={(q) => { searchQuery = q; load(); }}
				selectable
				bind:selectedIds
			>
				{#snippet bulkActions()}
					<Button variant="destructive" size="sm" onclick={bulkDelete} disabled={bulkDeleting || selectedIds.size === 0}>
						<Trash2 class="h-3.5 w-3.5 mr-1" />
						{bulkDeleting ? 'Deleting...' : 'Delete Selected'} ({selectedIds.size})
					</Button>
				{/snippet}
			</DataTable>
			<Pagination
				bind:page={currentPage}
				{totalPages}
				{totalItems}
				bind:pageSize
			/>
		{/if}
	</CardContent>
</Card>

<!-- Create Modal -->
<Modal bind:open={showModal} title={t('users.groups.create_title', 'Create Group')} onConfirm={handleSave} confirmLabel={saving ? t('common.saving', 'Saving...') : t('common.save', 'Save')}>
	{#if saveError}
		<div class="mb-3 rounded-md border px-3 py-2 text-xs" style="border-color: var(--color-danger); color: var(--color-danger)">{saveError}</div>
	{/if}
	<div class="space-y-4">
		<div>
			<label for="group-name" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('users.groups.name', 'Name')}</label>
			<input
				id="group-name"
				type="text"
				class="w-full rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
				placeholder={t('users.groups.name_placeholder', 'Enter group name')}
				bind:value={formName}
			/>
		</div>
		<div>
			<label for="group-desc" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('users.groups.description_label', 'Description')}</label>
			<textarea
				id="group-desc"
				rows="2"
				class="w-full rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
				placeholder={t('users.groups.desc_placeholder', 'Enter group description')}
				bind:value={formDescription}
			></textarea>
		</div>
		<div>
			<p class="mb-2 text-xs font-medium" style="color: var(--color-muted)">{t('users.groups.permissions', 'Permissions')}</p>
			<input
				type="text"
				class="mb-2 w-full rounded-md border px-3 py-1.5 text-xs"
				style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
				placeholder={t('users.groups.search_permissions', 'Search permissions...')}
				bind:value={permSearch}
			/>
			<div class="grid max-h-60 grid-cols-2 gap-2 overflow-y-auto">
				{#each filteredPermissions as perm}
					<label class="flex items-center gap-2 text-xs">
						<input type="checkbox" value={perm} bind:group={formPermissions} class="rounded" />
						{perm}
					</label>
				{/each}
			</div>
		</div>
	</div>
</Modal>
