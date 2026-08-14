<script lang="ts">
	import { page } from '$app/state';
	import { t } from '$lib/i18n';
	import { Ticket, BadgeCheck, Copy, ExternalLink, Clock } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const coupon = $derived(data.coupon);

	const title = $derived(
		t('coupon.title', '{name} — JapaConnect').replace('{name}', coupon.title)
	);
	const description = $derived(
		coupon.description ??
			t('coupon.description', 'Coupon code and discount from {provider}.').replace(
				'{provider}',
				coupon.provider_name ?? ''
			)
	);

	const jsonLd = $derived(
		JSON.stringify({
			'@context': 'https://schema.org',
			'@type': 'Offer',
			name: coupon.title,
			description,
			url: page.url.href,
			...(coupon.referral_url ? { url: coupon.referral_url } : {})
		})
	);

	const discountLabel = $derived(
		coupon.discount_type === 'percent'
			? t('coupon.percent_off', '{n}% off').replace('{n}', String(coupon.discount_value ?? ''))
			: coupon.discount_type === 'fixed'
				? t('coupon.amount_off', '{cur}{n} off')
						.replace('{cur}', coupon.currency ?? '')
						.replace('{n}', String(coupon.discount_value ?? ''))
				: coupon.discount_type === 'free_shipping'
					? t('coupon.free_shipping', 'Free shipping')
					: coupon.discount_value !== undefined && coupon.discount_value !== null
						? String(coupon.discount_value)
						: ''
	);

	let copied = $state(false);

	// recordEvent fires-and-forgets a click/copy event to the backend (Section 55).
	// Errors are silently ignored — event tracking must never block the UI.
	function recordEvent(eventType: 'click' | 'copy') {
		api.post(`/coupons/${coupon.id}/click`, { event_type: eventType }).catch(() => {});
	}

	async function copyCode() {
		try {
			await navigator.clipboard.writeText(coupon.code);
			copied = true;
			recordEvent('copy');
			setTimeout(() => (copied = false), 2000);
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
				<Ticket size={24} />
			</span>
			<div class="min-w-0 flex-1">
				<h1 class="truncate text-xl font-semibold text-[var(--color-text)]">{coupon.title}</h1>
				{#if coupon.provider_name}
					<p class="truncate text-xs text-[var(--color-muted)]">{coupon.provider_name}</p>
				{/if}
			</div>
			{#if coupon.is_verified}
				<span
					class="inline-flex shrink-0 items-center gap-1 rounded-[var(--radius)] bg-[var(--color-surface-subtle)] px-2 py-1 text-xs font-medium text-[var(--color-primary)]"
				>
					<BadgeCheck size={14} />
					{t('coupon.verified', 'Verified')}
				</span>
			{/if}
		</div>
	</div>

	<div class="p-4">
		<div
			class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5"
		>
			<!-- Code + discount -->
			<div class="flex flex-wrap items-center gap-3">
				<code
					class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-2 font-mono text-lg font-semibold tracking-wide text-[var(--color-text)]"
				>
					{coupon.code}
				</code>
				{#if discountLabel}
					<span class="text-base font-semibold text-[var(--color-primary)]">{discountLabel}</span>
				{/if}
			</div>

			{#if coupon.description}
				<p class="mt-4 text-sm text-[var(--color-text-subtle)]">{coupon.description}</p>
			{/if}

			<!-- Provider + expiry -->
			<div class="mt-4 flex flex-wrap items-center gap-4 text-xs text-[var(--color-muted)]">
				{#if coupon.provider_name}
					<span>{coupon.provider_name}</span>
				{/if}
				{#if coupon.expires_at}
					<span class="inline-flex items-center gap-1">
						<Clock size={12} />
						{t('coupon.expires', 'Expires {date}').replace(
							'{date}',
							new Date(coupon.expires_at).toLocaleDateString()
						)}
					</span>
				{/if}
			</div>

			<!-- Actions -->
			<div class="mt-5 flex flex-wrap items-center gap-3">
				<button
					type="button"
					onclick={copyCode}
					class="inline-flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
				>
					<Copy size={16} />
					{copied ? t('coupon.copied', 'Copied!') : t('coupon.copy', 'Copy code')}
				</button>
				{#if coupon.referral_url}
					<a
						href={coupon.referral_url}
						target="_blank"
						rel="noopener noreferrer nofollow"
						onclick={() => recordEvent('click')}
						class="inline-flex items-center gap-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
					>
						<ExternalLink size={16} />
						{t('coupon.cta', 'Get this deal')}
					</a>
				{/if}
				{#if coupon.terms_url}
					<a
						href={coupon.terms_url}
						target="_blank"
						rel="noopener noreferrer"
						class="text-sm text-[var(--color-text-subtle)] underline transition-colors hover:text-[var(--color-text)]"
					>
						{t('coupon.terms', 'Terms & conditions')}
					</a>
				{/if}
			</div>
		</div>
	</div>
</div>
