<script lang="ts">
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { spaceTypesApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, Pencil, Trash2 } from 'lucide-svelte';
	import type { ActionItem } from '$lib/components/ui/data-table';
	import { onMount } from 'svelte';

	type SpaceType = {
		id: string;
		name: string;
		slug: string;
		description: string;
		category?: string;
		icon: string;
		is_system: boolean;
	};

	type SpaceCategory = {
		value: string;
		selectable: boolean;
	};

	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let bulkDeleting = $state(false);
	let categories = $state<SpaceCategory[]>([]);

	const canEdit = $derived(hasPermission('spaces.types.edit'));
	const canDelete = $derived(hasPermission('spaces.types.delete'));

	onMount(async () => {
		const res = await spaceTypesApi.categories();
		if (res.ok) {
			categories = (res.data ?? []).map((c) => ({ value: c.value, selectable: c.selectable }));
		}
	});

	function categoryLabel(value: string): string {
		const key = value.toLowerCase();
		return t(`space-types.category.${key}`, key.charAt(0).toUpperCase() + key.slice(1));
	}

	const displayCategoryOptions = $derived(
		categories.map((c) => ({ value: c.value, label: categoryLabel(c.value) }))
	);

	const formCategoryOptions = $derived(
		categories.filter((c) => c.selectable).map((c) => ({ value: c.value, label: categoryLabel(c.value) }))
	);

	async function bulkDelete(ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0) return;
		if (bulkDeleting) return;
		bulkDeleting = true;
		pageError = '';
		try {
			await Promise.all(idArr.map((id) => spaceTypesApi.delete(id)));
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('app.space_types.failed_to_delete_selected', 'Failed to delete selected items');
		} finally {
			bulkDeleting = false;
		}
	}

	function slugify(text: string): string {
		return text
			.toLowerCase()
			.replace(/[^a-z0-9]+/g, '-')
			.replace(/^-|-$/g, '');
	}

	const fields: FieldDef[] = $derived([
		{ key: 'name', label: t('space-types.name', 'Name'), type: 'text', placeholder: 'University', required: true },
		{ key: 'slug', label: t('space-types.slug', 'Slug'), type: 'text', placeholder: 'university', help: t('app.space_types.slug_hint', 'Leave blank to auto-generate from name.'), readonlyInEdit: true },
		{ key: 'description', label: t('space-types.description', 'Description'), type: 'textarea', placeholder: '' },
		{ key: 'icon', label: t('space-types.icon_emoji_or_name', 'Icon (emoji or name)'), type: 'text', placeholder: '🎓' },
		{ key: 'category', label: t('space-types.category', 'Category'), type: 'select', options: formCategoryOptions, required: true }
	]);

	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row, ctx) => ctx?.openView(_row) }
		];
		if (canEdit) {
			acts.push({
				label: t('common.edit', 'Edit'),
				icon: Pencil,
				onclick: (row, ctx) => ctx?.openEdit(row),
				show: (row: SpaceType) => !row.is_system
			});
		}
		if (canDelete) {
			acts.push({
				label: t('common.delete', 'Delete'),
				icon: Trash2,
				variant: 'destructive',
				onclick: (row, ctx) => ctx?.openDelete(row),
				show: (row: SpaceType) => !row.is_system
			});
		}
		return acts;
	});

	const config = $derived<EntityPageConfig>({
		title: t('app.space_types.page_title', 'Space Types'),
		description: t('space-types.manage_space_type_categories_including_u', 'Manage space type categories including university types.'),
		createLabel: t('app.space_types.add_space_type', 'Add Space Type'),
		createPermission: 'spaces.types.create',
		pageSize: 25,
		columns: [
			{ key: 'name', label: t('space-types.name', 'Name'), sortable: true },
			{
				key: 'slug',
				label: t('space-types.slug', 'Slug'),
				render: (v: string) => `<span class="font-mono text-xs" style="color: var(--color-muted)">${v}</span>`
			},
			{
				key: 'description',
				label: t('space-types.description', 'Description'),
				render: (v: string) => (v ? `<span class="text-sm">${v}</span>` : '—')
			},
			{ key: 'category', label: t('space-types.category', 'Category'), componentSnippet: CategoryCell },
			{ key: 'icon', label: t('space-types.icon', 'Icon'), componentSnippet: IconCell }
		],
		filters: [
			{
				key: 'category',
				label: t('space-types.category', 'Category'),
				options: displayCategoryOptions
			}
		],
		actions: rowActions,
		bulkActions: canDelete
			? [
					{
						label: t('app.space_types.delete_selected', 'Delete Selected'),
						icon: Trash2,
						variant: 'destructive',
						onclick: bulkDelete
					}
				]
			: [],
		fields,
		load: async ({ page, pageSize, q, filter }) => {
			const result = await spaceTypesApi.list('?page_size=1000');
			if (result.ok) {
				const d = result.data as SpaceType[] | { items: SpaceType[]; total_pages: number };
				let data = Array.isArray(d) ? d : (d.items ?? []);
				if (filter?.category) {
					data = data.filter((row) => row.category === filter.category);
				}
				const query = (q ?? '').toLowerCase();
				if (query) {
					data = data.filter((row) =>
						[row.name, row.slug, row.description, row.category, row.icon].some((v) =>
							String(v ?? '').toLowerCase().includes(query)
						)
					);
				}
				const size = pageSize || 25;
				const offset = (page || 0) * size;
				const total = data.length;
				const totalPages = Math.max(1, Math.ceil(total / size));
				return {
					data: data.slice(offset, offset + size),
					total,
					totalPages
				};
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load space types'));
		},
		create: async (body) => {
			const slug = String(body.slug ?? '').trim() || slugify(String(body.name ?? ''));
			const res = await spaceTypesApi.create({
				name: String(body.name ?? '').trim(),
				slug,
				description: String(body.description ?? '').trim(),
				category: String(body.category ?? '').trim(),
				icon: String(body.icon ?? '').trim()
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const slug = String(body.slug ?? '').trim() || slugify(String(body.name ?? ''));
			const res = await spaceTypesApi.update(id, {
				name: String(body.name ?? '').trim(),
				slug,
				description: String(body.description ?? '').trim(),
				category: String(body.category ?? '').trim(),
				icon: String(body.icon ?? '').trim()
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await spaceTypesApi.delete(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: SpaceTypeView
	});
</script>

<svelte:head>
	<title>{t('app.space_types.page_title', 'Space Types')} — JapaConnect Admin</title>
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

<!-- Snippets -->

{#snippet IconCell({ value }: { value: any; row: SpaceType })}
	<span class="text-lg">{value || '—'}</span>
{/snippet}

{#snippet CategoryCell({ value }: { value: string; row: SpaceType })}
	{#if value}
		<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium bg-info/15 text-info">
			{displayCategoryOptions.find((o) => o.value === value)?.label ?? value}
		</span>
	{:else}
		<span class="text-muted">—</span>
	{/if}
{/snippet}

{#snippet SpaceTypeView({ row }: { row: SpaceType })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.space_types.name_label', 'Name')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.name}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.space_types.slug_label', 'Slug')}</p>
			<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.slug}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.space_types.description_label', 'Description')}</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.description || '—'}</p>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.space_types.category_label', 'Category')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
					{row.category ? (displayCategoryOptions.find((o) => o.value === row.category)?.label ?? row.category) : '—'}
				</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.space_types.icon_label', 'Icon')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.icon || '—'}</p>
			</div>
		</div>
	</div>
{/snippet}
