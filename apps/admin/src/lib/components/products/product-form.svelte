<script lang="ts">
	// ProductForm — shared create/edit body for /products/new + /products/[slug]
	// (§80). Type picker drives the custom-field schema; providers repeater and
	// deal multi-select write product_providers + product_deals in the save tx.
	import { api, productsAdminApi, productTypesAdminApi, affiliateProvidersAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { onMount } from 'svelte';
	import { Loader2, Plus, Trash2 } from 'lucide-svelte';

	type Provider = { id: string; name: string; slug: string };
	type Deal = { id: string; title: string; provider_name?: string | null; provider_id?: string | null };
	type FieldDef = {
		id: string; key: string; label: string; field_type: string;
		options?: any; placeholder?: string | null; is_required: boolean; section?: string | null;
	};
	type SchemaGroup = { id: string; slug: string; name: string; fields: FieldDef[] };

	let {
		mode = 'create',
		initial = null,
		onsaved,
		oncancel
	}: {
		mode: 'create' | 'edit';
		initial?: any;
		onsaved?: (slug: string) => void;
		oncancel?: () => void;
	} = $props();

	const inp =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]';
	const lbl = 'mb-1 block text-xs font-medium text-[var(--color-text-subtle)]';

	let types = $state<Array<{ slug: string; label: string }>>([]);
	let providersAll = $state<Provider[]>([]);
	let dealsAll = $state<Deal[]>([]);

	let typeSlug = $state(initial?.product?.type_slug ?? '');
	let name = $state(initial?.product?.name ?? '');
	let slug = $state(initial?.product?.slug ?? '');
	let slugTouched = $state(mode === 'edit');
	let tagline = $state(initial?.product?.tagline ?? '');
	let websiteUrl = $state(initial?.product?.website_url ?? '');
	let pricingModel = $state(initial?.product?.pricing_model ?? '');
	let priceFrom = $state(initial?.product?.price_from ?? '');

	type ProviderRow = { provider_id: string; price_from: string; offer_url: string; position: number };
	let providerRows = $state<ProviderRow[]>(
		(initial?.providers ?? []).map((p: any, i: number) => ({
			provider_id: p.provider_id,
			price_from: p.price_from ?? '',
			offer_url: p.offer_url ?? '',
			position: i
		}))
	);

	let dealIds = $state<string[]>((initial?.deals ?? []).map((d: any) => d.coupon_id));

	let schema = $state<SchemaGroup[]>([]);
	let schemaLoading = $state(false);
	let fieldValues = $state<Record<string, any>>({});
	// Seed cfv from the edit payload — keyed by field_id.
	$effect(() => {
		if (!initial?.field_values) return;
		const seeded: Record<string, any> = {};
		for (const fv of initial.field_values) {
			let v = fv.value_text ?? fv.value_number ?? fv.value_bool ?? fv.value_date ?? fv.value_json ?? '';
			// cfv dates are full ISO timestamps — date inputs need yyyy-MM-dd.
			if (typeof v === 'string' && fv.value_date) v = v.slice(0, 10);
			seeded[fv.field_id] = v;
		}
		fieldValues = seeded;
	});

	/** custom_fields.options is jsonb — may be a bare array or {choices:[...]}. */
	function fieldOptions(f: FieldDef): Array<{ value?: any; label?: string } | string> {
		const o = f.options;
		if (Array.isArray(o)) return o;
		if (o && Array.isArray(o.choices)) return o.choices;
		if (o && Array.isArray(o.options)) return o.options;
		return [];
	}

	let saving = $state(false);
	let saveErr = $state('');

	onMount(async () => {
		const [tRes, pRes] = await Promise.all([
			productTypesAdminApi.list(),
			affiliateProvidersAdminApi.list('page_size=100')
		]);
		if (tRes.ok) {
			const td = tRes.data as any;
			types = (td?.items ?? td ?? []).map((x: any) => ({ slug: x.slug, label: x.label }));
		}
		if (pRes.ok) providersAll = (pRes.data as any)?.items ?? (pRes.data as any) ?? [];
		// Deal options: auth-only endpoint (no coupons.manage needed).
		const dRes = await api('/products/deals');
		if (dRes.ok) {
			const dd = dRes.data as any;
			dealsAll = (dd?.items ?? dd ?? []) as Deal[];
		}
		if (typeSlug) await loadSchema(typeSlug);
	});

	async function loadSchema(ts: string) {
		if (!ts) {
			schema = [];
			return;
		}
		schemaLoading = true;
		const res = await productsAdminApi.fieldSchema(ts);
		schemaLoading = false;
		schema = res.ok ? (((res.data as any)?.groups) ?? []) : [];
	}

	function slugify(s: string): string {
		return s
			.toLowerCase()
			.trim()
			.replace(/[^a-z0-9]+/g, '-')
			.replace(/^-+|-+$/g, '');
	}

	function onNameInput() {
		if (!slugTouched) slug = slugify(name);
	}

	// Deal options scoped to the chosen providers when any are picked.
	const dealOptions = $derived.by(() => {
		const chosen = new Set(providerRows.map((r) => r.provider_id).filter(Boolean));
		if (chosen.size === 0) return dealsAll;
		const scoped = dealsAll.filter((d) => d.provider_id && chosen.has(d.provider_id));
		return scoped.length > 0 ? scoped : dealsAll;
	});

	function toggleDeal(id: string) {
		dealIds = dealIds.includes(id) ? dealIds.filter((x) => x !== id) : [...dealIds, id];
	}

	function addProvider() {
		providerRows = [...providerRows, { provider_id: '', price_from: '', offer_url: '', position: providerRows.length }];
	}
	function removeProvider(i: number) {
		providerRows = providerRows.filter((_, j) => j !== i);
	}

	function fieldValueType(f: FieldDef): 'text' | 'number' | 'bool' | 'date' | 'json' {
		switch (f.field_type) {
			case 'number': return 'number';
			case 'checkbox':
			case 'boolean': return 'bool';
			case 'date': return 'date';
			case 'json': return 'json';
			default: return 'text';
		}
	}

	async function submit() {
		saveErr = '';
		if (!typeSlug) { saveErr = t('products.err_type', 'Pick a product type'); return; }
		if (!name.trim()) { saveErr = t('products.err_name', 'Name is required'); return; }
		if (!slug.trim()) { saveErr = t('products.err_slug', 'Slug is required'); return; }
		saving = true;
		const custom_fields: any[] = [];
		for (const g of schema) {
			for (const f of g.fields) {
				const v = fieldValues[f.id];
				if (v === undefined || v === null || v === '') continue;
				const entry: any = { field_id: f.id };
				switch (fieldValueType(f)) {
					case 'number': entry.value_number = Number(v); break;
					case 'bool': entry.value_bool = !!v; break;
					case 'date': entry.value_date = new Date(v).toISOString(); break;
					case 'json': entry.value_json = typeof v === 'string' ? JSON.parse(v || 'null') : v; break;
					default: entry.value_text = String(v);
				}
				custom_fields.push(entry);
			}
		}
		const body: Record<string, unknown> = {
			type_slug: typeSlug,
			name: name.trim(),
			slug: slug.trim(),
			tagline: tagline || null,
			website_url: websiteUrl || null,
			pricing_model: pricingModel || null,
			price_from: priceFrom !== '' ? Number(priceFrom) : null,
			providers: providerRows.filter((r) => r.provider_id).map((r, i) => ({
				provider_id: r.provider_id,
				price_from: r.price_from,
				offer_url: r.offer_url,
				position: i
			})),
			deal_ids: dealIds,
			custom_fields
		};
		const res =
			mode === 'create'
				? await productsAdminApi.create(body)
				: await productsAdminApi.update(slug, body);
		saving = false;
		if (!res.ok) {
			saveErr = res.error ?? t('products.err_save', 'Save failed');
			return;
		}
		onsaved?.(slug.trim());
	}
