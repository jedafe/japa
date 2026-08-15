<script lang="ts">
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent } from '$lib/components/ui/card';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Button } from '$lib/components/ui/button';
	import { TableSkeleton } from '$lib/components/ui/skeleton';
	import { Modal } from '$lib/components/ui/modal';
	import { SlideOver } from '$lib/components/ui/slide-over';
	import type { ActionItem, BulkAction } from '$lib/components/ui/data-table';
	import { Plus, RefreshCw, Trash2, Pencil } from 'lucide-svelte';
	import { userManagementApi, permissionsApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { pageFromUrl, pageSizeFromUrl, searchFromUrl, syncListStateToUrl } from '$lib/utils/url-sync.svelte';
	import type { UserGroup } from '$lib/api';

	let groups = $state<UserGroup[]>([]);
	let loading = $state(true);
	let currentPage = $state(pageFromUrl());
	let totalItems = $state(0);
	let pageSize = $state(pageSizeFromUrl(25));
	let totalPages = $state(1);

	let showSlideOver = $state(false);
	let editingGroup = $state<UserGroup | null>(null);
	let formName = $state('');
	let formDescription = $state('');
	let formPermissions = $state<string[]>([]);
	let saveError = $state('');
	let saving = $state(false);

	let showDeleteConfirm = $state(false);
	let deleteTarget = $state<UserGroup | null>(null);
	let bulkDeleting = $state(false);

	let selectedIds = $state<Set<string>>(new Set());

	let allPermissions = $state<string[]>([]);
	let permSearch = $state('');
	let permCategory = $state('');
	let searchQuery = $state(searchFromUrl());

	const canManageGroups = $derived(hasPermission('admin.rbac.manage'));

	const permissionCategories = $derived(() => {
		const cats = new Set<string>();
		for (const p of allPermissions) {
			const dot = p.indexOf('.');
			if (dot > 0) cats.add(p.slice(0, dot));
		}
		return [...cats].sort();
	});

	const filteredPermissions = $derived(
		allPermissions.filter((p) => {
			if (permCategory && !p.startsWith(permCategory + '.') && !(permCategory === 'other' && !p.includes('.'))) return false;
			if (permSearch && !p.toLowerCase().includes(permSearch.toLowerCase())) return false;
			return true;
		})
	);

	async function load() {
		loading = true;
		try {
			const params = new URLSearchParams({
				page: String(currentPage + 1),
				limit: String(pageSize)
			});
			if (searchQuery) params.set('search', searchQuery);
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
			if (permsResult.ok) allPermissions = permsResult.data;
		} catch (e) {
			console.error('Failed to load groups:', e instanceof Error ? e.message : String(e));
		}
		finally { loading = false; }
	}

	function openCreate() {
		editingGroup = null;
		formName = '';
		formDescription = '';
		formPermissions = [];
		saveError = '';
		permSearch = '';
		permCategory = '';
		showSlideOver = true;
	}

	function openEdit(group: UserGroup) {
		editingGroup = group;
		formName = group.name;
		formDescription = group.description;
		formPermissions = [...(group.permissions ?? [])];
		saveError = '';
		permSearch = '';
		permCategory = '';
		showSlideOver = true;
	}

	async function handleSave() {
		if (!formName.trim()) { saveError = t('users.groups.name_required', 'Name is required'); return; }
		saving = true;
		saveError = '';
		const body = { name: formName, description: formDescription, permissions: formPermissions };
		const result = editingGroup
			? await userManagementApi.updateGroup(editingGroup.id, body)
			: await userManagementApi.createGroup(body);
		saving = false;
		if (result.ok) {
			showSlideOver = false;
			load();
		} else {
			saveError = result.error ?? t('users.groups.save_error', 'Failed to save group');
		}
	}

	function confirmDelete(group: UserGroup) {
		deleteTarget = group;
		showDeleteConfirm = true;
	}

	async function handleDelete() {
		if (!deleteTarget) return;
		const result = await userManagementApi.deleteGroup(deleteTarget.id);
		if (result.ok) {
			showDeleteConfirm = false;
			deleteTarget = null;
			load();
		}
	}

	async function bulkDelete(ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0 || bulkDeleting) return;
		bulkDeleting = true;
		try {
			await Promise.all(idArr.map((id) => userManagementApi.deleteGroup(id)));
			load();
		} catch (e) {
			console.error('Bulk delete failed:', e instanceof Error ? e.message : String(e));
		}
		bulkDeleting = false;
	}

	const columns = [
		{ key: 'name', label: t('users.groups.name', 'Name') },
		{ key: 'description', label: t('users.groups.description_label', 'Description') },
		{
			key: 'member_count',
			label: t('users.groups.members', 'Members'),
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
		}
	];

	const rowActions: ActionItem[] = [
		{
			label: t('common.edit', 'Edit'),
			icon: Pencil,
			onclick: (row: UserGroup) => openEdit(row),
			show: () => canManageGroups
		},
		{
			label: t('common.delete', 'Delete'),
			icon: Trash2,
			variant: 'destructive',
			onclick: (row: UserGroup) => confirmDelete(row),
			show: () => canManageGroups
		}
	];

	const bulkActions: BulkAction[] = $derived(canManageGroups
		? [{ label: t('users.groups.delete_selected', 'Delete Selected'), icon: Trash2, variant: 'destructive', onclick: bulkDelete }]
		: []);

	// Sync list state (page, search) to URL for back/forward survival.
	$effect(() => {
		void currentPage;
		void pageSize;
		void searchQuery;
		syncListStateToUrl({
			page: currentPage,
			pageSize,
			defaultPageSize: 25,
			search: searchQuery
		});
	});

	$effect(() => {
		void currentPage;
		void pageSize;
		void searchQuery;
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
				page={currentPage}
				{pageSize}
				totalRows={totalItems}
				{totalPages}
				onpagechange={(p) => { currentPage = p; }}
				onpagesizechange={(s) => { pageSize = s; currentPage = 0; }}
				emptyMessage={t('users.groups.empty', 'No groups found')}
				searchable
				onsearch={(q) => { searchQuery = q; currentPage = 0; }}
				selectable={canManageGroups}
				bind:selectedIds
				actions={rowActions}
				bulkActionsList={bulkActions}
			/>
		{/if}
	</CardContent>
</Card>

<!-- Create / Edit SlideOver -->
<SlideOver bind:open={showSlideOver} title={editingGroup ? t('users.groups.edit_title', 'Edit Group') : t('users.groups.create_title', 'Create Group')}>
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
			<div class="mb-2 flex gap-2">
				<select
					class="flex-1 rounded-md border px-2 py-1.5 text-xs"
					style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
					bind:value={permCategory}
				>
					<option value="">All categories</option>
					{#each permissionCategories() as cat (cat)}
						<option value={cat}>{cat}</option>
					{/each}
					<option value="other">Other</option>
				</select>
				<input
					type="text"
					class="flex-1 rounded-md border px-2 py-1.5 text-xs"
					style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
					placeholder={t('users.groups.search_permissions', 'Search permissions...')}
					bind:value={permSearch}
				/>
			</div>
			<div class="grid max-h-60 grid-cols-2 gap-2 overflow-y-auto">
				{#each filteredPermissions as perm (perm)}
					<label class="flex items-center gap-2 text-xs">
						<input type="checkbox" value={perm} bind:group={formPermissions} class="rounded" />
						{perm}
					</label>
				{/each}
			</div>
		</div>
	</div>
	{#snippet footer()}
		<div class="flex justify-end gap-2">
			<Button variant="outline" onclick={() => { showSlideOver = false; }}>{t('common.cancel', 'Cancel')}</Button>
			<Button onclick={handleSave} disabled={saving}>{saving ? t('common.saving', 'Saving...') : t('common.save', 'Save')}</Button>
		</div>
	{/snippet}
</SlideOver>

<!-- Delete Confirmation -->
<Modal bind:open={showDeleteConfirm} title={t('common.delete', 'Delete')} size="sm">
	<p class="text-sm" style="color: var(--color-foreground)">
		{t('users.groups.delete_confirm', 'Are you sure you want to delete this group?')}
	</p>
	<p class="mt-2 text-sm font-medium" style="color: var(--color-foreground)">{deleteTarget?.name}</p>
	<p class="mt-1 text-sm" style="color: var(--color-danger)">
		{t('common.cannotUndo', 'This action cannot be undone.')}
	</p>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="outline" onclick={() => { showDeleteConfirm = false; }}>{t('common.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={handleDelete}>{t('common.delete', 'Delete')}</Button>
	</div>
</Modal>
