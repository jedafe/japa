<script lang="ts">
	// /custom-fields/entity-types — migrated to the EntityPage scaffold.
	// Simple CRUD entity using declarative `fields` (EntityForm). System entity
	// types are protected from edit/delete via `show()` predicates; bulk delete
	// is disabled when any selected row is a system type.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { entityTypesAdminApi, type CustomFieldEntityType } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Eye, Pencil, Trash2 } from 'lucide-svelte';
	import type { ActionItem } from '$lib/components/ui/data-table';

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
			await Promise.all(idArr.map((id) => entityTypesAdminApi.delete(id)));
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'Failed to delete entity types');
		} finally {
			bulkDeleting = false;
		}
	}

	// `fields` drives EntityForm rendering and edit-form seeding. `value` is
	// immutable after creation (legacy page only sent it on create), so it is
	// included here for edit-form seeding but the API update omits it.
	const fields: FieldDef[] = [
		{
			key: 'value',
			label: t('fieldGroups.option_value', 'Value'),
			type: 'text',
			placeholder: t('fieldGroups.value_placeholder', 'e.g. event, project, course'),
			required: true,
			help: t('fieldGroups.value_help', 'Machine-readable token (snake_case). Cannot be changed after creation.')
		},
		{
			key: 'label',
			label: t('fieldGroups.label', 'Label'),
			type: 'text',
			placeholder: t('fieldGroups.label_placeholder_et', 'e.g. Event, Project, Course'),
			required: true
		},
		{ key: 'sort_order', label: t('fieldGroups.sort_order', 'Sort Order'), type: 'number' }
	];

	// Custom row actions: View/Edit/Delete with system-type protection.
	// Providing `actions` replaces the defaults entirely, so View/Edit/Delete
	// are included explicitly. Edit/Delete are hidden for system rows.
	const actions: ActionItem[] = [
		{ label: t('common.view', 'View'), icon: Eye, onclick: (_row, ctx) => ctx?.openView(_row) },
		{
			label: t('common.edit', 'Edit'),
			icon: Pencil,
			onclick: (row, ctx) => ctx?.openEdit(row),
			show: (row: CustomFieldEntityType) => !row.is_system
		},
		{
			label: t('common.delete', 'Delete'),
			icon: Trash2,
			variant: 'destructive',
			onclick: (row, ctx) => ctx?.openDelete(row),
			show: (row: CustomFieldEntityType) => !row.is_system
		}
	];

	const config: EntityPageConfig = {
		title: t('fieldGroups.entity_types', 'Entity Types'),
		description: t(
			'fieldGroups.entity_types_desc',
			'Manage the entity types that custom field groups can be assigned to.'
		),
		createLabel: t('common.create', 'Create'),
		createPermission: 'custom_fields.group.create',
		pageSize: 25,
		columns: [
			{
				key: 'value',
				label: t('fieldGroups.option_value', 'Value'),
				sortable: true,
				render: (v: string) =>
					`<span class="font-mono text-xs" style="color: var(--color-muted)">${v}</span>`
			},
			{ key: 'label', label: t('fieldGroups.label', 'Label'), sortable: true },
			{ key: 'sort_order', label: t('fieldGroups.sort_order', 'Sort Order'), sortable: true },
			{
				key: 'is_system',
				label: t('fieldGroups.system', 'System'),
				sortable: true,
				componentSnippet: SystemCell
			}
		],
		actions,
		bulkActions: [
			{
				label: t('common.deleteSelected', 'Delete Selected'),
				icon: Trash2,
				variant: 'destructive',
				onclick: bulkDelete,
				disabled: (_ids: Set<string>, selectedRows?: CustomFieldEntityType[]) =>
					!!selectedRows?.some((r) => r.is_system)
			}
		],
		fields,
		load: async () => {
			const result = await entityTypesAdminApi.list();
			if (result.ok) {
				const data = (result.data as CustomFieldEntityType[]) ?? [];
				return { data, total: data.length, totalPages: 1 };
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load entity types'));
		},
		create: async (body) => {
			const res = await entityTypesAdminApi.create({
				value: String(body.value ?? '')
					.trim()
					.toLowerCase()
					.replace(/\s+/g, '_'),
				label: String(body.label ?? '').trim(),
				sort_order: Number(body.sort_order ?? 0)
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			// `value` is immutable — update omits it (mirrors the legacy save()).
			const res = await entityTypesAdminApi.update(id, {
				label: String(body.label ?? '').trim(),
				sort_order: Number(body.sort_order ?? 0)
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await entityTypesAdminApi.delete(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: EntityTypeView
	};
</script>

<svelte:head>
	<title>{t('fieldGroups.entity_types', 'Entity Types')} — JapaConnect Admin</title>
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

{#snippet SystemCell({ value }: { value: boolean; row: CustomFieldEntityType })}
	{#if value}
		<span
			class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium"
			style="background-color: var(--color-primary-muted); color: var(--color-primary)"
		>
			{t('fieldGroups.system', 'System')}
		</span>
	{:else}
		<span class="text-xs" style="color: var(--color-muted)">—</span>
	{/if}
{/snippet}

<!-- View slide-over body (mirrors the legacy view) -->
{#snippet EntityTypeView({ row }: { row: CustomFieldEntityType })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('fieldGroups.option_value', 'Value')}
			</p>
			<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.value}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('fieldGroups.label', 'Label')}
			</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.label}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('fieldGroups.sort_order', 'Sort Order')}
			</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.sort_order}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('fieldGroups.system', 'System')}
			</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
				{row.is_system ? t('fieldGroups.system', 'System') : t('common.custom', 'Custom')}
			</p>
		</div>
	</div>
{/snippet}
