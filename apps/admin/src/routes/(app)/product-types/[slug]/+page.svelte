<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	// /product-types/[slug] — type edit + bound field-group management (§80).
	// Fields are edited via the existing /custom-field-groups endpoints.
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import Modal from '$lib/components/ui/modal/modal.svelte';
	import { api, productTypesAdminApi, fieldGroupsAdminApiExtended, fieldTypesAdminApi, type FieldTypeInfo } from '$lib/api';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { t } from '$lib/stores/i18n.svelte';
	import { onMount } from 'svelte';
	import { ExternalLink, Loader2, Plus, Trash2 } from 'lucide-svelte';

	const slug = $derived(page.params.slug ?? '');
	const inp =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]';
	const lbl = 'mb-1 block text-xs font-medium text-[var(--color-text-subtle)]';

	let detail = $state<any>(null);
	let loading = $state(true);
	let loadErr = $state('');
	let label = $state('');
	let icon = $state('');
	let isEnabled = $state(true);
	let newCat = $state({ slug: '', label: '' });
	let saving = $state(false);
	let err = $state('');

	// Add-field modal state.
	let fieldModal = $state<{ group: any } | null>(null);
	let newField = $state({ key: '', label: '', field_type: 'text', is_required: false });
	let fieldTypes = $state<FieldTypeInfo[]>([]);
	let fieldErr = $state('');

	onMount(async () => {
		load();
		const ft = await fieldTypesAdminApi.list();
		if (ft.ok && Array.isArray(ft.data)) fieldTypes = ft.data;
	});

	async function load() {
		loading = true;
		const res = await productTypesAdminApi.get(slug);
		loading = false;
		if (!res.ok) { loadErr = res.error ?? 'Failed'; return; }
		detail = res.data;
		label = detail?.type?.label ?? '';
		icon = detail?.type?.icon ?? '';
		isEnabled = detail?.type?.is_enabled ?? true;
	}

	async function save() {
		err = '';
		saving = true;
		const body: Record<string, unknown> = {
			label: label.trim(),
			icon: icon.trim(),
			is_enabled: isEnabled
		};
		if (newCat.slug.trim() && newCat.label.trim()) {
			body.categories = [{ slug: newCat.slug.trim(), label: newCat.label.trim() }];
		}
		const res = await productTypesAdminApi.update(slug, body);
		saving = false;
		if (!res.ok) { err = res.error ?? 'Save failed'; return; }
		newCat = { slug: '', label: '' };
		await load();
	}

	async function addField() {
		fieldErr = '';
		if (!newField.key.trim() || !newField.label.trim()) {
			fieldErr = t('product_types.err_field', 'Key and label are required');
			return;
		}
		const res = await fieldGroupsAdminApiExtended.addField(fieldModal!.group.id, {
			key: newField.key.trim(),
			label: newField.label.trim(),
			field_type: newField.field_type,
			is_required: newField.is_required
		});
		if (!res.ok) { fieldErr = res.error ?? 'Failed'; return; }
		fieldModal = null;
		newField = { key: '', label: '', field_type: 'text', is_required: false };
		await load();
	}

	async function removeField(groupId: string, fieldId: string) {
		await fieldGroupsAdminApiExtended.deleteField(groupId, fieldId);
		await load();
	}
</script>

<svelte:head><title>{detail?.type?.label ?? slug} — {siteName()}</title></svelte:head>

<PageHeader title={detail?.type?.label ?? t('product_types.edit', 'Edit product type')} description={slug} />

