<script lang="ts">
	import Modal from '$lib/components/Modal.svelte';
	import { userManagementApi, permissionsApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

	type Role = {
		id: string;
		name: string;
		permissions: string[];
		user_count: number;
	};

	let roles = $state<Role[]>([]);
	let loading = $state(true);
	let showModal = $state(false);
	let editingRole = $state<Role | null>(null);
	let formName = $state('');
	let formPermissions = $state<string[]>([]);
	let saveError = $state('');
	let saving = $state(false);
	let showDeleteConfirm = $state(false);
	let deletingRole = $state<Role | null>(null);

	let allPermissions = $state<string[]>([]);
	let permSearch = $state('');

	const filteredPermissions = $derived(
		permSearch
			? allPermissions.filter((p) => p.toLowerCase().includes(permSearch.toLowerCase()))
			: allPermissions
	);

	async function load() {
		loading = true;
		try {
			const [rolesResult, permsResult] = await Promise.all([
				userManagementApi.listRoles(),
				permissionsApi.list()
			]);
			if (rolesResult.ok) roles = rolesResult.data.roles;
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
		if (!formName.trim()) { saveError = t('users.roles.name_required' as never); return; }
		saving = true;
		saveError = '';
		const body = { name: formName, permissions: formPermissions };
		const result = editingRole
			? await userManagementApi.updateRole(editingRole.id, body)
			: await userManagementApi.createRole(body);
		saving = false;
		if (result.ok) {
			showModal = false;
			flash(editingRole ? t('users.roles.updated') : t('users.roles.created'));
			load();
		} else {
			saveError = t('users.roles.save_error');
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
		if (result.ok) { flash(t('common.saved')); load(); }
		else flash(t('users.roles.delete_error'), true);
	}

	let flashMessage = $state('');
	let flashError = $state('');
	function flash(msg: string, isError = false) {
		if (isError) { flashError = msg; flashMessage = ''; }
		else { flashMessage = msg; flashError = ''; }
		setTimeout(() => { flashMessage = ''; flashError = ''; }, 3000);
	}

	$effect(() => {
		load();
	});
</script>

<svelte:head>
	<title>{t('users.roles.title')} — JapaConnect Admin</title>
</svelte:head>

<div class="mb-4 flex items-center justify-between">
	<div>
		<h1 class="text-2xl font-bold tracking-tight">{t('users.roles.title')}</h1>
		<p class="mt-1 text-sm" style="color: var(--color-muted)">{t('users.roles.description')}</p>
	</div>
	<button class="btn btn-primary" onclick={openCreate}>{t('users.roles.create')}</button>
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

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-color: var(--color-border)">
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('users.roles.name')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('users.roles.permissions')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('users.roles.user_count')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('users.roles.actions')}</th>
				</tr>
			</thead>
			<tbody>
				{#each roles as role}
					<tr class="border-t" style="border-color: var(--color-border)">
						<td class="px-4 py-3 font-medium">{role.name}</td>
						<td class="px-4 py-3">
							<div class="flex flex-wrap gap-1">
								{#each role.permissions.slice(0, 4) as perm}
									<span class="inline-block rounded-full bg-black/5 px-2 py-0.5 text-xs dark:bg-white/10">{perm}</span>
								{/each}
								{#if role.permissions.length > 4}
									<span class="inline-block rounded-full bg-black/5 px-2 py-0.5 text-xs dark:bg-white/10">+{role.permissions.length - 4}</span>
								{/if}
							</div>
						</td>
						<td class="px-4 py-3">
							<span class="rounded-full bg-black/5 px-2 py-0.5 text-xs font-bold dark:bg-white/10">{role.user_count}</span>
						</td>
						<td class="px-4 py-3">
							<div class="flex gap-1">
								<button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10" style="color: var(--color-primary)" onclick={() => openEdit(role)}>
									{t('common.edit')}
								</button>
								<button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10" style="color: var(--color-danger)" onclick={() => confirmDelete(role)}>
									{t('common.delete')}
								</button>
							</div>
						</td>
					</tr>
				{:else}
					<tr>
						<td colspan="4" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							{t('users.roles.empty')}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}

<!-- Create / Edit Modal -->
<Modal bind:open={showModal} title={editingRole ? t('users.roles.edit_title') : t('users.roles.create_title')} onConfirm={handleSave} confirmLabel={saving ? t('common.saving') : t('common.save')}>
	{#if saveError}
		<div class="mb-3 rounded-md border px-3 py-2 text-xs" style="border-color: var(--color-danger); color: var(--color-danger)">{saveError}</div>
	{/if}
	<div class="space-y-4">
		<div>
			<label for="role-name" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('users.roles.name')}</label>
			<input
				id="role-name"
				type="text"
				class="w-full rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-surface)"
				placeholder={t('users.roles.name_placeholder')}
				bind:value={formName}
			/>
		</div>
		<div>
			<p class="mb-2 text-xs font-medium" style="color: var(--color-muted)">{t('users.roles.select_permissions')}</p>
			<input
				type="text"
				class="mb-2 w-full rounded-md border px-3 py-1.5 text-xs"
				style="border-color: var(--color-border); background-color: var(--color-surface)"
				placeholder="Search permissions..."
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
<Modal bind:open={showDeleteConfirm} title={t('common.delete')} onConfirm={handleDelete} confirmLabel={t('common.delete')}>
	<p class="text-sm" style="color: var(--color-muted)">{t('users.roles.delete_confirm')}</p>
</Modal>
