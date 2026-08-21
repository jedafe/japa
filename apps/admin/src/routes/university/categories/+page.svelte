<script lang="ts">
	// /university/categories — migrated to the EntityPage scaffold.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { universityAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Trash2 } from 'lucide-svelte';

	type Category = {
		id: string;
		slug: string;
		name: string;
		sort_order: number;
		is_active: boolean;
		created_at: string;
		updated_at: string;
	};

	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let bulkDeleting = $state(false);

	const canManage = $derived(true); // create/edit/delete gated by createPermission/editPermission/deletePermission

	async function bulkDelete(ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0) return;
		if (bulkDeleting) return;
		bulkDeleting = true;
		pageError = '';
		try {
			await Promise.all(idArr.map((id) => universityAdminApi.deleteCategory(id)));
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'Failed to delete categories');
		} finally {
			bulkDeleting = false;
		}
	}

	const fields: FieldDef[] = [
		{ key: 'name', label: t('university.category_name', 'Category Name'), type: 'text', required: true },
		{ key: 'slug', label: t('university.slug', 'Slug'), type: 'text', required: true },
		{ key: 'sort_order', label: t('university.sort_order', 'Sort Order'), type: 'number' },
		{ key: 'is_active', label: t('university.active', 'Active'), type: 'checkbox' }
	];

	const config = $derived<EntityPageConfig>({
		title: t('university.categories_title', 'University Track Categories'),
		description: t('university.categories_desc', 'Manage the canonical category list for university marketplace tracks'),
		createLabel: t('university.add_category', 'Add Category'),
		createPermission: 'university.category.manage',
		editPermission: 'university.category.manage',
		deletePermission: 'university.category.manage',
		pageSize: 25,
		columns: [
			{ key: 'name', label: t('university.category_name', 'Category'), render: (v: string) => `<span class="font-medium">${v}</span>` },
			{ key: 'slug', label: t('university.slug', 'Slug'), render: (v: string) => `<span class="font-mono text-xs" style="color: var(--color-muted)">${v}</span>` },
			{ key: 'sort_order', label: t('university.sort_order', 'Sort Order'), render: (v: number) => `<span class="text-xs">${v}</span>` },
			{
				key: 'is_active',
				label: t('university.status', 'Status'),
				render: (v: boolean) => v
					? `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium" style="background: color-mix(in srgb, var(--color-success) 15%, transparent); color: var(--color-success)">${t('university.active', 'Active')}</span>`
					: `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium" style="background: color-mix(in srgb, var(--color-muted) 15%, transparent); color: var(--color-muted)">${t('university.inactive', 'Inactive')}</span>`
			}
		],
		filters: [
			{
				key: 'active',
				label: t('university.status', 'Status'),
				options: [
					{ value: 'true', label: t('university.active', 'Active') },
					{ value: 'false', label: t('university.inactive', 'Inactive') }
				]
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
		load: async ({ page, pageSize, q, filter, sort }) => {
			const params = new URLSearchParams();
			params.set('page', String(page + 1));
			params.set('limit', String(pageSize));
			if (filter.active) params.set('active', filter.active);
			if (q) params.set('search', q);
			if (sort) {
				const [key, dir] = sort.split(':');
				if (key) {
					params.set('sort', key);
					params.set('dir', dir ?? 'asc');
				}
			}
			const result = await universityAdminApi.listCategories(params.toString());
			if (result.ok) {
				const data = result.data.data ?? [];
				const total = result.data.meta?.total ?? data.length;
				const totalPages = pageSize > 0 ? Math.max(1, Math.ceil(total / pageSize)) : 1;
				return { data, total, totalPages };
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load categories'));
		},
		create: async (body) => {
			const res = await universityAdminApi.createCategory({
				name: String(body.name ?? '').trim(),
				slug: String(body.slug ?? '').trim(),
				sort_order: Number(body.sort_order ?? 0),
				is_active: body.is_active === undefined ? true : !!body.is_active
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const res = await universityAdminApi.updateCategory(id, {
				name: String(body.name ?? '').trim() || undefined,
				slug: String(body.slug ?? '').trim() || undefined,
				sort_order: body.sort_order === undefined ? undefined : Number(body.sort_order),
				is_active: body.is_active === undefined ? undefined : !!body.is_active
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await universityAdminApi.deleteCategory(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: CategoryView
	});
</script>

<svelte:head>
	<title>{t('university.categories_title', 'University Track Categories')} — JapaConnect Admin</title>
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

{#snippet CategoryView({ row }: { row: Category })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('university.category_name', 'Category Name')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.name}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('university.slug', 'Slug')}</p>
			<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.slug}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('university.sort_order', 'Sort Order')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.sort_order}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('university.status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.is_active ? t('university.active', 'Active') : t('university.inactive', 'Inactive')}</p>
		</div>
	</div>
{/snippet}
