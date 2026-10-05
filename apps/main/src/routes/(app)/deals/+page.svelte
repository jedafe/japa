<script lang="ts">
	import { page } from '$app/state';
	import { t } from '$lib/i18n';
	import { Ticket, BadgeCheck, Copy, ExternalLink, Store } from 'lucide-svelte';
	import { EmptyState, ListPageBar } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import type { PageData } from './$types';
	import { safeJsonLdScript } from '$lib/utils/safeHtml';
	import { dealPath, type Coupon } from '$lib/types';
	import { currencySymbol } from '$lib/utils/currency';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('deals.title', 'Deals — JapaConnect'));
	const description = $derived(
		t('deals.description', 'Browse active deals and discount codes from JapaConnect partners.')
	);

	const jsonLd = $derived(
		JSON.stringify({
			'@context': 'https://schema.org',
			'@type': 'CollectionPage',
			name: 'Deals',
			description,
			url: page.url.href
		})
	);

	// Partner directory strip — unique providers among the listed deals,
	// deduplicated by provider_slug (coupon rows embed provider_* fields).
	const partners = $derived(
		[
			...new Map(
				data.coupons
					.filter((c) => c.provider_slug && c.provider_name)
					.map((c) => [
						c.provider_slug as string,
						{ slug: c.provider_slug as string, name: c.provider_name as string }
					])
			).values()
		].sort((a, b) => a.name.localeCompare(b.name))
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

	// recordEvent fires-and-forgets a click/copy event to the backend (Section 55).
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
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
	<link rel="canonical" href="{page.url.origin}/deals" />
	<meta name="twitter:card" content="summary" />
	<meta name="twitter:title" content={title} />
	<meta name="twitter:description" content={description} />
	{@html safeJsonLdScript(jsonLd)}
</svelte:head>

<div class="mx-auto w-full max-w-full" style="--page-gutter: 1rem">
	<ListPageBar title={t('deals.heading', 'Deals')} />

	<div class="p-4">
		{#if data.loadError}
			<p class="py-12 text-center text-[var(--color-text-subtle)]">
				{t('deals.load_error', 'Failed to load deals. Please try again later.')}
			</p>
		{:else if data.coupons.length === 0}
			<EmptyState icon={Ticket} title={t('deals.empty', 'No deals available right now.')} />
		{:else}
			{#if partners.length > 0}
				<nav aria-label={t('deals.partners', 'Deal partners')} class="mb-4">
					<h2
						class="mb-2 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-subtle)]"
					>
						<Store size={14} />
						{t('deals.partners', 'Deal partners')}
					</h2>
					<ul class="flex flex-wrap gap-2">
						{#each partners as partner (partner.slug)}
							<li>
								<a
									href="/deal/{partner.slug}"
									class="inline-flex items-center rounded-full border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] transition-colors hover:border-[var(--color-primary)]/40 hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-primary)]"
								>
									{partner.name}
								</a>
							</li>
						{/each}
					</ul>
				</nav>
			{/if}

			<div class="grid gap-3 sm:grid-cols-2">
				{#each data.coupons as coupon (coupon.id)}
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
								{#if coupon.provider_name}
									<p class="mt-0.5 truncate text-xs text-[var(--color-text-subtle)]">
										{coupon.provider_name}
									</p>
								{/if}
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
