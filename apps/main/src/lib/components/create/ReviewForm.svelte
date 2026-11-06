<script lang="ts">
	// §14 review content type — product picker + rating + providers repeater.
	// field_values carries {product_slug, product_type_slug, rating, providers[]}
	// through CreateShell → POST /content; the providers repeater auto-populates
	// from GET /wiki/e/{type}/{slug}/providers on product select.
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import { t } from '$lib/i18n';
	import PostEditor, { type EditorMode } from './editors/PostEditor.svelte';

	let { initial = null, defaultEditor = 'markdown' as EditorMode, onvalues = (fv: Record<string, unknown>) => {}, onhtml = (v: string) => {}, onedit = () => {} } = $props();

	interface ProductType { slug: string; label: string }
	interface Product { slug: string; name: string }
	interface Provider { provider_id: string; name: string; price_from?: string; offer_url?: string }

	let types = $state<ProductType[]>([]);
	let typeSlug = $state(initial?.product_type_slug ?? '');
	let products = $state<Product[]>([]);
	let productSlug = $state(initial?.product_slug ?? '');
	let rating = $state<number>(initial?.rating ?? 0);
	let providers = $state<Provider[]>(initial?.providers ?? []);
	let picked = $state<Set<string>>(new Set((initial?.providers ?? []).map((p: Provider) => p.provider_id)));
	let bodyHtml = $state('');

	onMount(async () => {
		const res = await api.get<{ kind: string; types?: { slug: string; label: string }[] }[]>('/wiki/entity-kinds');
		types = res?.data?.find((k: { kind: string }) => k.kind === 'product')?.types ?? [];
	});

	async function pickType() {
		productSlug = '';
		products = [];
		providers = [];
		emit();
		if (!typeSlug) return;
		const res = await api.get<{ items?: Product[] }>(`/wiki/entity-kinds/${typeSlug}/entities?limit=100`);
		products = res?.data?.items ?? [];
		emit();
	}

	async function pickProduct() {
		providers = [];
		picked = new Set();
		emit();
		if (!productSlug || !typeSlug) return;
		const res = await api.get<{ items?: Provider[] }>(`/wiki/e/${typeSlug}/${productSlug}/providers`);
		providers = res?.data?.items ?? [];
		// Default-check every provider — reviewer prunes to what they evaluated.
		picked = new Set(providers.map((p) => p.provider_id));
		emit();
	}

	function toggleProvider(id: string) {
		const next = new Set(picked);
		next.has(id) ? next.delete(id) : next.add(id);
		picked = next;
		onedit();
		emit();
	}

	function emit() {
		onvalues({
			product_type_slug: typeSlug,
			product_slug: productSlug,
			rating,
			providers: providers.filter((p) => picked.has(p.provider_id)),
		});
	}
</script>

<div class="review-form">
	<label class="fld">
		<span>{t('review.product_type', 'Product type')}</span>
		<select bind:value={typeSlug} onchange={() => { onedit(); pickType(); }}>
			<option value="">—</option>
			{#each types as ty (ty.slug)}<option value={ty.slug}>{ty.label}</option>{/each}
		</select>
	</label>

	<label class="fld">
		<span>{t('review.product', 'Product')}</span>
		<select bind:value={productSlug} onchange={() => { onedit(); pickProduct(); }} disabled={!typeSlug}>
			<option value="">—</option>
			{#each products as p (p.slug)}<option value={p.slug}>{p.name}</option>{/each}
		</select>
	</label>

	<label class="fld">
		<span>{t('review.rating', 'Rating')}</span>
		<div class="stars">
			{#each [1, 2, 3, 4, 5] as n}
				<button type="button" class="star" class:on={rating >= n} onclick={() => { rating = n; onedit(); emit(); }}>★</button>
			{/each}
		</div>
	</label>

	{#if providers.length}
		<fieldset class="fld">
			<legend>{t('review.providers', 'Providers')}</legend>
			{#each providers as p (p.provider_id)}
				<label class="prov">
					<input type="checkbox" checked={picked.has(p.provider_id)} onchange={() => toggleProvider(p.provider_id)} />
					{p.name}
					{#if p.price_from}<span class="price">${p.price_from}</span>{/if}
				</label>
			{/each}
		</fieldset>
	{/if}

	<PostEditor html={bodyHtml} onedit={() => { onedit(); onhtml(bodyHtml); }} onhtml={(v: string) => { bodyHtml = v; onhtml(v); }} />
</div>

<style>
	.review-form { display: flex; flex-direction: column; gap: 0.75rem; }
	.fld { display: flex; flex-direction: column; gap: 0.25rem; font-size: 0.875rem; }
	.fld > span, legend { font-weight: 600; }
	select { padding: 0.5rem; border: 1px solid var(--color-border, #e2e8f0); border-radius: 0.5rem; }
	.stars { display: flex; gap: 0.125rem; font-size: 1.5rem; }
	.star { background: none; border: none; cursor: pointer; color: var(--color-muted-foreground, #94a3b8); padding: 0; }
	.star.on { color: #f59e0b; }
	.prov { display: flex; align-items: center; gap: 0.5rem; padding: 0.25rem 0; }
	.price { color: var(--color-muted-foreground, #64748b); }
</style>