</script>

<div class="space-y-6">
	<section class="grid grid-cols-1 gap-4 sm:grid-cols-2">
		<div>
			<label for="pf-type" class={lbl}>{t('products.fld_type', 'Type')} *</label>
			<select id="pf-type" class={inp} bind:value={typeSlug} disabled={mode === 'edit'}
				onchange={() => loadSchema(typeSlug)}>
				<option value="">—</option>
				{#each types as tp (tp.slug)}<option value={tp.slug}>{tp.label}</option>{/each}
			</select>
		</div>
		<div>
			<label for="pf-name" class={lbl}>{t('products.fld_name', 'Name')} *</label>
			<input id="pf-name" class={inp} bind:value={name} oninput={onNameInput} />
		</div>
		<div>
			<label for="pf-slug" class={lbl}>{t('products.fld_slug', 'Slug')} *</label>
			<input id="pf-slug" class={inp} bind:value={slug}
				disabled={mode === 'edit'} oninput={() => (slugTouched = true)} />
		</div>
		<div>
			<label for="pf-tagline" class={lbl}>{t('products.fld_tagline', 'Tagline')}</label>
			<input id="pf-tagline" class={inp} bind:value={tagline} />
		</div>
		<div>
			<label for="pf-website" class={lbl}>{t('products.fld_website', 'Website URL')}</label>
			<input id="pf-website" class={inp} type="url" bind:value={websiteUrl} />
		</div>
		<div>
			<label for="pf-pricing" class={lbl}>{t('products.fld_pricing', 'Pricing model')}</label>
			<input id="pf-pricing" class={inp} bind:value={pricingModel} placeholder="free / freemium / subscription" />
		</div>
		<div>
			<label for="pf-price" class={lbl}>{t('products.fld_price_from', 'Price from')}</label>
			<input id="pf-price" class={inp} type="number" step="0.01" min="0" bind:value={priceFrom} />
		</div>
	</section>

	{#if schemaLoading}
		<div class="flex items-center gap-2 text-sm text-[var(--color-text-subtle)]">
			<Loader2 size={15} class="animate-spin" /> {t('common.loading', 'Loading')}
		</div>
	{:else if schema.length > 0}
		{#each schema as g (g.id)}
			<section>
				<h3 class="mb-2 text-sm font-semibold text-[var(--color-text)]">{g.name}</h3>
				<div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
					{#each g.fields as f (f.id)}
						<div>
							<label class={lbl} for="pf-f-{f.id}">
								{f.label}{f.is_required ? ' *' : ''}
							</label>
							{#if f.field_type === 'checkbox' || f.field_type === 'boolean'}
								<input id="pf-f-{f.id}" type="checkbox" bind:checked={fieldValues[f.id]} class="h-4 w-4" />
							{:else if f.field_type === 'number'}
								<input id="pf-f-{f.id}" class={inp} type="number" bind:value={fieldValues[f.id]} placeholder={f.placeholder ?? ''} />
							{:else if f.field_type === 'date'}
								<input id="pf-f-{f.id}" class={inp} type="date" bind:value={fieldValues[f.id]} />
							{:else if f.field_type === 'select' && fieldOptions(f).length > 0}
								<select id="pf-f-{f.id}" class={inp} bind:value={fieldValues[f.id]}>
									<option value="">—</option>
									{#each fieldOptions(f) as o}<option value={typeof o === 'object' ? (o.value ?? o.label) : o}>{typeof o === 'object' ? (o.label ?? o.value) : o}</option>{/each}
								</select>
							{:else if f.field_type === 'textarea' || f.field_type === 'rich_text'}
								<textarea id="pf-f-{f.id}" class={inp} rows="3" bind:value={fieldValues[f.id]}></textarea>
							{:else}
								<input id="pf-f-{f.id}" class={inp} bind:value={fieldValues[f.id]} placeholder={f.placeholder ?? ''} />
							{/if}
						</div>
					{/each}
				</div>
			</section>
		{/each}
	{/if}

	<section>
		<div class="mb-2 flex items-center justify-between">
			<h3 class="text-sm font-semibold text-[var(--color-text)]">{t('products.providers', 'Providers')}</h3>
			<button type="button" onclick={addProvider}
				class="inline-flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] px-2.5 py-1 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]">
				<Plus size={13} /> {t('products.add_provider', 'Add provider')}
			</button>
		</div>
		{#if providerRows.length === 0}
			<p class="text-xs text-[var(--color-text-subtle)]">{t('products.no_providers', 'No providers linked — add at least one reseller.')}</p>
		{:else}
			<div class="space-y-2">
				{#each providerRows as row, i (i)}
					<div class="grid grid-cols-[1fr_120px_1fr_auto] items-end gap-2">
						<div>
							<label class={lbl} for="pf-pv-{i}">{t('products.provider', 'Provider')}</label>
							<select id="pf-pv-{i}" class={inp} bind:value={row.provider_id}>
								<option value="">—</option>
								{#each providersAll as p (p.id)}<option value={p.id}>{p.name}</option>{/each}
							</select>
						</div>
						<div>
							<label class={lbl} for="pf-pp-{i}">{t('products.fld_price_from', 'Price from')}</label>
							<input id="pf-pp-{i}" class={inp} bind:value={row.price_from} placeholder="0.00" />
						</div>
						<div>
							<label class={lbl} for="pf-po-{i}">{t('products.offer_url', 'Offer URL')}</label>
							<input id="pf-po-{i}" class={inp} type="url" bind:value={row.offer_url} />
						</div>
						<button type="button" onclick={() => removeProvider(i)}
							class="rounded-[var(--radius)] p-2 text-[var(--color-muted)] hover:text-red-500"
							aria-label={t('common.remove', 'Remove')}>
							<Trash2 size={15} />
						</button>
					</div>
				{/each}
			</div>
		{/if}
	</section>

	<section>
		<h3 class="mb-2 text-sm font-semibold text-[var(--color-text)]">{t('products.deals', 'Deals')}</h3>
		{#if dealOptions.length === 0}
			<p class="text-xs text-[var(--color-text-subtle)]">{t('products.no_deals', 'No active deals available.')}</p>
		{:else}
			<ul class="max-h-48 space-y-1 overflow-y-auto rounded-[var(--radius)] border border-[var(--color-border)] p-2">
				{#each dealOptions as d (d.id)}
					<li>
						<label class="flex items-center gap-2 px-1 py-1 text-sm text-[var(--color-text)]">
							<input type="checkbox" checked={dealIds.includes(d.id)} onchange={() => toggleDeal(d.id)} class="h-4 w-4" />
							<span class="min-w-0 flex-1 truncate">{d.title}</span>
							{#if d.provider_name}
								<span class="shrink-0 text-xs text-[var(--color-muted)]">{d.provider_name}</span>
							{/if}
						</label>
					</li>
				{/each}
			</ul>
		{/if}
	</section>

	{#if saveErr}
		<p class="text-sm text-red-500" role="alert">{saveErr}</p>
	{/if}

	<div class="flex justify-end gap-2">
		{#if oncancel}
			<button type="button" onclick={oncancel}
				class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]">
				{t('common.cancel', 'Cancel')}
			</button>
		{/if}
		<button type="button" onclick={submit} disabled={saving}
			class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white transition-opacity disabled:opacity-50">
			{#if saving}<Loader2 size={14} class="inline animate-spin" />{/if}
			{mode === 'create' ? t('common.create', 'Create') : t('common.save', 'Save')}
		</button>
	</div>
</div>
