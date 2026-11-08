<script lang="ts">
	import { siteName } from '#lib/stores/site.svelte.js';
	// /products/[slug] — product edit page (§80). Loads the admin detail
	// payload (core + providers + deals + cfv) into the shared ProductForm.
	import PageHeader from '#lib/components/layout/page-header.svelte';
	import ProductForm from '#lib/components/products/product-form.svelte';
	import { productsAdminApi } from '#lib/api.js';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { t } from '#lib/stores/i18n.svelte.js';
	import { onMount } from 'svelte';
	import { Loader2, RotateCcw, Trash2 } from 'lucide-svelte';

	const slug = $derived(page.params.slug ?? '');

	let detail = $state<any>(null);
	let loading = $state(true);
	let loadErr = $state('');
	let actionErr = $state('');
	let acting = $state(false);

	onMount(async () => {
		const res = await productsAdminApi.get(slug);
		loading = false;
		if (!res.ok) {
			loadErr = res.error ?? t('products.err_load', 'Failed to load product');
			return;
		}
		detail = res.data;
	});

	const isDeleted = $derived(detail?.product?.deleted_at != null);

	async function doDelete(force = false) {
		actionErr = '';
		acting = true;
		const res = await productsAdminApi.delete(slug, force);
		acting = false;
		if (!res.ok) { actionErr = res.error ?? t('products.err_save', 'Action failed'); return; }
		goto('/products');
	}
	async function doRestore() {
		actionErr = '';
		acting = true;
		const res = await productsAdminApi.restore(slug);
		acting = false;
		if (!res.ok) { actionErr = res.error ?? t('products.err_save', 'Action failed'); return; }
		const d = await productsAdminApi.get(slug);
		if (d.ok) detail = d.data;
	}
</script>

<svelte:head><title>{detail?.product?.name ?? slug} — {siteName()}</title></svelte:head>

<PageHeader
	title={detail?.product?.name ?? t('products.edit_product', 'Edit product')}
	description={detail?.product?.type_label ?? ''}
	actions={[
		...(isDeleted
			? [{
				label: t('common.restore', 'Restore'),
				icon: RotateCcw,
				variant: 'secondary' as const,
				onclick: doRestore
			}]
			: [{
				label: t('common.delete', 'Delete'),
				icon: Trash2,
				variant: 'destructive' as const,
				onclick: () => doDelete()
			}])
	]}
/>

<div class="mx-auto max-w-3xl p-4">
	{#if loading}
		<div class="flex items-center gap-2 py-8 text-sm text-[var(--color-text-subtle)]">
			<Loader2 size={16} class="animate-spin" /> {t('common.loading', 'Loading')}
		</div>
	{:else if loadErr}
		<p class="py-8 text-sm text-red-500" role="alert">{loadErr}</p>
	{:else if detail}
		{#if isDeleted}
			<div class="mb-4 rounded-[var(--radius)] border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-sm text-amber-600">
				{t('products.deleted_banner', 'This product is deleted. Restore it to make it visible again.')}
			</div>
		{/if}
		{#if actionErr}
			<p class="mb-4 text-sm text-red-500" role="alert">{actionErr}</p>
		{/if}
		<ProductForm
			mode="edit"
			initial={detail}
			onsaved={() => goto('/products')}
			oncancel={() => goto('/products')}
		/>
	{/if}
</div>
