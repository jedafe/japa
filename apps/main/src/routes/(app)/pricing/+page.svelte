<script lang="ts">
	import { t, i18n } from '$lib/i18n';
	import { Check, X } from 'lucide-svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	// Currency symbols from the API (Rule 16 — no hardcoded data).
	const currencySymbolMap = $derived(
		new Map((data.currencies ?? []).map((c: { code: string; symbol: string }) => [c.code, c.symbol]))
	);

	function formatPrice(amount: number, currency: string): string {
		if (amount === 0) return t('pricing.free', 'Free');
		const symbol = currencySymbolMap.get(currency) ?? currency;
		return `${symbol}${(amount / 100).toLocaleString(i18n.locale, { minimumFractionDigits: 0, maximumFractionDigits: 2 })}`;
	}
</script>

<svelte:head>
	<title>{t('pricing.title', 'Pricing — {site}')}</title>
	<meta name="description" content={t('pricing.description', 'Choose the plan that fits your journey.')} />
</svelte:head>

<div class="mx-auto max-w-3xl px-4 py-8">
	<header class="mb-8 text-center">
		<h1 class="text-2xl font-bold text-[var(--color-text)]">
			{t('pricing.heading', 'Choose your plan')}
		</h1>
		<p class="mt-2 text-sm text-[var(--color-muted)]">
			{t('pricing.description', 'Choose the plan that fits your journey.')}
		</p>
	</header>

	{#if data.plans.length === 0}
		<p class="text-center text-sm text-[var(--color-muted)]">{t('pricing.unavailable', 'Plans are temporarily unavailable. Please try again later.')}</p>
	{:else}
		<div class="grid gap-4 sm:grid-cols-3">
			{#each data.plans as plan (plan.id)}
				<div
					class="flex flex-col rounded-[var(--radius)] border p-6 {plan.highlighted
						? 'border-[var(--color-primary)] bg-[var(--color-surface)] ring-1 ring-[var(--color-primary)]'
						: 'border-[var(--color-border)] bg-[var(--color-surface)]'}"
				>
					{#if plan.highlighted}
						<span class="mb-3 inline-flex w-fit rounded-full bg-[var(--color-primary)] px-2.5 py-0.5 text-xs font-medium text-[var(--color-primary-contrast)]">
							{t('pricing.popular', 'Most popular')}
						</span>
					{/if}

					<div class="flex items-center gap-2">
						<h2 class="text-lg font-semibold text-[var(--color-text)]">{plan.name}</h2>
						<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 text-xs font-medium text-[var(--color-muted)]">
							{t(`pricing.tier.${plan.tier}`, plan.tier)}
						</span>
					</div>
					<p class="mt-1 text-sm text-[var(--color-muted)]">{plan.description}</p>

					<div class="mt-4">
						<span class="text-2xl font-bold text-[var(--color-text)]">{formatPrice(plan.monthly.amount, plan.monthly.currency)}</span>
						<span class="text-sm text-[var(--color-muted)]">/{t('pricing.month', 'mo')}</span>
					</div>

					<ul class="mt-6 flex-1 space-y-2.5">
						{#each plan.features as feature (feature.text)}
							<li class="flex items-start gap-2 text-sm">
								{#if feature.included}
									<Check class="mt-0.5 h-4 w-4 shrink-0 text-[var(--color-success)]" />
									<span class="text-[var(--color-text)]">{feature.text}</span>
								{:else}
									<X class="mt-0.5 h-4 w-4 shrink-0 text-[var(--color-muted)]" />
									<span class="text-[var(--color-muted)] line-through">{feature.text}</span>
								{/if}
							</li>
						{/each}
					</ul>

					<a
						href="/settings/subscription"
						class="mt-6 block rounded-[var(--radius)] px-4 py-2 text-center text-sm font-medium {plan.highlighted
							? 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)]'
							: 'border border-[var(--color-border)] text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]'}"
					>
						{plan.monthly.amount === 0
							? t('pricing.current_plan', 'Current plan')
							: t('pricing.choose', 'Choose {plan}').replace('{plan}', plan.name)}
					</a>
				</div>
			{/each}
		</div>
	{/if}
</div>