<div class="mx-auto max-w-3xl space-y-6 p-4">
	{#if loading}
		<div class="flex items-center gap-2 py-8 text-sm text-[var(--color-text-subtle)]">
			<Loader2 size={16} class="animate-spin" /> {t('common.loading', 'Loading')}
		</div>
	{:else if loadErr}
		<p class="py-8 text-sm text-red-500" role="alert">{loadErr}</p>
	{:else if detail}
		<section class="grid grid-cols-1 gap-4 sm:grid-cols-2">
			<div>
				<label class={lbl} for="pt-label">{t('products.fld_label', 'Label')}</label>
				<input id="pt-label" class={inp} bind:value={label} />
			</div>
			<div>
				<label class={lbl} for="pt-icon">{t('products.fld_icon', 'Icon')}</label>
				<input id="pt-icon" class={inp} bind:value={icon} />
			</div>
			<label class="flex items-center gap-2 text-sm text-[var(--color-text)]">
				<input type="checkbox" bind:checked={isEnabled} class="h-4 w-4" />
				{t('product_types.enabled', 'Enabled — visible as a product type')}
			</label>
		</section>

		<section>
			<h3 class="mb-2 text-sm font-semibold text-[var(--color-text)]">
				{t('product_types.groups', 'Bound field groups')}
			</h3>
			{#each detail.groups ?? [] as g (g.id)}
				<div class="mb-3 rounded-[var(--radius)] border border-[var(--color-border)] p-3">
					<div class="mb-2 flex items-center justify-between">
						<div>
							<span class="text-sm font-medium text-[var(--color-text)]">{g.name}</span>
							<span class="ml-2 text-xs text-[var(--color-muted)]">{g.slug}</span>
						</div>
						<div class="flex items-center gap-2">
							<button type="button" onclick={() => (fieldModal = { group: g })}
								class="inline-flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] px-2 py-1 text-xs text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]">
								<Plus size={12} /> {t('product_types.add_field', 'Add field')}
							</button>
							<a href="/custom-fields" class="inline-flex items-center gap-1 text-xs text-[var(--color-primary)] hover:underline">
								<ExternalLink size={12} /> {t('product_types.full_editor', 'Full editor')}
							</a>
						</div>
					</div>
					{#if g.fields.length === 0}
						<p class="text-xs text-[var(--color-text-subtle)]">{t('product_types.no_fields', 'No fields yet.')}</p>
					{:else}
						<ul>
							{#each g.fields as f (f.id)}
								<li class="flex items-center justify-between border-t border-[var(--color-border)] py-1.5 text-sm">
									<span class="text-[var(--color-text)]">{f.label}
										<span class="ml-1 text-xs text-[var(--color-muted)]">{f.key} · {f.field_type}{f.is_required ? ' · required' : ''}</span>
									</span>
									<button type="button" onclick={() => removeField(g.id, f.id)}
										class="rounded-[var(--radius)] p-1 text-[var(--color-muted)] hover:text-red-500" aria-label={t('common.remove', 'Remove')}>
										<Trash2 size={13} />
									</button>
								</li>
							{/each}
						</ul>
					{/if}
				</div>
			{/each}
		</section>

		<section>
			<h3 class="mb-2 text-sm font-semibold text-[var(--color-text)]">{t('product_types.add_category', 'Add category')}</h3>
			<div class="grid grid-cols-2 gap-2">
				<input class={inp} bind:value={newCat.slug} placeholder="pricing" />
				<input class={inp} bind:value={newCat.label} placeholder="Pricing" />
			</div>
			<p class="mt-1 text-xs text-[var(--color-text-subtle)]">
				{t('product_types.add_category_hint', 'Saved on Save — provisions the group, type assignment and comparison pool row.')}
			</p>
		</section>

		{#if err}<p class="text-sm text-red-500" role="alert">{err}</p>{/if}
		<div class="flex justify-end gap-2">
			<button type="button" onclick={() => goto('/product-types')}
				class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]">
				{t('common.back', 'Back')}
			</button>
			<button type="button" onclick={save} disabled={saving}
				class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white disabled:opacity-50">
				{t('common.save', 'Save')}
			</button>
		</div>
	{/if}
</div>

{#if fieldModal}
	<Modal title={t('product_types.add_field', 'Add field')} onclose={() => (fieldModal = null)}>
		<div class="space-y-3">
			<div>
				<label class={lbl} for="nf-key">{t('products.fld_key', 'Key')} *</label>
				<input id="nf-key" class={inp} bind:value={newField.key} />
			</div>
			<div>
				<label class={lbl} for="nf-label">{t('products.fld_label', 'Label')} *</label>
				<input id="nf-label" class={inp} bind:value={newField.label} />
			</div>
			<div>
				<label class={lbl} for="nf-type">{t('products.fld_field_type', 'Field type')}</label>
				<select id="nf-type" class={inp} bind:value={newField.field_type}>
					{#if fieldTypes.length === 0}
						<option value="text">text</option>
					{:else}
						{#each fieldTypes as ft}
							<option value={ft.value}>{ft.label}</option>
						{/each}
					{/if}
				</select>
			</div>
			<label class="flex items-center gap-2 text-sm text-[var(--color-text)]">
				<input type="checkbox" bind:checked={newField.is_required} class="h-4 w-4" />
				{t('products.required', 'Required')}
			</label>
			{#if fieldErr}<p class="text-sm text-red-500" role="alert">{fieldErr}</p>{/if}
		</div>
		{#snippet footer()}
			<button type="button" onclick={() => (fieldModal = null)}
				class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm text-[var(--color-text)]">
				{t('common.cancel', 'Cancel')}
			</button>
			<button type="button" onclick={addField}
				class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white">
				{t('common.add', 'Add')}
			</button>
		{/snippet}
	</Modal>
{/if}
