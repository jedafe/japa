<script lang="ts">
	// /guide-categories — migrated to the EntityPage scaffold.
	// Create + delete only (no update API); custom `actions` array omits Edit.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { guideCategoriesAdminApi, type GuideCategory } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Trash2, Eye } from 'lucide-svelte';

	// Normalize an API row: copy `Slug` into `id` so the scaffold's id-based remove + bulk selection work.
	function mapCategory(raw: any): GuideCategory & { id: string } {
		return { ...raw, id: raw.Slug };
	}

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
			await Promise.all(idArr.map((id) => guideCategoriesAdminApi.delete(id)));
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'Failed to delete guide categories');
		} finally {
			bulkDeleting = false;
		}
	}

	// `fields` drives EntityForm rendering (create only — no update API).
	const fields: FieldDef[] = [
		{
			key: 'slug',
			label: t('guide_categories.slug', 'Slug'),
			type: 'text',
			placeholder: 'visa',
			required: true,
			help: t('guide_categories.slug_hint', 'Machine-readable token (snake_case). Cannot be changed after creation.')
		},
		{
			key: 'label',
			label: t('guide_categories.label', 'Label'),
			type: 'text',
			placeholder: 'Visa Guide',
			required: true,
			help: t('guide_categories.label_hint', 'Human-readable name shown in the composer category dropdown.')
		}
	];

	const config: EntityPageConfig = {
		title: t('guide_categories.title', 'Guide Categories'),
		description: t(
			'guide_categories.description',
			'Manage evergreen guide categories for the Guide content type. Each category has its own custom field group.'
		),
		createLabel: t('guide_categories.create', 'Create'),
		createPermission: 'custom_fields.group.create',
		deletePermission: 'custom_fields.group.delete',
		pageSize: 25,
		columns: [
			{
				key: 'Slug',
				label: t('guide_categories.slug', 'Slug'),
				sortable: true,
				render: (v: string) => `<span class="font-mono text-xs" style="color: var(--color-muted)">${v}</span>`
			},
			{ key: 'Label', label: t('guide_categories.label', 'Label'), sortable: true }
		],
		// Custom actions: View + Delete only (no Edit — there is no update API).
		actions: [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row, ctx) => ctx?.openView(_row) },
			{
				label: t('common.delete', 'Delete'),
				icon: Trash2,
				variant: 'destructive',
				onclick: (_row, ctx) => ctx?.openDelete(_row),
				show: (row) => !row.is_system
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
		load: async () => {
			const result = await guideCategoriesAdminApi.list();
			if (result.ok) {
				const data = (result.data as GuideCategory[]) ?? [];
				const mapped = data.map(mapCategory);
				return { data: mapped, total: mapped.length, totalPages: 1 };
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load guide categories'));
		},
		create: async (body) => {
			const res = await guideCategoriesAdminApi.create({
				slug: String(body.slug ?? '').trim().toLowerCase().replace(/\s+/g, '_'),
				label: String(body.label ?? '').trim()
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await guideCategoriesAdminApi.delete(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: CategoryView
	};
</script>

<svelte:head>
	<title>{t('guide_categories.title', 'Guide Categories')} — JapaConnect Admin</title>
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

<!-- View slide-over body -->
{#snippet CategoryView({ row }: { row: GuideCategory & { id: string } })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('guide_categories.slug', 'Slug')}
			</p>
			<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.Slug}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('guide_categories.label', 'Label')}
			</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.Label}</p>
		</div>
	</div>
{/snippet}
