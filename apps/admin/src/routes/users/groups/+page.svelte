<script lang="ts">
	import { onMount } from 'svelte';
	import Modal from '$lib/components/Modal.svelte';
	import { userManagementApi, permissionsApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

	type Group = {
		id: string;
		name: string;
		description: string;
		member_count: number;
		permissions: string[];
	};

	let groups = $state<Group[]>([]);
	let loading = $state(true);
	let showModal = $state(false);
	let editingGroup = $state<Group | null>(null);
	let formName = $state('');
	let formDescription = $state('');
	let formPermissions = $state<string[]>([]);
	let saveError = $state('');
	let saving = $state(false);
	let showDeleteConfirm = $state(false);
	let deletingGroup = $state<Group | null>(null);

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
			const [groupsResult, permsResult] = await Promise.all([
				userManagementApi.listGroups(),
				permissionsApi.list()
			]);
			if (groupsResult.ok) groups = groupsResult.data.groups;
			if (permsResult.ok) allPermissions = permsResult.data;
		} catch { /* ignore */ }
		finally { loading = false; }
	}

	function openCreate() {
		editingGroup = null;
		formName = '';
		formDescription = '';
		formPermissions = [];
		saveError = '';
		showModal = true;
	}

	function openEdit(group: Group) {
		editingGroup = group;
		formName = group.name;
		formDescription = group.description;
		formPermissions = [...group.permissions];
		saveError = '';
		showModal = true;
	}

	async function handleSave() {
		if (!formName.trim()) { saveError = t('users.groups.name_required'); return; }
		saving = true;
		saveError = '';
		const body = { name: formName, description: formDescription, permissions: formPermissions };
		const result = editingGroup
			? await userManagementApi.updateGroup(editingGroup.id, body)
			: await userManagementApi.createGroup(body);
		saving = false;
		if (result.ok) {
			showModal = false;
			flash(editingGroup ? t('users.groups.updated') : t('users.groups.created'));
			load();
		} else {
			saveError = t('users.groups.save_error');
		}
	}

	function confirmDelete(group: Group) {
		deletingGroup = group;
		showDeleteConfirm = true;
	}

	async function handleDelete() {
		if (!deletingGroup) return;
		showDeleteConfirm = false;
		const result = await userManagementApi.deleteGroup(deletingGroup.id);
		if (result.ok) { flash(t('common.saved')); load(); }
		else flash(t('users.groups.delete_error'), true);
	}

	let flashMessage = $state('');
	let flashError = $state('');
	function flash(msg: string, isError = false) {
		if (isError) { flashError = msg; flashMessage = ''; }
		else { flashMessage = msg; flashError = ''; }
		setTimeout(() => { flashMessage = ''; flashError = ''; }, 3000);
	}

	onMount(load);
</script>

<svelte:head>
	<title>{t('users.groups.title')} — JapaConnect Admin</title>
</svelte:head>

<div class="mb-4 flex items-center justify-between">
	<div>
		<h1 class="text-2xl font-bold tracking-tight">{t('users.groups.title')}</h1>
		<p class="mt-1 text-sm" style="color: var(--color-muted)">{t('users.groups.description')}</p>
	</div>
	<button class="btn btn-primary" onclick={openCreate}>{t('users.groups.create')}</button>
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
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('users.groups.name')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('users.groups.description_label')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('users.groups.members')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('users.groups.permissions')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Actions</th>
				</tr>
			</thead>
			<tbody>
				{#each groups as group}
					<tr class="border-t" style="border-color: var(--color-border)">
						<td class="px-4 py-3 font-medium">{group.name}</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{group.description}</td>
						<td class="px-4 py-3">
							<span class="rounded-full bg-black/5 px-2 py-0.5 text-xs font-bold dark:bg-white/10">{group.member_count}</span>
						</td>
						<td class="px-4 py-3">
							<div class="flex flex-wrap gap-1">
								{#each group.permissions.slice(0, 3) as perm}
									<span class="inline-block rounded-full bg-black/5 px-2 py-0.5 text-xs dark:bg-white/10">{perm}</span>
								{/each}
								{#if group.permissions.length > 3}
									<span class="inline-block rounded-full bg-black/5 px-2 py-0.5 text-xs dark:bg-white/10">+{group.permissions.length - 3}</span>
								{/if}
							</div>
						</td>
						<td class="px-4 py-3">
							<div class="flex gap-1">
								<button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10" style="color: var(--color-primary)" onclick={() => openEdit(group)}>
									{t('common.edit')}
								</button>
								<button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10" style="color: var(--color-danger)" onclick={() => confirmDelete(group)}>
									{t('common.delete')}
								</button>
							</div>
						</td>
					</tr>
				{:else}
					<tr>
						<td colspan="5" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							{t('users.groups.empty')}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}

<!-- Create / Edit Modal -->
<Modal bind:open={showModal} title={editingGroup ? t('users.groups.edit_title') : t('users.groups.create_title')} onConfirm={handleSave} confirmLabel={saving ? t('common.saving') : t('common.save')}>
	{#if saveError}
		<div class="mb-3 rounded-md border px-3 py-2 text-xs" style="border-color: var(--color-danger); color: var(--color-danger)">{saveError}</div>
	{/if}
	<div class="space-y-4">
		<div>
			<label for="group-name" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('users.groups.name')}</label>
			<input
				id="group-name"
				type="text"
				class="w-full rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-surface)"
				placeholder={t('users.groups.name_placeholder')}
				bind:value={formName}
			/>
		</div>
		<div>
			<label for="group-desc" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('users.groups.description_label')}</label>
			<textarea
				id="group-desc"
				rows="2"
				class="w-full rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-surface)"
				placeholder={t('users.groups.desc_placeholder')}
				bind:value={formDescription}
			></textarea>
		</div>
		<div>
			<p class="mb-2 text-xs font-medium" style="color: var(--color-muted)">{t('users.groups.permissions')}</p>
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
	<p class="text-sm" style="color: var(--color-muted)">{t('users.groups.delete_confirm')}</p>
</Modal>
