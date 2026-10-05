<script lang="ts">
	import { page } from '$app/state';
	import { invalidateAll } from '$app/navigation';
	import { t } from '$lib/i18n';
	import {
		Ticket,
		BadgeCheck,
		Copy,
		ExternalLink,
		Handshake,
		Pencil,
		Loader2,
		AlertCircle
	} from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import type { PageData } from './$types';
	import { safeJsonLdScript } from '$lib/utils/safeHtml';
	import { dealPath, type Coupon } from '$lib/types';
	import { currencySymbol } from '$lib/utils/currency';

	let { data }: { data: PageData } = $props();

	const provider = $derived(data.partner.provider);
	const coupons = $derived(data.partner.coupons);

	const title = $derived(
		t('deal.partner_title', '{name} deals — JapaConnect').replace('{name}', provider.name)
	);
	const description = $derived(
		provider.description ??
			t('deal.partner_description', 'Active coupons and deals from {name}.').replace(
				'{name}',
				provider.name
			)
	);

	const jsonLd = $derived(
		JSON.stringify({
			'@context': 'https://schema.org',
			'@type': 'CollectionPage',
			name: title,
			description,
			url: page.url.href
		})
	);

	function discountLabel(c: Coupon): string {
		if (c.discount_type === 'percent') {
			return t('coupons.percent_off', '{n}% off').replace('{n}', String(c.discount_value ?? ''));
		}
		if (c.discount_type === 'fixed') {
			const cur = currencySymbol(c.currency, data.currencies) || (c.currency ?? '');
			return t('coupons.amount_off', '{cur}{n} off')
				.replace('{cur}', cur)
				.replace('{n}', String(c.discount_value ?? ''));
		}
		if (c.discount_type === 'free_shipping') {
			return t('coupons.free_shipping', 'Free shipping');
		}
		if (c.discount_value !== undefined && c.discount_value !== null) {
			return String(c.discount_value);
		}
		return '';
	}

	// Inline provider edit (staff only — gated by data.canManageProviders, §55).
	// Page-local form: this is its only use, so no shared component.
	let editing = $state(false);
	let pf = $state({
		name: '',
		slug: '',
		description: '',
		category: '',
		base_url: '',
		is_featured: false,
		is_active: false
	});
	let providerSaving = $state(false);
	let providerError = $state('');

	const inputClass =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20';
	const labelClass = 'mb-1 block text-sm font-medium text-[var(--color-text)]';

	function toggleEdit() {
		if (editing) {
			editing = false;
			return;
		}
		// Seed the form from the loaded provider on open.
		pf = {
			name: provider.name ?? '',
			slug: provider.slug ?? '',
			description: provider.description ?? '',
			category: provider.category ?? '',
			base_url: provider.base_url ?? '',
			is_featured: provider.is_featured,
			is_active: provider.is_active
		};
		providerError = '';
		editing = true;
	}

	async function saveProvider(e: SubmitEvent) {
		e.preventDefault();
		providerError = '';
		if (!pf.name.trim()) {
			providerError = t('deal.form.errors.name_required', 'Name is required');
			return;
		}
		providerSaving = true;
		try {
			// PATCH /admin/affiliate-providers/{id} — UpdateProvider rewrites
			// every column, so send the full CreateProviderInput field set:
			// fields this form doesn't edit are passed through unchanged so a
			// save can't null them out.
			const res = await api.patch(`/admin/affiliate-providers/${provider.id}`, {
				name: pf.name.trim(),
				slug: pf.slug.trim() || provider.slug,
				provider_type: provider.provider_type,
				description: pf.description.trim() || null,
				category: pf.category.trim() || null,
				base_url: pf.base_url.trim() || null,
				is_featured: pf.is_featured,
				is_active: pf.is_active,
				logo_media_id: provider.logo_media_id ?? null,
				tracking_param: provider.tracking_param ?? null,
				tracking_value: provider.tracking_value ?? null,
				commission_rate: provider.commission_rate ?? null
			});
			if (res.data) {
				await invalidateAll();
				editing = false;
			} else {
				providerError =
					res.error?.message ??
					t('deal.form.errors.provider_save_failed', 'Failed to save provider');
			}
		} catch (err) {
			providerError =
				err instanceof Error
					? err.message
					: t('deal.form.errors.provider_save_failed', 'Failed to save provider');
		} finally {
			providerSaving = false;
		}
	}

	// recordEvent fires-and-forgets a copy event to the backend (Section 55).
	// Errors are silently ignored — event tracking must never block the UI.
	function recordEvent(couponId: string, eventType: 'click' | 'copy') {
		api.post(`/coupons/${couponId}/click`, { event_type: eventType }).catch(() => {});
	}

	async function copyCode(coupon: Coupon) {
		if (!coupon.code) return;
		try {
			await navigator.clipboard.writeText(coupon.code);
			recordEvent(coupon.id, 'copy');
		} catch {
			// Clipboard may be unavailable; ignore silently.
		}
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<link rel="canonical" href={page.url.href} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
	<meta name="twitter:card" content="summary" />
	<meta name="twitter:title" content={title} />
	<meta name="twitter:description" content={description} />
	{@html safeJsonLdScript(jsonLd)}
</svelte:head>

<div class="mx-auto w-full max-w-full">
	<div class="mx-4 border-b border-[var(--color-border)] px-4 py-4">
		<a
			href="/deals"
			class="mb-2 inline-flex items-center gap-1.5 text-sm text-[var(--color-text-subtle)] transition-colors hover:text-[var(--color-text)]"
		>
			{t('deals.back', 'All deals')}
		</a>
		<div class="flex items-center gap-3">
			<span
				class="grid h-12 w-12 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-[var(--color-primary)]"
			>
				<Handshake size={24} />
			</span>
			<div class="min-w-0 flex-1">
				<div class="flex items-center gap-2">
					<h1 class="truncate text-xl font-semibold text-[var(--color-text)]">{provider.name}</h1>
					{#if provider.category}
						<span
							class="inline-flex shrink-0 items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] px-2 py-0.5 text-xs font-medium text-[var(--color-text-subtle)]"
							title={t('deal.category', 'Category')}
						>
							{provider.category}
						</span>
					{/if}
				</div>
				{#if provider.description}
					<p class="truncate text-xs text-[var(--color-muted)]">{provider.description}</p>
				{/if}
			</div>
			{#if provider.is_featured}
				<span
					class="inline-flex shrink-0 items-center gap-1 rounded-[var(--radius)] bg-[var(--color-surface-subtle)] px-2 py-1 text-xs font-medium text-[var(--color-primary)]"
				>
					<BadgeCheck size={14} />
					{t('deal.featured', 'Featured partner')}
				</span>
			{/if}
			{#if data.canManageProviders}
				<button
					type="button"
					onclick={toggleEdit}
					aria-expanded={editing}
					class="inline-flex shrink-0 items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
				>
					<Pencil size={14} />
					{t('deal.edit_provider', 'Edit provider')}
				</button>
			{/if}
		</div>

		{#if editing}
			<!-- Ref: §55 — staff inline provider edit (PATCH /admin/affiliate-providers/{id}). -->
			<form
				onsubmit={saveProvider}
				novalidate
				class="mt-4 space-y-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-4"
			>
				<div class="grid gap-4 sm:grid-cols-2">
					<div>
						<label for="pf-name" class={labelClass}>
							{t('deal.form.provider_name', 'Provider name')}
							<span class="text-red-500">*</span>
						</label>
						<input id="pf-name" type="text" bind:value={pf.name} class={inputClass} />
					</div>
					<div>
						<label for="pf-slug" class={labelClass}>{t('deal.form.slug', 'Slug')}</label>
						<input id="pf-slug" type="text" bind:value={pf.slug} class={inputClass} />
					</div>
					<div>
						<label for="pf-category" class={labelClass}>
							{t('deal.form.provider_category', 'Category')}
						</label>
						<input id="pf-category" type="text" bind:value={pf.category} class={inputClass} />
					</div>
					<div>
						<label for="pf-base-url" class={labelClass}>
							{t('deal.form.provider_base_url', 'Base URL')}
						</label>
						<input
							id="pf-base-url"
							type="url"
							bind:value={pf.base_url}
							placeholder="https://example.com"
							class={inputClass}
						/>
					</div>
				</div>
				<div>
					<label for="pf-description" class={labelClass}>
						{t('deal.form.provider_description', 'Provider description')}
					</label>
					<textarea id="pf-description" bind:value={pf.description} rows={2} class={inputClass}
					></textarea>
				</div>
				<div class="flex flex-wrap items-center gap-4">
					<label class="flex items-center gap-2 text-sm text-[var(--color-text)]">
						<input
							type="checkbox"
							bind:checked={pf.is_active}
							class="h-4 w-4 rounded border-[var(--color-border)]"
						/>
						{t('deal.form.provider_active', 'Active')}
					</label>
					<label class="flex items-center gap-2 text-sm text-[var(--color-text)]">
						<input
							type="checkbox"
							bind:checked={pf.is_featured}
							class="h-4 w-4 rounded border-[var(--color-border)]"
						/>
						{t('deal.form.provider_featured', 'Featured')}
					</label>
				</div>

				{#if providerError}
					<div
						class="flex items-center gap-2 rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500"
						role="alert"
					>
						<AlertCircle size={16} /> {providerError}
					</div>
				{/if}

				<div class="flex justify-end gap-2">
					<button
						type="button"
						onclick={() => (editing = false)}
						disabled={providerSaving}
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
					>
						{t('common.cancel', 'Cancel')}
					</button>
					<button
						type="submit"
						disabled={providerSaving}
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
					>
						{#if providerSaving}
							<Loader2 size={16} class="animate-spin" />
						{/if}
						{providerSaving
							? t('common.submitting', 'Submitting…')
							: t('deal.form.submit_edit', 'Save changes')}
					</button>
				</div>
			</form>
		{/if}
	</div>

	<div class="p-4">
		{#if coupons.length === 0}
			<EmptyState
				icon={Ticket}
				title={t('deal.no_deals', 'No active deals from {name} right now.').replace(
					'{name}',
					provider.name
				)}
			/>
		{:else}
			<div class="grid gap-3 sm:grid-cols-2">
				{#each coupons as coupon (coupon.id)}
					<a
						href={dealPath(coupon)}
						class="group flex flex-col gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:border-[var(--color-primary)]/40 hover:bg-[var(--color-surface-subtle)]"
					>
						<div class="flex items-start justify-between gap-2">
							<div class="min-w-0">
								<h2
									class="truncate text-sm font-semibold text-[var(--color-text)] group-hover:text-[var(--color-primary)]"
								>
									{coupon.title}
								</h2>
							</div>
							{#if coupon.is_verified}
								<span
									class="inline-flex shrink-0 items-center gap-1 rounded-[var(--radius)] bg-[var(--color-surface-subtle)] px-1.5 py-0.5 text-xs font-medium text-[var(--color-primary)]"
								>
									<BadgeCheck size={12} />
									{t('coupons.verified', 'Verified')}
								</span>
							{/if}
						</div>

						<div class="flex items-center gap-2">
							{#if coupon.code}
								<code
									class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-2 py-1 font-mono text-xs text-[var(--color-text)]"
								>
									{coupon.code}
								</code>
							{/if}
							{#if discountLabel(coupon)}
								<span class="text-xs font-medium text-[var(--color-primary)]">
									{discountLabel(coupon)}
								</span>
							{/if}
						</div>

						<div class="flex items-center gap-2">
							{#if coupon.code}
								<button
									type="button"
									onclick={(e) => {
										e.preventDefault();
										e.stopPropagation();
										copyCode(coupon);
									}}
									class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-2.5 py-1.5 text-xs font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
								>
									<Copy size={12} />
									{t('coupons.copy', 'Copy code')}
								</button>
							{/if}
							{#if coupon.referral_url}
								<span
									class="inline-flex items-center gap-1.5 text-xs font-medium text-[var(--color-primary)]"
								>
									<ExternalLink size={12} />
									{t('coupons.visit', 'Visit')}
								</span>
							{/if}
						</div>
					</a>
				{/each}
			</div>
		{/if}
	</div>
</div>
