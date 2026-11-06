<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	// /products — product management list (§80 Product Domain). DataTable via
	// the EntityPage scaffold; create lives on the dedicated /products/new page.
	import { EntityPage, type EntityPageConfig } from '$lib/components/entity';
	import { productsAdminApi, productTypesAdminApi } from '$lib/api';
	import { goto } from '$app/navigation';
	import { t } from '$lib/stores/i18n.svelte';
	import { onMount } from 'svelte';
	import { Pencil, Plus } from 'lucide-svelte';

	type Row = {
		id: string;
		slug: string;
		name: string;
		type_slug: string;
		type_label: string;
		pricing_model: string | null;
		price_from: number | null;
		rating_avg: number | null;
		review_count: number;
		provider_count: number;
		deal_count: number;
		deleted_at: string | null;
		created_at: string;
	};

	let typeOptions = $state<Array<{ value: string; label: string }>>([]);

	onMount(async () => {
		const res = await productTypesAdminApi.list();
		const d = res.ok ? (res.data as any) : null;
		const items = d?.items ?? [];
		if (res.ok) {
			typeOptions = items.map((tp: any) => ({ value: tp.slug, label: tp.label }));
		}
	});

	const config = $derived<EntityPageConfig>({
		title: t('products.title', 'Products'),
		description: t('products.description', 'Manage the product catalog, providers and deals.'),
		hideCreate: true,
		headerActions: [
			{
				label: t('products.new_product', 'New product'),
				icon: Plus,
				onclick: () => goto('/products/new')
			}
		],
		createPermission: 'products.create',
		editPermission: 'products.update',
		deletePermission: 'products.delete',
		columns: [
			{ key: 'name', label: t('products.col_name', 'Name'), sortable: true, sortKey: 'name' },
			{ key: 'type_label', label: t('products.col_type', 'Type'), sortable: false },
			{
				key: 'price_from',
				label: t('products.col_price', 'From'),
				sortable: false,
				render: (v: any) => (v == null ? '—' : String(v))
			},
			{
				key: 'rating_avg',
				label: t('products.col_rating', 'Rating'),
				sortable: false,
				render: (v: any, row: Row) =>
					v == null ? '—' : `${v} (${row.review_count ?? 0})`
			},
			{ key: 'provider_count', label: t('products.col_providers', 'Providers'), sortable: false },
			{ key: 'deal_count', label: t('products.col_deals', 'Deals'), sortable: false },
			{
				key: 'created_at',
				label: t('products.col_created', 'Created'),
				sortable: true,
				sortKey: 'created_at',
				render: (v: any) => new Date(v).toLocaleDateString()
			}
		],
		filters: [
			{
				key: 'type',
				label: t('products.filter_type', 'Type'),
				type: 'select',
				options: typeOptions
			},
			{
				key: 'status',
				label: t('products.filter_status', 'Status'),
				type: 'select',
				options: [
					{ value: 'active', label: t('common.active', 'Active') },
					{ value: 'deleted', label: t('common.deleted', 'Deleted') },
					{ value: 'all', label: t('common.all', 'All') }
				]
			}
		],
		fields: [
			{ key: 'name', label: t('products.fld_name', 'Name'), type: 'text', required: true },
			{ key: 'slug', label: t('products.fld_slug', 'Slug'), type: 'text', required: true }
		],
		load: async ({ page, pageSize, q, sort, filter }) => {
			const params = new URLSearchParams({ page: String(page), limit: String(pageSize) });
			if (q) params.set('q', q);
			if (sort) {
				const [k, d] = sort.split(':');
				params.set('sort', k);
				params.set('dir', d === 'desc' ? 'desc' : 'asc');
			}
			if (filter?.type) params.set('type', filter.type);
			if (filter?.status) params.set('status', filter.status);
			const res = await productsAdminApi.list(params.toString());
			const body = res.ok ? (res.data as any) : null;
			// Products are slug-addressed everywhere (delete/restore/edit) —
			// the row id doubles as the slug for EntityPage's id-based calls.
			const rows = (body?.data ?? []).map((r: any) => ({ ...r, id: r.slug }));
			const total = body?.total ?? rows.length;
			return { data: rows, total, totalPages: Math.max(1, Math.ceil(total / pageSize)) };
		},
		remove: async (id: string) => {
			const res = await productsAdminApi.delete(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		restore: async (id: string) => {
			const res = await productsAdminApi.restore(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		hardRemove: async (id: string) => {
			const res = await productsAdminApi.delete(id, true);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		isDeleted: (row: Row) => row.deleted_at != null,
		actions: [
			{
				label: t('common.edit', 'Edit'),
				icon: Pencil,
				onclick: (row: Row) => goto(`/products/${row.slug}`)
			}
		],
		exportable: true,
		exportFilename: 'products'
	});
</script>

<svelte:head><title>{t('products.title', 'Products')} — {siteName()}</title></svelte:head>

<EntityPage {config} />
