<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	// /users/groups — migrated to the EntityPage scaffold (admin AGENTS.md
	// mandate for entity list pages).
	//
	// §03/ADR-003 three-tier RBAC: Groups → Roles → Permissions. Groups are
	// collections of users; permissions live only on roles. This page manages
	// members and which roles are attached (Roles sub-flow) — permission sets
	// themselves are edited on /users/roles. Editing raw permissions here was
	// removed because the backend materializes each group's permission set as
	// a hidden "<name> Role" user_roles row, so every group appeared twice —
	// once here and once on the roles page editing the same grants.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { userManagementApi, api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { Shield, Trash2, Users2 } from 'lucide-svelte';

	type GroupRole = { id: string; name: string; slug?: string };
	type GroupMember = { id: string; username: string; display_name: string };
	type Group = {
		id: string;
		name: string;
		slug: string;
		description: string;
		is_system: boolean;
		member_count: number;
		permissions: string[];
	};

	// ── Members sub-flow ──────────────────────────────────────────────
	let entityController = $state<EntityPageController | undefined>(undefined);

	let membersGroup = $state<Group | null>(null);
	let showMembersModal = $state(false);
	let members = $state<GroupMember[]>([]);
	let membersTotal = $state(0);
	let membersPage = $state(0);
	let membersSearch = $state('');
	let membersLoading = $state(false);
	let membersError = $state('');
	const membersPageSize = 25;

	// Add-member state — resolves a user by search (admin users list).
	let addMemberQuery = $state('');
	let addMemberOptions = $state<Array<{ id: string; username: string; display_name?: string }>>([]);
	let addMemberSelected = $state<string>('');
	let memberActionError = $state('');

	async function loadMembers() {
		if (!membersGroup) return;
		membersLoading = true;
		const params = new URLSearchParams({
			page_size: String(membersPageSize),
			page_offset: String(membersPage * membersPageSize)
		});
		if (membersSearch) params.set('search', membersSearch);
		const result = await userManagementApi.listGroupMembers(membersGroup.id, params.toString());
		if (result.ok) {
			members = result.data.members ?? [];
			membersTotal = result.data.total ?? 0;
			membersError = '';
		} else {
			membersError = result.error ?? t('users.groups.members_error', 'Failed to load members');
		}
		membersLoading = false;
	}

	function openMembers(group: Group) {
		membersGroup = group;
		membersPage = 0;
		membersSearch = '';
		membersError = '';
		memberActionError = '';
		addMemberQuery = '';
		addMemberOptions = [];
		addMemberSelected = '';
		showMembersModal = true;
		// No explicit loadMembers() — the $effect below fires on
		// showMembersModal/page/search changes and performs the fetch.
	}

	$effect(() => {
		if (!showMembersModal) return;
		void membersPage;
		void membersSearch;
		loadMembers();
	});

	async function searchUsers(q: string) {
		const params = new URLSearchParams({ page_size: '10', search: q });
		const result = await api<{ users: Array<{ id: string; username: string; display_name?: string }> }>(`/admin/users?${params}`);
		if (result.ok) addMemberOptions = result.data.users ?? [];
	}

	async function addMember() {
		if (!membersGroup || !addMemberSelected) return;
		const result = await userManagementApi.addGroupMember(membersGroup.id, addMemberSelected);
		if (result.ok) {
			addMemberSelected = '';
			addMemberQuery = '';
			addMemberOptions = [];
			memberActionError = '';
			loadMembers();
			entityController?.reload();
		} else {
			memberActionError = result.error ?? t('users.groups.member_add_error', 'Failed to add member');
		}
	}

	async function removeMember(userId: string) {
		if (!membersGroup) return;
		const result = await userManagementApi.removeGroupMember(membersGroup.id, userId);
		if (result.ok) {
			memberActionError = '';
			loadMembers();
			entityController?.reload();
		} else {
			memberActionError = result.error ?? t('users.groups.member_remove_error', 'Failed to remove member');
		}
	}

	// ── Roles sub-flow ────────────────────────────────────────────────
	// Attaches/detaches named permission sets via the group_roles join —
	// the spec'd way groups get permissions (§03). The group's effective
	// permission list shown in the table/view is the union of its roles.
	let rolesGroup = $state<Group | null>(null);
	let showRolesModal = $state(false);
	let groupRoles = $state<GroupRole[]>([]);
	let rolesLoading = $state(false);
	let rolesError = $state('');
	let roleActionError = $state('');
	let allRoles = $state<GroupRole[]>([]);
	let addRoleSelected = $state('');

	// Roles not already attached to this group — picker options.
	const attachableRoles = $derived(
		allRoles.filter((r) => !groupRoles.some((gr) => gr.id === r.id))
	);

	async function loadGroupRoles() {
		if (!rolesGroup) return;
		rolesLoading = true;
		const result = await userManagementApi.listGroupRoles(rolesGroup.id);
		if (result.ok) {
			groupRoles = result.data.roles ?? [];
			rolesError = '';
		} else {
			rolesError = result.error ?? t('users.groups.roles_error', 'Failed to load roles');
		}
		rolesLoading = false;
	}

	async function loadAllRoles() {
		// Picker needs the full set — page_size is capped at 100 server-side,
		// which covers any realistic role catalogue.
		const result = await userManagementApi.listRoles('page_size=100');
		if (result.ok) {
			allRoles = result.data.roles ?? [];
		} else {
			roleActionError = result.error ?? t('users.groups.roles_error', 'Failed to load roles');
		}
	}

	function openRoles(group: Group) {
		rolesGroup = group;
		groupRoles = [];
		rolesError = '';
		roleActionError = '';
		addRoleSelected = '';
		showRolesModal = true;
		loadGroupRoles();
		if (allRoles.length === 0) loadAllRoles();
	}

	async function attachRole() {
		if (!rolesGroup || !addRoleSelected) return;
		const result = await userManagementApi.assignRoleToGroup(rolesGroup.id, addRoleSelected);
		if (result.ok) {
			addRoleSelected = '';
			roleActionError = '';
			loadGroupRoles();
			entityController?.reload();
		} else {
			roleActionError = result.error ?? t('users.groups.role_add_error', 'Failed to attach role');
		}
	}

	async function detachRole(roleId: string) {
		if (!rolesGroup) return;
		const result = await userManagementApi.removeRoleFromGroup(rolesGroup.id, roleId);
		if (result.ok) {
			roleActionError = '';
			loadGroupRoles();
			entityController?.reload();
		} else {
			roleActionError = result.error ?? t('users.groups.role_remove_error', 'Failed to detach role');
		}
	}

	// Bulk delete uses the built-in confirm-gated "Delete Selected" — a
	// page-declared destructive entry with that label is auto-replaced by the
	// scaffold, which runs config.remove per row and collects failures.

	const fields: FieldDef[] = [
		{ key: 'name', label: t('users.groups.name', 'Name'), type: 'text', required: true },
		{ key: 'description', label: t('users.groups.description_label', 'Description'), type: 'textarea' }
	];

	const config: EntityPageConfig = {
		title: t('users.groups.title', 'User Groups'),
		description: t('users.groups.description', 'Manage user groups, their members and attached roles'),
		createLabel: t('users.groups.create', 'Create Group'),
		createPermission: 'admin.rbac.manage',
		pageSize: 25,
		columns: [
			{ key: 'name', label: t('users.groups.name', 'Name'), sortable: true },
			{
				key: 'slug',
				label: 'Slug',
				defaultHidden: true,
				render: (v: string) => `<span class="font-mono text-xs" style="color: var(--color-muted)">${v ?? ''}</span>`
			},
			{
				key: 'is_system',
				label: t('users.groups.type', 'Type'),
				render: (v: boolean) =>
					v
						? `<span class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-semibold" style="background-color: color-mix(in srgb, var(--color-warning) 15%, transparent); color: var(--color-foreground)">system</span>`
						: ''
			},
			{ key: 'description', label: t('users.groups.description_label', 'Description') },
			{
				key: 'member_count',
				label: t('users.groups.members', 'Members'),
				sortable: true,
				render: (v: number) => `<span class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-semibold" style="background-color: color-mix(in srgb, var(--color-primary) 10%, transparent); color: var(--color-primary)">${v}</span>`
			},
			{
				key: 'permissions',
				label: t('users.groups.effective_permissions', 'Effective permissions'),
				render: (v: string[]) => {
					if (!v || !Array.isArray(v)) return '';
					const shown = v.slice(0, 3);
					const rest = v.length > 3 ? `<span class="inline-flex items-center rounded-full px-2 py-0.5 text-xs" style="background-color: var(--color-surface); color: var(--color-muted)">+${v.length - 3}</span>` : '';
					return `<div class="flex flex-wrap gap-1">${shown.map((p) => `<span class="inline-flex items-center rounded-full px-2 py-0.5 text-xs" style="background-color: var(--color-surface); color: var(--color-muted)">${p}</span>`).join('')}${rest}</div>`;
				}
			}
		],
		actions: [
			// Custom actions replace the defaults entirely (scaffold contract) —
			// View/Edit/Delete are re-declared here via the ActionContext so the
			// standard modals still work alongside the members sub-flow.
			{
				label: t('common.view', 'View'),
				onclick: (_row: Group, ctx) => ctx?.openView(_row)
			},
			{
				label: t('common.edit', 'Edit'),
				onclick: (_row: Group, ctx) => ctx?.openEdit(_row)
			},
			{
				label: t('users.groups.view_members', 'Members'),
				icon: Users2,
				onclick: (row: Group) => openMembers(row)
			},
			{
				label: t('users.groups.view_roles', 'Roles'),
				icon: Shield,
				onclick: (row: Group) => openRoles(row)
			},
			{
				label: t('common.delete', 'Delete'),
				icon: Trash2,
				variant: 'destructive',
				onclick: (row: Group, ctx) => ctx?.openDelete(row),
				show: (row: Group) => !row.is_system
			}
		],
		fields,
		load: async ({ page, pageSize, q, sort }) => {
			const params = new URLSearchParams({
				page_size: String(pageSize),
				page_offset: String(page * pageSize)
			});
			if (q) params.set('search', q);
			if (sort) {
				const [key, dir] = sort.split(':');
				params.set('sort', key);
				params.set('sort_dir', dir === 'desc' ? 'desc' : 'asc');
			}
			const result = await userManagementApi.listGroups(params.toString());
			if (result.ok) {
				const d = result.data;
				return {
					data: d.groups ?? [],
					total: d.total ?? 0,
					totalPages: d.total_pages ?? (Math.ceil((d.total ?? 0) / pageSize) || 1)
				};
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load groups'));
		},
		create: async (body) => {
			// No permissions on create — a group starts with no roles attached;
			// permission sets are attached via the Roles action (§03 group→role).
			const res = await userManagementApi.createGroup({
				name: String(body.name ?? '').trim(),
				description: String(body.description ?? '')
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			// IMPORTANT: omit `permissions` entirely — the backend treats a
			// non-nil permissions field as a full-set replace on the group's
			// first attached role, so sending [] would wipe that role's grants.
			const res = await userManagementApi.updateGroup(id, {
				name: String(body.name ?? '').trim(),
				description: String(body.description ?? '')
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await userManagementApi.deleteGroup(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: GroupView,
		formSnippet: GroupForm
	};
</script>

<svelte:head>
	<title>{t('users.groups.title', 'User Groups')} — {siteName()} Admin</title>
</svelte:head>

<EntityPage {config} bind:controller={entityController} />

<!-- ── Snippets ─────────────────────────────────────────────────────────── -->

<!-- View modal body -->
{#snippet GroupView({ row }: { row: Group })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('users.groups.name', 'Name')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.name}
				{#if row.is_system}
					<span class="ml-1 rounded-full px-2 py-0.5 text-xs" style="background-color: color-mix(in srgb, var(--color-warning) 15%, transparent)">system</span>
				{/if}
			</p>
			<p class="mt-0.5 font-mono text-xs" style="color: var(--color-muted)">{row.slug}</p>
		</div>
		{#if row.description}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('users.groups.description_label', 'Description')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.description}</p>
			</div>
		{/if}
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('users.groups.members', 'Members')}</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.member_count}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('users.groups.effective_permissions', 'Effective permissions')}</p>
			{#if !row.permissions || row.permissions.length === 0}
				<p class="mt-1 text-sm" style="color: var(--color-muted)">—</p>
			{:else}
				<div class="mt-1 flex flex-wrap gap-1">
					{#each row.permissions as perm (perm)}
						<span class="inline-block rounded-full px-2 py-0.5 text-xs" style="background-color: color-mix(in srgb, var(--color-secondary) 15%, transparent); color: var(--color-foreground)">{perm}</span>
					{/each}
				</div>
			{/if}
		</div>
	</div>
{/snippet}

<!-- Create/Edit form body — permissions are managed via the Roles action,
     not edited directly on the group (§03 three-tier RBAC). -->
{#snippet GroupForm({ value, editing }: { value: Record<string, any>; editing: any | null })}
	<div class="space-y-4">
		<div>
			<label for="group-name" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('users.groups.name', 'Name')}</label>
			<input
				id="group-name"
				type="text"
				class="w-full rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
				placeholder={t('users.groups.name_placeholder', 'Enter group name')}
				bind:value={value.name}
			/>
			{#if editing?.is_system}
				<p class="mt-1 text-xs" style="color: var(--color-muted)">{t('users.groups.system_slug_locked', 'System group — the slug is locked.')}</p>
			{:else if editing}
				<p class="mt-1 font-mono text-xs" style="color: var(--color-muted)">slug: {editing.slug}</p>
			{/if}
		</div>
		<div>
			<label for="group-desc" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('users.groups.description_label', 'Description')}</label>
			<textarea
				id="group-desc"
				rows="2"
				class="w-full rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
				placeholder={t('users.groups.desc_placeholder', 'Enter group description')}
				bind:value={value.description}
			></textarea>
		</div>
		<p class="text-xs" style="color: var(--color-muted)">{t('users.groups.roles_form_hint', 'Permission sets are attached to the group as roles — use the Roles action on the group row.')}</p>
	</div>
{/snippet}

<!-- Members modal -->
<Modal bind:open={showMembersModal} title={membersGroup ? `${t('users.groups.members', 'Members')} — ${membersGroup.name}` : t('users.groups.members', 'Members')} size="lg">
	<div class="space-y-3">
		{#if memberActionError}
			<div class="rounded-md border px-3 py-2 text-xs" style="border-color: var(--color-danger); color: var(--color-danger)">{memberActionError}</div>
		{/if}
		<div class="flex gap-2">
			<input
				type="text"
				class="flex-1 rounded-md border px-3 py-1.5 text-xs"
				style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
				placeholder={t('users.groups.search_members', 'Search members...')}
				bind:value={membersSearch}
				oninput={() => { membersPage = 0; }}
			/>
		</div>

		{#if membersError}
			<p class="text-xs" style="color: var(--color-danger)">{membersError}</p>
		{:else if membersLoading}
			<p class="text-xs" style="color: var(--color-muted)">{t('common.loading', 'Loading...')}</p>
		{:else if members.length === 0}
			<p class="text-xs" style="color: var(--color-muted)">{t('users.groups.no_members', 'No members in this group.')}</p>
		{:else}
			<div class="max-h-64 overflow-y-auto">
				{#each members as m (m.id)}
					<div class="flex items-center justify-between gap-2 flex-wrap border-b py-1.5 text-xs" style="border-color: var(--color-border)">
						<span>
							<span class="font-medium" style="color: var(--color-foreground)">{m.username}</span>
							{#if m.display_name}<span style="color: var(--color-muted)"> — {m.display_name}</span>{/if}
						</span>
						{#if !membersGroup?.is_system}
							<button
								class="rounded px-2 py-0.5 text-xs"
								style="color: var(--color-danger)"
								onclick={() => removeMember(m.id)}
							>{t('common.remove', 'Remove')}</button>
						{/if}
					</div>
				{/each}
			</div>
			{#if membersTotal > membersPageSize}
				<div class="flex items-center justify-between gap-2 flex-wrap text-xs" style="color: var(--color-muted)">
					<Button variant="outline" size="sm" disabled={membersPage === 0} onclick={() => { membersPage -= 1; }}>{t('common.previous', 'Previous')}</Button>
					<span>{membersPage + 1} / {Math.ceil(membersTotal / membersPageSize)}</span>
					<Button variant="outline" size="sm" disabled={(membersPage + 1) * membersPageSize >= membersTotal} onclick={() => { membersPage += 1; }}>{t('common.next', 'Next')}</Button>
				</div>
			{/if}
		{/if}

		{#if !membersGroup?.is_system}
			<div class="border-t pt-3" style="border-color: var(--color-border)">
				<p class="mb-2 text-xs font-medium" style="color: var(--color-muted)">{t('users.groups.add_member', 'Add member')}</p>
				<div class="flex gap-2">
					<input
						type="text"
						class="flex-1 rounded-md border px-3 py-1.5 text-xs"
						style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
						placeholder={t('users.groups.search_users', 'Search users by username...')}
						bind:value={addMemberQuery}
						oninput={() => { if (addMemberQuery.length >= 2) searchUsers(addMemberQuery); else addMemberOptions = []; }}
					/>
					<Button size="sm" onclick={addMember} disabled={!addMemberSelected}>{t('common.add', 'Add')}</Button>
				</div>
				{#if addMemberOptions.length > 0}
					<div class="mt-1 max-h-32 overflow-y-auto rounded-md border" style="border-color: var(--color-border)">
						{#each addMemberOptions as u (u.id)}
							<button
								class="block w-full px-3 py-1.5 text-left text-xs hover:opacity-80"
								style="color: var(--color-foreground); background-color: {addMemberSelected === u.id ? 'var(--color-surface)' : 'transparent'}"
								onclick={() => { addMemberSelected = u.id; addMemberQuery = u.username; addMemberOptions = []; }}
							>
								{u.username}{#if u.display_name} — {u.display_name}{/if}
							</button>
						{/each}
					</div>
				{/if}
			</div>
		{/if}
	</div>
	{#snippet footer()}
		<Button variant="outline" onclick={() => { showMembersModal = false; }}>{t('common.close', 'Close')}</Button>
	{/snippet}
</Modal>

<!-- Roles modal — attach/detach named permission sets (§03 group→role).
     Permission sets themselves are edited on /users/roles. -->
<Modal bind:open={showRolesModal} title={rolesGroup ? `${t('users.groups.roles', 'Roles')} — ${rolesGroup.name}` : t('users.groups.roles', 'Roles')} size="lg">
	<div class="space-y-3">
		{#if roleActionError}
			<div class="rounded-md border px-3 py-2 text-xs" style="border-color: var(--color-danger); color: var(--color-danger)">{roleActionError}</div>
		{/if}

		{#if rolesError}
			<p class="text-xs" style="color: var(--color-danger)">{rolesError}</p>
		{:else if rolesLoading}
			<p class="text-xs" style="color: var(--color-muted)">{t('common.loading', 'Loading...')}</p>
		{:else if groupRoles.length === 0}
			<p class="text-xs" style="color: var(--color-muted)">{t('users.groups.no_roles', 'No roles attached — this group grants no permissions.')}</p>
		{:else}
			<div class="max-h-64 overflow-y-auto">
				{#each groupRoles as r (r.id)}
					<div class="flex items-center justify-between gap-2 border-b py-1.5 text-xs" style="border-color: var(--color-border)">
						<span class="font-medium" style="color: var(--color-foreground)">{r.name}</span>
						{#if !rolesGroup?.is_system}
							<button
								class="rounded px-2 py-0.5 text-xs"
								style="color: var(--color-danger)"
								onclick={() => detachRole(r.id)}
							>{t('common.remove', 'Remove')}</button>
						{/if}
					</div>
				{/each}
			</div>
		{/if}

		{#if !rolesGroup?.is_system}
			<div class="border-t pt-3" style="border-color: var(--color-border)">
				<label for="attach-role-select" class="mb-2 block text-xs font-medium" style="color: var(--color-muted)">{t('users.groups.attach_role', 'Attach role')}</label>
				<div class="flex gap-2">
					<select
						id="attach-role-select"
						class="flex-1 rounded-md border px-3 py-1.5 text-xs"
						style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
						bind:value={addRoleSelected}
					>
						<option value="">{t('users.groups.select_role', 'Select a role...')}</option>
						{#each attachableRoles as r (r.id)}
							<option value={r.id}>{r.name}</option>
						{/each}
					</select>
					<Button size="sm" onclick={attachRole} disabled={!addRoleSelected}>{t('common.add', 'Add')}</Button>
				</div>
				<p class="mt-2 text-xs" style="color: var(--color-muted)">{t('users.groups.roles_hint', 'Manage a role’s permission set on the Roles page.')}</p>
			</div>
		{/if}
	</div>
	{#snippet footer()}
		<Button variant="outline" onclick={() => { showRolesModal = false; }}>{t('common.close', 'Close')}</Button>
	{/snippet}
</Modal>
