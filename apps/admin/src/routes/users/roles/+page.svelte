<script lang="ts">
	import Modal from '$lib/components/Modal.svelte';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Card, CardContent } from '$lib/components/ui/card';
	import Pagination from '$lib/components/ui/Pagination.svelte';
	import { TableSkeleton } from '$lib/components/ui/skeleton';
	import { userManagementApi, permissionsApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';

	const canManage = $derived(hasPermission('admin.rbac.manage'));

	type Role = {
		id: string;
		name: string;
		permissions: string[];
		user_count: number;
	};

	let roles = $state<Role[]>([]);
	let loading = $state(true);
	let currentPage = $state(0);
	let totalItems = $state(0);
	let pageSize = $state(25);
	let totalPages = $state(1);
	let showModal = $state(false);
	let editingRole = $state<Role | null>(null);
	let formName = $state('');
	let formPermissions = $state<string[]>([]);
	let saveError = $state('');
	let saving = $state(false);
	let showDeleteConfirm = $state(false);
	let deletingRole = $state<Role | null>(null);
	let searchQuery = $state('');

	let allPermissions = $state<string[]>([]);
	let permSearch = $state('');

	const filteredPermissions = $derived(
		permSearch
			? allPermissions.filter((p) => p.toLowerCase().includes(permSearch.toLowerCase()))
			: allPermissions
	);

	const columns = [
		{ key: 'name', label: t('users.roles.name', 'Name'), sortable: true },
		{ key: 'permissions', label: t('users.roles.permissions', 'Permissions'), render: (_: any, row: Role) => {
			const chips = row.permissions.slice(0, 4).map((p: string) => `<span class="inline-block rounded-full px-2 py-0.5 text-xs" style="background-color: color-mix(in srgb, var(--color-secondary) 15%, transparent); color: var(--color-foreground)">${p}</span>`).join('');
			const extra = row.permissions.length > 4 ? `<span class="inline-block rounded-full px-2 py-0.5 text-xs" style="background-color: color-mix(in srgb, var(--color-secondary) 15%, transparent); color: var(--color-muted)">+${row.permissions.length - 4}</span>` : '';
			return `<div class="flex flex-wrap gap-1">${chips}${extra}</div>`;
		}},
		{ key: 'user_count', label: t('users.roles.user_count', 'Users'), sortable: true, render: (v: number) => `<span class="rounded-full px-2 py-0.5 text-xs font-bold" style="background-color: color-mix(in srgb, var(--color-primary) 10%, transparent); color: var(--color-primary)">${v}</span>` },
		{ key: 'actions', label: t('users.roles.actions', 'Actions'), class: 'w-36', component: RoleActionsCell }
	];

	async function load() {
		loading = true;
		try {
			const params = new URLSearchParams({
				page_size: String(pageSize),
				page_offset: String(currentPage * pageSize)
			});
			if (searchQuery) params.set('search', searchQuery);
			const [rolesResult, permsResult] = await Promise.all([
				userManagementApi.listRoles(params.toString()),
				permissionsApi.list()
			]);
			if (rolesResult.ok) {
				const d = rolesResult.data as unknown as { roles: Role[]; total: number; page: number; total_pages: number };
				roles = d.roles ?? [];
				totalItems = d.total ?? 0;
				totalPages = d.total_pages ?? 1;
			}
			if (permsResult.ok) allPermissions = permsResult.data;
		} catch { /* ignore */ }
		finally { loading = false; }
	}

	function openCreate() {
		editingRole = null;
		formName = '';
		formPermissions = [];
		saveError = '';
		showModal = true;
	}

	function openEdit(role: Role) {
		editingRole = role;
		formName = role.name;
		formPermissions = [...role.permissions];
		saveError = '';
		showModal = true;
	}

	async function handleSave() {
		if (!formName.trim()) { saveError = t('users.roles.name_required', 'Name is required'); return; }
		saving = true;
		saveError = '';
		const body = { name: formName, permissions: formPermissions };
		const result = editingRole
			? await userManagementApi.updateRole(editingRole.id, body)
			: await userManagementApi.createRole(body);
		saving = false;
		if (result.ok) {
			showModal = false;
			flash(editingRole ? t('users.roles.updated', 'Role updated') : t('users.roles.created', 'Role created'));
			load();
		} else {
			saveError = t('users.roles.save_error', 'Failed to save role');
		}
	}

	function confirmDelete(role: Role) {
		deletingRole = role;
		showDeleteConfirm = true;
	}

	async function handleDelete() {
		if (!deletingRole) return;
		showDeleteConfirm = false;
		const result = await userManagementApi.deleteRole(deletingRole.id);
		if (result.ok) { flash(t('common.saved', 'Saved')); load(); }
		else flash(t('users.roles.delete_error', 'Failed to delete role'), true);
	}

	let flashMessage = $state('');
	let flashError = $state('');
	function flash(msg: string, isError = false) {
		if (isError) { flashError = msg; flashMessage = ''; }
		else { flashMessage = msg; flashError = ''; }
		setTimeout(() => { flashMessage = ''; flashError = ''; }, 3000);
	}

	$effect(() => {
		void currentPage;
		void pageSize;
		void searchQuery;
		load();
	});
</script>

<svelte:head>
	<title>{t('users.roles.title', 'Roles')} — JapaConnect Admin</title>
</svelte:head>

<div class="mb-4 flex items-center justify-between">
	<div>
		<h1 class="text-2xl font-bold tracking-tight">{t('users.roles.title', 'Roles')}</h1>
		<p class="mt-1 text-sm" style="color: var(--color-muted)">{t('users.roles.description', 'Manage roles and their permission sets')}</p>
	</div>
	{#if canManage}<button class="btn btn-primary" onclick={openCreate}>{t('users.roles.create', 'Create Role')}</button>{/if}
</div>

{#if flashMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-success); background-color: color-mix(in srgb, var(--color-success) 8%, transparent); color: var(--color-success)">
		{flashMessage}
	</div>
{/if}
{#if flashError}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{flashError}
	</div>
{/if}

<Card>
	<CardContent>
		{#if loading}
			<TableSkeleton rows={5} columns={4} />
		{:else}
			<DataTable
				columns={columns}
				data={roles}
				searchable={true}
				onsearch={(q: string) => { searchQuery = q; currentPage = 0; }}
				pageSize={25}
				emptyMessage={t('users.roles.empty', 'No roles configured')}
			/>
			<Pagination
				bind:page={currentPage}
				{totalPages}
				{totalItems}
				bind:pageSize
			/>
		{/if}
	</CardContent>
</Card>

{#snippet RoleActionsCell({ value, row }: { value: any; row: Role })}
	<div class="flex items-center gap-1">
		{#if canManage}
			<button class="rounded px-2 py-1 text-xs transition-colors hover:opacity-80" style="color: var(--color-primary)" onclick={() => openEdit(row)}>
				{t('common.edit', 'Edit')}
			</button>
			<button class="rounded px-2 py-1 text-xs transition-colors hover:opacity-80" style="color: var(--color-danger)" onclick={() => confirmDelete(row)}>
				{t('common.delete', 'Delete')}
			</button>
		{/if}
	</div>
{/snippet}

<!-- Create / Edit Modal -->
<Modal bind:open={showModal} title={editingRole ? t('users.roles.edit_title', 'Edit Role') : t('users.roles.create_title', 'Create Role')} onConfirm={handleSave} confirmLabel={saving ? t('common.saving', 'Saving...') : t('common.save', 'Save')}>
	{#if saveError}
		<div class="mb-3 rounded-md border px-3 py-2 text-xs" style="border-color: var(--color-danger); color: var(--color-danger)">{saveError}</div>
	{/if}
	<div class="space-y-4">
		<div>
			<label for="role-name" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('users.roles.name', 'Name')}</label>
			<input
				id="role-name"
				type="text"
				class="w-full rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-surface)"
				placeholder={t('users.roles.name_placeholder', 'e.g. content_moderator')}
				bind:value={formName}
			/>
		</div>
		<div>
			<p class="mb-2 text-xs font-medium" style="color: var(--color-muted)">{t('users.roles.select_permissions', 'Select permissions for this role')}</p>
			<input
				type="text"
				class="mb-2 w-full rounded-md border px-3 py-1.5 text-xs"
				style="border-color: var(--color-border); background-color: var(--color-surface)"
				placeholder={t('users.roles.search_permissions', 'Search permissions...')}
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

<!-- Delete Confirm Modal -->
<Modal bind:open={showDeleteConfirm} title={t('common.delete', 'Delete')} onConfirm={handleDelete} confirmLabel={t('common.delete', 'Delete')}>
	<p class="text-sm" style="color: var(--color-muted)">{t('users.roles.delete_confirm', 'Are you sure you want to delete this role? This cannot be undone.')}</p>
</Modal>
