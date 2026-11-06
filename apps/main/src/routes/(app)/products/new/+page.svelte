<script lang="ts">
	import SeoHead from '$lib/seo/SeoHead.svelte';
	// /products/new — product create form (§80). Gated server-side by
	// products.create; submits POST /admin/products with providers, deals and
	// the selected type's custom field values in one body.
	import { goto } from '$app/navigation';
	import { ArrowLeft, Loader2, Plus, Trash2 } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import MediaLibraryModal from '$lib/components/media/MediaLibraryModal.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('products.new_title', 'New product — {site}'));
	const lbl = 'mb-1 block text-xs font-medium text-[var(--color-muted)]';
	const inp =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]';

	type FieldDef = {
		id: string; key: string; label: string; field_type: string;
		options?: any; placeholder?: string | null; is_required: boolean;
	};
	type SchemaGroup = { id: string; slug: string; name: string; fields: FieldDef[] };

	let typeSlug = $state(data.types[0]?.slug ?? '');
	let name = $state('');
	let slug = $state('');
	let slugTouched = $state(false);
	let tagline = $state('');
	let description = $state('');
	let logoId = $state<string | null>(null);
	let logoUrl = $state<string | null>(null);
	type GalleryItem = { media_id: string; url: string };
	let gallery = $state<GalleryItem[]>([]);
	let pickerOpen = $state(false);
	let pickerTarget = $state<'logo' | 'gallery' | string>('logo');
	function openPicker(target: 'logo' | 'gallery' | string) {
		pickerTarget = target;
		pickerOpen = true;
	}
	function onPicked(m: { id: string; url: string }) {
		if (pickerTarget === 'logo') {
			logoId = m.id;
			logoUrl = m.url;
		} else if (pickerTarget === 'gallery') {
			if (!gallery.some((g) => g.media_id === m.id)) {
				gallery = [...gallery, { media_id: m.id, url: m.url }];
			}
		} else if (pickerTarget.startsWith('multi:')) {
			const fid = pickerTarget.slice(6);
			const cur = Array.isArray(fieldValues[fid]) ? fieldValues[fid] : [];
			fieldValues[fid] = [...cur, m.id];
		} else {
			fieldValues[pickerTarget] = m.id;
		}
	}
	let websiteUrl = $state('');
	let pricingModel = $state('');
	let priceFrom = $state('');

	let providerRows = $state<Array<{ provider_id: string; price_from: string; offer_url: string }>>([]);
	let dealIds = $state<string[]>([]);
	let schema = $state<SchemaGroup[]>([]);
	let schemaLoading = $state(false);
	let fieldValues = $state<Record<string, any>>({});
	let saving = $state(false);
	let saveErr = $state('');

	/** The 22 backend field types collapse into these renderer kinds. */
	function fieldKind(f: FieldDef): string {
		switch (f.field_type) {
			case 'number': case 'currency': case 'percentage': case 'duration': case 'rating':
				return 'number';
			case 'checkbox': case 'boolean': return 'bool';
			case 'date': return 'date';
			case 'select': return 'select';
			case 'multiselect': return 'multiselect';
			case 'suggest': return 'suggest';
			case 'image_single': case 'media': return 'image_single';
			case 'image_multi': return 'image_multi';
			case 'textarea': case 'rich_text': return 'textarea';
			case 'email': return 'email';
			case 'phone': return 'tel';
			case 'url': return 'url';
			case 'repeater': case 'price_parts': case 'price_range': case 'json': return 'json';
			default: return 'text';
		}
	}

	/** custom_fields.options is jsonb — bare array or {choices:[...]}. */
	function fieldOptions(f: FieldDef): Array<any> {
		const o = f.options;
		if (Array.isArray(o)) return o;
		if (o && Array.isArray(o.choices)) return o.choices;
		if (o && Array.isArray(o.options)) return o.options;
		return [];
	}

	async function loadSchema(ts: string) {
		schema = [];
		fieldValues = {};
		if (!ts) return;
		schemaLoading = true;
		const res = await api.get<{ groups: SchemaGroup[] }>(`/products/types/${ts}/field-schema`);
		schemaLoading = false;
		schema = res.data?.groups ?? [];
	}

	$effect(() => {
		loadSchema(typeSlug);
	});

	function slugify(s: string) {
		return s.toLowerCase().trim().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '');
	}
	function onName() {
		if (!slugTouched) slug = slugify(name);
	}

	const dealOptions = $derived.by(() => {
		const chosen = new Set(providerRows.map((r) => r.provider_id).filter(Boolean));
		if (chosen.size === 0) return data.deals;
		const scoped = data.deals.filter((d) => d.provider_id && chosen.has(d.provider_id));
		return scoped.length > 0 ? scoped : data.deals;
	});

	function toggleDeal(id: string) {
		dealIds = dealIds.includes(id) ? dealIds.filter((x) => x !== id) : [...dealIds, id];
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
				switch (fieldKind(f)) {
					case 'number': entry.value_number = Number(v); break;
					case 'bool': entry.value_bool = !!v; break;
					case 'date': entry.value_date = new Date(v).toISOString(); break;
					case 'multiselect': case 'image_multi': case 'json':
						entry.value_json = typeof v === 'string' ? JSON.parse(v || 'null') : v; break;
					default: entry.value_text = String(v);
				}
				custom_fields.push(entry);
			}
		}
		const res = await api.post<{ slug?: string }>('/admin/products', {
			type_slug: typeSlug,
			name: name.trim(),
			slug: slug.trim(),
			tagline: tagline || null,
			description: description || null,
			logo_id: logoId,
			gallery_media_ids: gallery.map((g) => g.media_id),
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
			custom_fields,
			clear_field_ids: schema
				.flatMap((g) => g.fields)
				.filter((f) => !custom_fields.some((cf) => cf.field_id === f.id))
				.map((f) => f.id)
		});
		saving = false;
		if (res.error) {
			saveErr = res.error.message || t('products.err_save', 'Save failed');
			return;
		}
		goto(`/product/${encodeURIComponent(res.data?.slug ?? slug.trim())}`);
	}
