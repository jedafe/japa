<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	// /products/new — dedicated product create page (§80). Gated by
	// products.create; submits POST /admin/products.
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import ProductForm from '$lib/components/products/product-form.svelte';
	import { goto } from '$app/navigation';
	import { t } from '$lib/stores/i18n.svelte';
</script>

<svelte:head><title>{t('products.new_product', 'New product')} — {siteName()}</title></svelte:head>

<PageHeader
	title={t('products.new_product', 'New product')}
	description={t('products.new_description', 'Create a catalog product with its type fields, providers and deals.')}
/>

<div class="mx-auto max-w-3xl p-4">
	<ProductForm
		mode="create"
		onsaved={(slug: string) => goto(`/products/${slug}`)}
		oncancel={() => goto('/products')}
	/>
</div>
