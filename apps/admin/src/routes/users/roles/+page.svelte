<script lang="ts">
	import { siteName } from '#lib/stores/site.svelte.js';
	// /users/roles — migrated to the EntityPage scaffold.
	//
	// The role form has a searchable permissions checkbox grid that EntityForm
	// cannot express, so it is rendered via the `formSnippet` escape hatch.
	// The full permissions list is fetched on mount (mirrors the legacy load).
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '#lib/components/entity/index.js';
	import { userManagementApi, permissionsApi } from '#lib/api.js';
	import { t } from '#lib/stores/i18n.svelte.js';
	import { user } from '#lib/stores/auth.svelte.js';
	import PermissionGrid from '#lib/components/ui/PermissionGrid.svelte';
	import { Modal } from '#lib/components/ui/modal/index.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Trash2 } from 'lucide-svelte';

	// The wildcard grant is owner-only on the backend — mirror that in the
	// picker so non-owners see it locked instead of a 403 on save.
	const isOwner = $derived(!!user.current?.is_owner);

	type Role = {
		id: string;
		name: string;
		slug?: string;
		permissions: string[];
		user_count: number;
		group_count?: number;
		is_system?: boolean;
	};

	// Full permission catalogue — fetched once on mount so the form can render
	// the searchable checkbox grid without hardcoding the permission list.
	let allPermissions = $state<string[]>([]);

	async function loadAllPermissions() {
		try {
			const result = await permissionsApi.list();
			if (result.ok) allPermissions = result.data ?? [];
		} catch {
			allPermissions = [];
		}
	}

	$effect(() => {
		void loadAllPermissions();
	});

	// Permission-picker search state (used by the formSnippet below).
	let permSearch = $state('');
	const filteredPermissions = $derived(
		permSearch
			? allPermissions.filter((p) => p.toLowerCase().includes(permSearch.toLowerCase()))
			: allPermissions
	);

	// ── Bulk delete ───── EntityPage auto-reloads + clears selection after a bulk action handler completes.
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let bulkDeleting = $state(false);

	// ── Delete confirm ── a role attached to groups strips those groups'
	// grants on delete; the page-local modal spells out the impact before
	// confirming (the scaffold's built-in confirm is generic).
	let deleteTarget = $state<Role | null>(null);
	let showDeleteModal = $state(false);
	let deleteError = $state('');
	let deleting = $state(false);

	function askDeleteRole(role: Role) {
		deleteTarget = role;
		deleteError = '';
		showDeleteModal = true;
	}

	async function confirmDeleteRole() {
		if (!deleteTarget || deleting) return;
		deleting = true;
		const result = await userManagementApi.deleteRole(deleteTarget.id);
		deleting = false;
		if (result.ok) {
			showDeleteModal = false;
			deleteTarget = null;
			entityController?.reload();
		} else {
			deleteError = result.error ?? t('users.roles.delete_error', 'Failed to delete role');
		}
	}

	async function bulkDelete(ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0) return;
		if (bulkDeleting) return; // in-flight guard
		bulkDeleting = true;
		pageError = '';
		try {
			const results = await Promise.all(idArr.map((id) => userManagementApi.deleteRole(id)));
			const failed = results.filter((r) => !r.ok);
			if (failed.length > 0) {
				pageError = failed[0].error ?? t('users.roles.delete_error', 'Failed to delete role');
			}
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('users.roles.delete_error', 'Failed to delete role');
		} finally {
			bulkDeleting = false;
		}
	}

	// `fields` seeds the edit form value; the actual inputs are rendered by the
	// `formSnippet` below (the searchable permissions grid).
	const fields: FieldDef[] = [
		{ key: 'name', label: t('users.roles.name', 'Name'), type: 'text', required: true },
		{ key: 'permissions', label: t('users.roles.permissions', 'Permissions'), type: 'text' }
	];

	const config: EntityPageConfig = {
		title: t('users.roles.title', 'Roles'),
		description: t('users.roles.description', 'Manage named permission sets — attach them to user groups from the Groups page'),
		createLabel: t('users.roles.create', 'Create Role'),
		createPermission: 'admin.rbac.manage',
		pageSize: 25,
		columns: [
			{ key: 'name', label: t('users.roles.name', 'Name'), sortable: true },
			{
				key: 'is_system',
				label: t('users.roles.type', 'Type'),
				render: (v: boolean) =>
					v
						? `<span class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-semibold" style="background-color: color-mix(in srgb, var(--color-warning) 15%, transparent); color: var(--color-foreground)">system</span>`
						: ''
			},
			{
				key: 'permissions',
				label: t('users.roles.permissions', 'Permissions'),
				render: (_: any, row: Role) => {
					const perms = row.permissions ?? [];
					const chips = perms.slice(0, 4).map((p: string) => `<span class="inline-block rounded-full px-2 py-0.5 text-xs" style="background-color: color-mix(in srgb, var(--color-secondary) 15%, transparent); color: var(--color-foreground)">${p}</span>`).join('');
					const extra = perms.length > 4 ? `<span class="inline-block rounded-full px-2 py-0.5 text-xs" style="background-color: color-mix(in srgb, var(--color-secondary) 15%, transparent); color: var(--color-muted)">+${perms.length - 4}</span>` : '';
					return `<div class="flex flex-wrap gap-1">${chips}${extra}</div>`;
				}
			},
			{
				key: 'group_count',
				label: t('users.roles.group_count', 'Groups'),
				render: (v: number) => `<span class="rounded-full px-2 py-0.5 text-xs font-bold" style="background-color: color-mix(in srgb, var(--color-primary) 10%, transparent); color: var(--color-primary)">${v ?? 0}</span>`
			},
			{
				key: 'user_count',
				label: t('users.roles.user_count', 'Users'),
				sortable: true,
				render: (v: number) => `<span class="rounded-full px-2 py-0.5 text-xs font-bold" style="background-color: color-mix(in srgb, var(--color-primary) 10%, transparent); color: var(--color-primary)">${v}</span>`
			}
		],
		actions: [
			// Custom actions replace the defaults entirely (scaffold contract) —
			// View/Edit are re-declared via the ActionContext; Delete routes to
			// the page-local impact modal instead of the generic confirm.
			{
				label: t('common.view', 'View'),
				onclick: (_row: Role, ctx) => ctx?.openView(_row)
			},
			{
				label: t('common.edit', 'Edit'),
				onclick: (_row: Role, ctx) => ctx?.openEdit(_row)
			},
			{
				label: t('common.delete', 'Delete'),
				icon: Trash2,
				variant: 'destructive',
				onclick: (row: Role) => askDeleteRole(row),
				show: (row: Role) => !row.is_system
			}
		],
		bulkActions: [
			{
				label: t('common.deleteSelected', 'Delete Selected'),
				icon: Trash2,
				variant: 'destructive',
				onclick: bulkDelete
			}
		],
		fields,
		load: async ({ page, pageSize, q, sort }) => {
			const params = new URLSearchParams({
				page_size: String(pageSize),
				page_offset: String(page * pageSize)
			});
			if (q) params.set('search', q);
			// EntityPage passes sort as "key:dir"; the API takes sort + sort_dir.
			if (sort) {
				const [key, dir] = sort.split(':');
				params.set('sort', key);
				params.set('sort_dir', dir === 'desc' ? 'desc' : 'asc');
			}
			const result = await userManagementApi.listRoles(params.toString());
			if (result.ok) {
				const d = result.data as { roles: Role[]; total: number; total_pages: number };
				return {
					data: d.roles ?? [],
					total: d.total ?? 0,
					totalPages: d.total_pages ?? 1
				};
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load roles'));
		},
		create: async (body) => {
			const res = await userManagementApi.createRole({
				name: String(body.name ?? '').trim(),
				permissions: Array.isArray(body.permissions) ? body.permissions : []
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const res = await userManagementApi.updateRole(id, {
				name: String(body.name ?? '').trim(),
				permissions: Array.isArray(body.permissions) ? body.permissions : []
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await userManagementApi.deleteRole(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: RoleView,
		formSnippet: RoleForm
	};
</script>

<svelte:head>
	<title>{t('users.roles.title', 'Roles')} — {siteName()} Admin</title>
</svelte:head>

{#if pageError}
	<div
		class="mb-4 rounded-lg p-3 text-sm"
		style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
	>
		{pageError}
	</div>
{/if}

<EntityPage {config} bind:controller={entityController} />

<!-- ── Snippets ─────────────────────────────────────────────────────────── -->

<!-- View modal body -->
{#snippet RoleView({ row }: { row: Role })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('users.roles.name', 'Name')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.name}
				{#if row.is_system}
					<span class="ml-1 rounded-full px-2 py-0.5 text-xs" style="background-color: color-mix(in srgb, var(--color-warning) 15%, transparent)">system</span>
				{/if}
			</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('users.roles.group_count', 'Groups')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.group_count ?? 0}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('users.roles.user_count', 'Users')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.user_count}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('users.roles.permissions', 'Permissions')}</p>
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

<!-- Create/Edit form body (searchable permissions checkbox grid) -->
{#snippet RoleForm({ value, editing }: { value: Record<string, any>; editing: any | null })}
	<div class="space-y-4">
		<div>
			<label for="role-name" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('users.roles.name', 'Name')}</label>
			<input
				id="role-name"
				type="text"
				class="w-full rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-surface)"
				placeholder={t('users.roles.name_placeholder', 'e.g. content_moderator')}
				bind:value={value.name}
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
			<PermissionGrid permissions={filteredPermissions} bind:value={value.permissions} {isOwner} />
		</div>
	</div>
{/snippet}

<!-- Delete confirm — surfaces the group/user impact, which the scaffold's
     generic confirm cannot express. -->
<Modal bind:open={showDeleteModal} title={t('users.roles.delete_title', 'Delete Role')} size="default">
	<div class="space-y-3">
		{#if deleteError}
			<div class="rounded-md border px-3 py-2 text-xs" style="border-color: var(--color-danger); color: var(--color-danger)">{deleteError}</div>
		{/if}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('users.roles.delete_confirm', 'Are you sure you want to delete this role? This cannot be undone.')}
		</p>
		{#if deleteTarget}
			<p class="text-sm font-medium" style="color: var(--color-foreground)">{deleteTarget.name}</p>
		{/if}
		{#if deleteTarget && (deleteTarget.group_count ?? 0) > 0}
			<p class="rounded-md border px-3 py-2 text-xs" style="border-color: var(--color-warning); color: var(--color-warning)">
				{t('users.roles.delete_impact', `This role is attached to ${deleteTarget.group_count} group(s), affecting ${deleteTarget.user_count} user(s) — those groups lose its permissions immediately.`)}
			</p>
		{/if}
	</div>
	{#snippet footer()}
		<Button variant="outline" onclick={() => { showDeleteModal = false; }}>{t('common.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={confirmDeleteRole} disabled={deleting}>{deleting ? t('common.loading', 'Loading...') : t('common.delete', 'Delete')}</Button>
	{/snippet}
</Modal>