</script>

<SeoHead title={title} ogTitle={title} robots="noindex" description={t('products.new_description', 'Add a product to the catalog.')} />

<div class="mx-auto w-full max-w-full">
	<div class="mx-auto flex max-w-2xl items-center gap-3 border-b border-[var(--color-border)] px-4 py-3">
		<a href="/products" class="grid h-8 w-8 place-items-center rounded-[var(--radius)] text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]" aria-label={t('products.back', 'All products')}>
			<ArrowLeft class="rtl:-scale-x-100" size={18} />
		</a>
		<h1 class="text-lg font-semibold text-[var(--color-text)]">{t('products.new_product', 'New product')}</h1>
	</div>

	<div class="mx-auto max-w-2xl space-y-6 p-4">
		<section class="grid grid-cols-1 gap-4 sm:grid-cols-2">
			<div>
				<label for="np-type" class={lbl}>{t('products.fld_type', 'Type')} *</label>
				<select id="np-type" class={inp} bind:value={typeSlug}>
					{#each data.types as tp (tp.slug)}<option value={tp.slug}>{tp.label}</option>{/each}
				</select>
			</div>
			<div>
				<label for="np-name" class={lbl}>{t('products.fld_name', 'Name')} *</label>
				<input id="np-name" class={inp} bind:value={name} oninput={onName} />
			</div>
			<div>
				<label for="np-slug" class={lbl}>{t('products.fld_slug', 'Slug')} *</label>
				<input id="np-slug" class={inp} bind:value={slug} oninput={() => (slugTouched = true)} />
			</div>
			<div>
				<label for="np-tagline" class={lbl}>{t('products.fld_tagline', 'Tagline')}</label>
				<input id="np-tagline" class={inp} bind:value={tagline} />
			</div>
			<div class="sm:col-span-2">
				<label for="np-desc" class={lbl}>{t('products.fld_description', 'Description')}</label>
				<textarea id="np-desc" class={inp} rows="3" bind:value={description}></textarea>
			</div>
			<div>
				<label for="np-website" class={lbl}>{t('products.fld_website', 'Website URL')}</label>
				<input id="np-website" class={inp} type="url" bind:value={websiteUrl} />
			</div>
			<div>
				<label for="np-pricing" class={lbl}>{t('products.fld_pricing', 'Pricing model')}</label>
				<input id="np-pricing" class={inp} bind:value={pricingModel} placeholder="free / freemium / subscription" />
			</div>
			<div>
				<label for="np-price" class={lbl}>{t('products.fld_price_from', 'Price from')}</label>
				<input id="np-price" class={inp} type="number" step="0.01" min="0" bind:value={priceFrom} />
			</div>
		</section>

		{#if schemaLoading}
			<div class="flex items-center gap-2 text-sm text-[var(--color-text-subtle)]">
				<Loader2 size={15} class="animate-spin" /> {t('common.loading', 'Loading')}
			</div>
		{:else}
			{#each schema as g (g.id)}
				<section>
					<h2 class="mb-2 text-sm font-semibold text-[var(--color-text)]">{g.name}</h2>
					<div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
						{#each g.fields as f (f.id)}
							{@const kind = fieldKind(f)}
							<div>
								<label class={lbl} for="np-f-{f.id}">{f.label}{f.is_required ? ' *' : ''}</label>
								{#if kind === 'bool'}
									<input id="np-f-{f.id}" type="checkbox" bind:checked={fieldValues[f.id]} class="h-4 w-4" />
								{:else if kind === 'number'}
									<input id="np-f-{f.id}" class={inp} type="number" bind:value={fieldValues[f.id]} placeholder={f.placeholder ?? ''} />
								{:else if kind === 'date'}
									<input id="np-f-{f.id}" class={inp} type="date" bind:value={fieldValues[f.id]} />
								{:else if kind === 'select'}
									<select id="np-f-{f.id}" class={inp} bind:value={fieldValues[f.id]}>
										<option value="">—</option>
										{#each fieldOptions(f) as o}<option value={typeof o === 'object' ? (o.value ?? o.label) : o}>{typeof o === 'object' ? (o.label ?? o.value) : o}</option>{/each}
									</select>
								{:else if kind === 'multiselect'}
									<div class="space-y-1">
										{#each fieldOptions(f) as o}
											{@const ov = typeof o === 'object' ? (o.value ?? o.label) : o}
											<label class="flex items-center gap-2 text-sm text-[var(--color-text)]">
												<input type="checkbox" class="h-4 w-4"
													checked={Array.isArray(fieldValues[f.id]) && fieldValues[f.id].includes(ov)}
													onchange={(e) => {
														const cur = Array.isArray(fieldValues[f.id]) ? fieldValues[f.id] : [];
														fieldValues[f.id] = e.currentTarget.checked ? [...cur, ov] : cur.filter((x: any) => x !== ov);
													}} />
												{typeof o === 'object' ? (o.label ?? o.value) : o}
											</label>
										{/each}
									</div>
								{:else if kind === 'suggest'}
									<input id="np-f-{f.id}" class={inp} list="dl-{f.id}" bind:value={fieldValues[f.id]} placeholder={f.placeholder ?? ''} />
									<datalist id="dl-{f.id}">
										{#each fieldOptions(f) as o}<option value={typeof o === 'object' ? (o.value ?? o.label) : o}>{typeof o === 'object' ? (o.label ?? o.value) : o}</option>{/each}
									</datalist>
								{:else if kind === 'image_single'}
									<div class="flex items-center gap-2">
										<button type="button" onclick={() => openPicker(f.id)}
											class="inline-flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] px-2.5 py-1.5 text-xs text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)] disabled:opacity-50">
											<Plus size={13} /> {fieldValues[f.id] ? t('products.change_image', 'Change image') : t('products.pick_image', 'Pick image')}
										</button>
										{#if fieldValues[f.id]}
											<span class="text-xs text-green-600">✓</span>
											<button type="button" onclick={() => (fieldValues[f.id] = '')} class="text-xs text-red-500 hover:underline">{t('common.clear', 'Clear')}</button>
										{/if}
									</div>
								{:else if kind === 'image_multi'}
									<div class="flex items-center gap-2">
										<button type="button" onclick={() => openPicker('multi:' + f.id)}
											class="inline-flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] px-2.5 py-1.5 text-xs text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)] disabled:opacity-50">
											<Plus size={13} /> {t('products.add_image', 'Add image')}
										</button>
										<span class="text-xs text-[var(--color-muted)]">{Array.isArray(fieldValues[f.id]) ? fieldValues[f.id].length : 0}</span>
									</div>
								{:else if kind === 'textarea'}
									<textarea id="np-f-{f.id}" class={inp} rows="3" bind:value={fieldValues[f.id]}></textarea>
								{:else if kind === 'email'}
									<input id="np-f-{f.id}" class={inp} type="email" bind:value={fieldValues[f.id]} />
								{:else if kind === 'tel'}
									<input id="np-f-{f.id}" class={inp} type="tel" bind:value={fieldValues[f.id]} />
								{:else if kind === 'url'}
									<input id="np-f-{f.id}" class={inp} type="url" bind:value={fieldValues[f.id]} />
								{:else if kind === 'json'}
									<textarea id="np-f-{f.id}" class={inp} rows="3"
										value={typeof fieldValues[f.id] === 'object' ? JSON.stringify(fieldValues[f.id], null, 2) : (fieldValues[f.id] ?? '')}
										oninput={(e) => { try { fieldValues[f.id] = JSON.parse(e.currentTarget.value); } catch { fieldValues[f.id] = e.currentTarget.value; } }}
										placeholder={'{"key": "value"}'}></textarea>
								{:else}
									<input id="np-f-{f.id}" class={inp} bind:value={fieldValues[f.id]} placeholder={f.placeholder ?? ''} />
								{/if}
							</div>
						{/each}
					</div>
				</section>
			{/each}
		{/if}

		<section class="grid grid-cols-1 gap-4 sm:grid-cols-2">
			<div>
				<h2 class="mb-2 text-sm font-semibold text-[var(--color-text)]">{t('products.logo', 'Logo')}</h2>
				<div class="flex items-center gap-3">
					{#if logoUrl}
						<img src={logoUrl} alt="" class="h-14 w-14 rounded-[var(--radius)] border border-[var(--color-border)] object-cover" />
					{/if}
					<button type="button" onclick={() => openPicker('logo')}
						class="inline-flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] px-2.5 py-1.5 text-xs text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)] disabled:opacity-50">
						<Plus size={13} /> {logoUrl ? t('products.change_image', 'Change image') : t('products.pick_image', 'Pick image')}
					</button>
					{#if logoUrl}
						<button type="button" onclick={() => { logoId = null; logoUrl = null; }} class="text-xs text-red-500 hover:underline">{t('common.clear', 'Clear')}</button>
					{/if}
				</div>
			</div>
			<div>
				<h2 class="mb-2 text-sm font-semibold text-[var(--color-text)]">{t('products.gallery', 'Gallery')}</h2>
				<div class="flex flex-wrap items-center gap-2">
					{#each gallery as g, i (g.media_id)}
						<div class="relative">
							<img src={g.url} alt="" class="h-14 w-14 rounded-[var(--radius)] border border-[var(--color-border)] object-cover" />
							<button type="button" onclick={() => (gallery = gallery.filter((_, j) => j !== i))}
								class="absolute -right-1 -top-1 rounded-full bg-[var(--color-surface)] p-0.5 text-[var(--color-muted)] hover:text-red-500">✕</button>
						</div>
					{/each}
					<button type="button" onclick={() => openPicker('gallery')}
						class="inline-flex h-14 w-14 items-center justify-center rounded-[var(--radius)] border border-dashed border-[var(--color-border)] text-[var(--color-muted)] hover:border-[var(--color-primary)] disabled:opacity-50">
						<Plus size={16} />
					</button>
				</div>
			</div>
		</section>

		<section>
			<div class="mb-2 flex items-center justify-between">
				<h2 class="text-sm font-semibold text-[var(--color-text)]">{t('products.providers', 'Providers')}</h2>
				<button type="button" onclick={() => (providerRows = [...providerRows, { provider_id: '', price_from: '', offer_url: '' }])}
					class="inline-flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] px-2.5 py-1 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]">
					<Plus size={13} /> {t('products.add_provider', 'Add provider')}
				</button>
			</div>
			<div class="space-y-2">
				{#each providerRows as row, i (i)}
					<div class="grid grid-cols-[1fr_110px_1fr_auto] items-end gap-2">
						<div>
							<label class={lbl} for="np-pv-{i}">{t('products.provider', 'Provider')}</label>
							<select id="np-pv-{i}" class={inp} bind:value={row.provider_id}>
								<option value="">—</option>
								{#each data.providers as p (p.id)}<option value={p.id}>{p.name}</option>{/each}
							</select>
						</div>
						<div>
							<label class={lbl} for="np-pp-{i}">{t('products.fld_price_from', 'Price from')}</label>
							<input id="np-pp-{i}" class={inp} bind:value={row.price_from} placeholder="0.00" />
						</div>
						<div>
							<label class={lbl} for="np-po-{i}">{t('products.offer_url', 'Offer URL')}</label>
							<input id="np-po-{i}" class={inp} type="url" bind:value={row.offer_url} />
						</div>
						<button type="button" onclick={() => (providerRows = providerRows.filter((_, j) => j !== i))}
							class="rounded-[var(--radius)] p-2 text-[var(--color-muted)] hover:text-red-500" aria-label={t('common.remove', 'Remove')}>
							<Trash2 size={15} />
						</button>
					</div>
				{/each}
			</div>
		</section>

		<section>
			<h2 class="mb-2 text-sm font-semibold text-[var(--color-text)]">{t('products.deals', 'Deals')}</h2>
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

	{#if pickerOpen}
		<MediaLibraryModal onSelect={onPicked} onClose={() => (pickerOpen = false)} allowedType="image" />
	{/if}

		{#if saveErr}
			<p class="text-sm text-red-500" role="alert">{saveErr}</p>
		{/if}

		<div class="flex justify-end">
			<button type="button" onclick={submit} disabled={saving}
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white transition-opacity disabled:opacity-50">
				{#if saving}<Loader2 size={14} class="animate-spin" />{/if}
				{t('common.create', 'Create')}
			</button>
		</div>
	</div>
</div>
