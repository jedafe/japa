<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	// /product-types/new — create a product type (§80). The backend provisions
	// a custom_field_groups row per category (+ cfga assignment + comparison pool).
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { productTypesAdminApi } from '$lib/api';
	import { goto } from '$app/navigation';
	import { t } from '$lib/stores/i18n.svelte';
	import { Plus, Trash2 } from 'lucide-svelte';

	const inp =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]';
	const lbl = 'mb-1 block text-xs font-medium text-[var(--color-text-subtle)]';

	let slug = $state('');
	let slugTouched = $state(false);
	let label = $state('');
	let icon = $state('');
	let categories = $state([{ slug: 'overview', label: 'Overview' }]);
	let saving = $state(false);
	let err = $state('');

	function slugify(s: string) {
		return s.toLowerCase().trim().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '');
	}
	function onLabel() {
		if (!slugTouched) slug = slugify(label);
	}
	async function submit() {
		err = '';
		if (!slug.trim() || !label.trim()) {
			err = t('product_types.err_required', 'Slug and label are required');
			return;
		}
		saving = true;
		const res = await productTypesAdminApi.create({
			slug: slug.trim(),
			label: label.trim(),
			icon: icon.trim() || undefined,
			categories: categories.filter((c) => c.slug.trim() && c.label.trim())
		});
		saving = false;
		if (!res.ok) { err = res.error ?? t('products.err_save', 'Save failed'); return; }
		goto(`/product-types/${slug.trim()}`);
	}
</script>

<svelte:head><title>{t('product_types.new', 'New product type')} — {siteName()}</title></svelte:head>

<PageHeader title={t('product_types.new', 'New product type')} description={t('product_types.new_desc', 'A type binds one custom field group per category — products of this type inherit the schema.')} />

<div class="mx-auto max-w-2xl space-y-6 p-4">
	<div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
		<div>
			<label for="pt-label" class={lbl}>{t('products.fld_label', 'Label')} *</label>
			<input id="pt-label" class={inp} bind:value={label} oninput={onLabel} />
		</div>
		<div>
			<label for="pt-slug" class={lbl}>{t('products.fld_slug', 'Slug')} *</label>
			<input id="pt-slug" class={inp} bind:value={slug} oninput={() => (slugTouched = true)} />
		</div>
		<div>
			<label for="pt-icon" class={lbl}>{t('products.fld_icon', 'Icon')}</label>
			<input id="pt-icon" class={inp} bind:value={icon} placeholder="sparkles" />
		</div>
	</div>

	<section>
		<div class="mb-2 flex items-center justify-between">
			<h3 class="text-sm font-semibold text-[var(--color-text)]">{t('product_types.categories', 'Field group categories')}</h3>
			<button type="button" onclick={() => (categories = [...categories, { slug: '', label: '' }])}
				class="inline-flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] px-2.5 py-1 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]">
				<Plus size={13} /> {t('product_types.add_category', 'Add category')}
			</button>
		</div>
		<div class="space-y-2">
			{#each categories as cat, i (i)}
				<div class="grid grid-cols-[1fr_1fr_auto] items-end gap-2">
					<div>
						<label class={lbl} for="pt-cs-{i}">{t('products.fld_slug', 'Slug')}</label>
						<input id="pt-cs-{i}" class={inp} bind:value={cat.slug} placeholder="overview" />
					</div>
					<div>
						<label class={lbl} for="pt-cl-{i}">{t('products.fld_label', 'Label')}</label>
						<input id="pt-cl-{i}" class={inp} bind:value={cat.label} placeholder="Overview" />
					</div>
					<button type="button" onclick={() => (categories = categories.filter((_, j) => j !== i))}
						class="rounded-[var(--radius)] p-2 text-[var(--color-muted)] hover:text-red-500" aria-label={t('common.remove', 'Remove')}>
						<Trash2 size={15} />
					</button>
				</div>
			{/each}
		</div>
	</section>

	{#if err}<p class="text-sm text-red-500" role="alert">{err}</p>{/if}

	<div class="flex justify-end gap-2">
		<button type="button" onclick={() => goto('/product-types')}
			class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]">
			{t('common.cancel', 'Cancel')}
		</button>
		<button type="button" onclick={submit} disabled={saving}
			class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white disabled:opacity-50">
			{t('common.create', 'Create')}
		</button>
	</div>
</div>
