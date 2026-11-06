<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	// /product-types — product type management (§80). Each type carries an
	// associated custom field group set; edit lives on /product-types/{slug}.
	import { EntityPage, type EntityPageConfig } from '$lib/components/entity';
	import { productTypesAdminApi } from '$lib/api';
	import { goto } from '$app/navigation';
	import { t } from '$lib/stores/i18n.svelte';
	import { Pencil, Plus } from 'lucide-svelte';

	type Row = {
		id: string;
		slug: string;
		label: string;
		icon: string | null;
		is_enabled: boolean;
		product_count: number;
		created_at: string;
	};

	const config = $derived<EntityPageConfig>({
		title: t('product_types.title', 'Product Types'),
		description: t('product_types.description', 'Product types drive the catalog structure — each type binds a custom field group per category.'),
		hideCreate: true,
		headerActions: [
			{
				label: t('product_types.new', 'New type'),
				icon: Plus,
				onclick: () => goto('/product-types/new')
			}
		],
		createPermission: 'product_types.create',
		editPermission: 'product_types.update',
		deletePermission: 'product_types.delete',
		columns: [
			{ key: 'slug', label: t('product_types.col_slug', 'Slug'), sortable: true },
			{ key: 'label', label: t('product_types.col_label', 'Label'), sortable: true },
			{ key: 'icon', label: t('product_types.col_icon', 'Icon'), sortable: false, render: (v: any) => v ?? '—' },
			{
				key: 'is_enabled',
				label: t('product_types.col_enabled', 'Enabled'),
				sortable: false,
				render: (v: any) => (v ? t('common.yes', 'Yes') : t('common.no', 'No'))
			},
			{ key: 'product_count', label: t('product_types.col_products', 'Products'), sortable: false },
			{
				key: 'created_at',
				label: t('product_types.col_created', 'Created'),
				sortable: false,
				render: (v: any) => new Date(v).toLocaleDateString()
			}
		],
		fields: [
			{ key: 'slug', label: t('products.fld_slug', 'Slug'), type: 'text', required: true },
			{ key: 'label', label: t('products.fld_label', 'Label'), type: 'text', required: true },
			{ key: 'icon', label: t('products.fld_icon', 'Icon'), type: 'text' }
		],
		load: async ({ page, pageSize, q }) => {
			const params = new URLSearchParams({ page: String(page), limit: String(pageSize) });
			if (q) params.set('q', q);
			const res = await productTypesAdminApi.list(params.toString());
			const d = res.ok ? (res.data as any) : null;
			const rows = (d?.data ?? []).map((r: any) => ({ ...r, id: r.slug }));
			const total = d?.total ?? rows.length;
			return { data: rows, total, totalPages: Math.max(1, Math.ceil(total / pageSize)) };
		},
		remove: async (id: string) => {
			const res = await productTypesAdminApi.delete(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		actions: [
			{
				label: t('common.edit', 'Edit'),
				icon: Pencil,
				onclick: (row: Row) => goto(`/product-types/${row.slug}`)
			}
		]
	});
</script>

<svelte:head><title>{t('product_types.title', 'Product Types')} — {siteName()}</title></svelte:head>

<EntityPage {config} />
