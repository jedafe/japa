<script lang="ts">
	import { page } from '$app/state';
	import { t } from '$lib/i18n';
	import { Ticket, BadgeCheck, Copy, ExternalLink } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import type { PageData } from './$types';
	import type { Coupon } from '$lib/types';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('coupons.title', 'Coupons — JapaConnect'));
	const description = $derived(
		t('coupons.description', 'Browse active coupons and discount codes from JapaConnect partners.')
	);

	const jsonLd = $derived(
		JSON.stringify({
			'@context': 'https://schema.org',
			'@type': 'CollectionPage',
			name: 'Coupons',
			description,
			url: page.url.href
		})
	);

	function discountLabel(c: Coupon): string {
		if (c.discount_type === 'percent') {
			return t('coupons.percent_off', '{n}% off').replace('{n}', String(c.discount_value ?? ''));
		}
		if (c.discount_type === 'fixed' || c.discount_type === 'amount') {
			const cur = c.currency ?? '';
			return t('coupons.amount_off', '{cur}{n} off')
				.replace('{cur}', cur)
				.replace('{n}', String(c.discount_value ?? ''));
		}
		if (c.discount_value !== undefined && c.discount_value !== null) {
			return String(c.discount_value);
		}
		return '';
	}

	async function copyCode(code: string) {
		try {
			await navigator.clipboard.writeText(code);
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
	<meta name="twitter:card" content="summary" />
	<meta name="twitter:title" content={title} />
	<meta name="twitter:description" content={description} />
	{@html `<script type="application/ld+json">${jsonLd}</script>`}
</svelte:head>

<div class="mx-auto w-full max-w-full">
	<div class="border-b border-[var(--color-border)] px-4 py-4">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<Ticket size={20} /> {t('coupons.heading', 'Coupons')}
		</h1>
		<p class="mt-1 text-sm text-[var(--color-text-subtle)]">
			{t('coupons.subtitle', 'Active discount codes from our partners.')}
		</p>
	</div>

	<div class="p-4">
		{#if data.loadError}
			<p class="py-12 text-center text-[var(--color-text-subtle)]">
				{t('coupons.load_error', 'Failed to load coupons. Please try again later.')}
			</p>
		{:else if data.coupons.length === 0}
			<EmptyState icon={Ticket} title={t('coupons.empty', 'No coupons available right now.')} />
		{:else}
			<div class="grid gap-3 sm:grid-cols-2">
				{#each data.coupons as coupon (coupon.id)}
					<a
						href="/coupons/{coupon.id}"
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
									copyCode(coupon.code);
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
