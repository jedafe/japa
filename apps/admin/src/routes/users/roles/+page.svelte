<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	// /users/roles — migrated to the EntityPage scaffold.
	//
	// The role form has a searchable permissions checkbox grid that EntityForm
	// cannot express, so it is rendered via the `formSnippet` escape hatch.
	// The full permissions list is fetched on mount (mirrors the legacy load).
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { userManagementApi, permissionsApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Trash2 } from 'lucide-svelte';

	type Role = {
		id: string;
		name: string;
		permissions: string[];
		user_count: number;
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
		description: t('users.roles.description', 'Manage roles and their permission sets'),
		createLabel: t('users.roles.create', 'Create Role'),
		createPermission: 'admin.rbac.manage',
		pageSize: 25,
		columns: [
			{ key: 'name', label: t('users.roles.name', 'Name'), sortable: true },
			{
				key: 'permissions',
				label: t('users.roles.permissions', 'Permissions'),
				render: (_: any, row: Role) => {
					const chips = row.permissions.slice(0, 4).map((p: string) => `<span class="inline-block rounded-full px-2 py-0.5 text-xs" style="background-color: color-mix(in srgb, var(--color-secondary) 15%, transparent); color: var(--color-foreground)">${p}</span>`).join('');
					const extra = row.permissions.length > 4 ? `<span class="inline-block rounded-full px-2 py-0.5 text-xs" style="background-color: color-mix(in srgb, var(--color-secondary) 15%, transparent); color: var(--color-muted)">+${row.permissions.length - 4}</span>` : '';
					return `<div class="flex flex-wrap gap-1">${chips}${extra}</div>`;
				}
			},
			{
				key: 'user_count',
				label: t('users.roles.user_count', 'Users'),
				sortable: true,
				render: (v: number) => `<span class="rounded-full px-2 py-0.5 text-xs font-bold" style="background-color: color-mix(in srgb, var(--color-primary) 10%, transparent); color: var(--color-primary)">${v}</span>`
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

<!-- View slide-over body -->
{#snippet RoleView({ row }: { row: Role })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('users.roles.name', 'Name')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.name}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('users.roles.user_count', 'Users')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.user_count}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('users.roles.permissions', 'Permissions')}</p>
			{#if row.permissions.length === 0}
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
			<div class="grid max-h-60 grid-cols-2 gap-2 overflow-y-auto">
				{#each filteredPermissions as perm (perm)}
					<label class="flex items-center gap-2 text-xs">
						<input
							type="checkbox"
							class="rounded"
							checked={Array.isArray(value.permissions) && value.permissions.includes(perm)}
							onchange={(e) => {
								const current = Array.isArray(value.permissions) ? value.permissions : [];
								value.permissions = e.currentTarget.checked
									? [...current, perm]
									: current.filter((p) => p !== perm);
							}}
						/>
						{perm}
					</label>
				{/each}
			</div>
		</div>
	</div>
{/snippet}
