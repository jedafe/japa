<script lang="ts">
	import { page } from '$app/state';
	import { t } from '$lib/i18n';
	import { Ticket, BadgeCheck, Copy, ExternalLink, Handshake } from 'lucide-svelte';
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

	// recordEvent fires-and-forgets a copy event to the backend (Section 55).
	// Errors are silently ignored — event tracking must never block the UI.
	function recordEvent(couponId: string, eventType: 'click' | 'copy') {
		api.post(`/coupons/${couponId}/click`, { event_type: eventType }).catch(() => {});
	}

	async function copyCode(coupon: Coupon) {
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
			href="/coupons"
			class="mb-2 inline-flex items-center gap-1.5 text-sm text-[var(--color-text-subtle)] transition-colors hover:text-[var(--color-text)]"
		>
			{t('coupon.back', 'All coupons')}
		</a>
		<div class="flex items-center gap-3">
			<span
				class="grid h-12 w-12 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-[var(--color-primary)]"
			>
				<Handshake size={24} />
			</span>
			<div class="min-w-0 flex-1">
				<h1 class="truncate text-xl font-semibold text-[var(--color-text)]">{provider.name}</h1>
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
		</div>
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
							<code
								class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-2 py-1 font-mono text-xs text-[var(--color-text)]"
							>
								{coupon.code}
							</code>
							{#if discountLabel(coupon)}
								<span class="text-xs font-medium text-[var(--color-primary)]">
									{discountLabel(coupon)}
								</span>
							{/if}
						</div>

						<div class="flex items-center gap-2">
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
