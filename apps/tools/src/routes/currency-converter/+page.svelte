<script lang="ts">
	import { untrack, onMount, onDestroy } from 'svelte';
	import { api } from '$lib/api/client';
	import { buildSeoMeta } from '$lib/seo';
	import SeoHead from '$lib/components/seo/SeoHead.svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import Icon from '$lib/components/ui/Icon.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import LoadingSkeleton from '$lib/components/ui/LoadingSkeleton.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import ToolDisabled from '$lib/components/ui/ToolDisabled.svelte';
	import { formatMoney, currencySymbol } from '$lib/utils/currency';
	import type { FxCurrency, FxConvertResult } from '$lib/types';

	/**
	 * FX Currency Converter tool — §112, ADR-125.
	 * Any-to-any mid-market conversion: amount + from/to pickers → result card
	 * (converted amount, unit rate + inverse, fetched_at, stale warning).
	 * Picker codes come only from GET /fx/currencies (Rule 16 — no hardcoded
	 * currency lists). Conversion runs client-side, debounced per keystroke.
	 */
	let { data } = $props();

	// Picker list — this page's own `currencies` payload (GET /fx/currencies)
	// shadows the root layout's platform list; both share the {code,name,symbol}
	// shape formatMoney expects, and the FX list is the correct lookup for the
	// codes this page can actually convert.
	let currencies = $state<FxCurrency[]>(untrack(() => data.currencies));

	let amount = $state(1000);
	let from = $state(untrack(() => data.defaultFrom) || 'USD');
	let to = $state('NGN');
	let result = $state<FxConvertResult | null>(null);
	let loading = $state(false);
	let convertError = $state('');
	let timer: ReturnType<typeof setTimeout> | null = null;

	const seo = $derived(
		buildSeoMeta({
			title: t('fx.seo_title', 'Currency Converter — JapaConnect Tools'),
			description: t(
				'fx.seo_description',
				'Convert between 160+ currencies at live mid-market rates — including NGN, GHS, KES — powered by Open Exchange Rates.'
			),
			path: '/currency-converter',
			origin: page.url.origin,
			ogImage: `${page.url.origin}/og/og-currency-converter.png`,
			jsonLd: {
				'@context': 'https://schema.org',
				'@type': 'SoftwareApplication',
				name: 'Currency Converter',
				applicationCategory: 'UtilitiesApplication',
				operatingSystem: 'Web',
				offers: { '@type': 'Offer', price: '0', priceCurrency: 'USD' },
				description: 'Convert between 160+ currencies at live mid-market rates.'
			}
		})
	);

	const inverseRate = $derived(result && result.rate > 0 ? 1 / result.rate : null);

	const currencyOptions = $derived(
		currencies.map((c) => ({
			value: c.code,
			label: `${c.code} — ${c.name}`,
			sublabel: c.symbol
		}))
	);

	async function convert() {
		if (amount <= 0 || !from || !to || currencies.length === 0) return;
		loading = true;
		convertError = '';
		const res = await api.get<FxConvertResult>(
			`/fx/convert?amount=${amount}&from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`
		);
		loading = false;
		if (res.error) {
			// Backend codes: invalid_amount / invalid_currency /
			// unsupported_currency (400), fx_unavailable (503) — §112.
			convertError = res.error.message || t('fx.error', 'Conversion failed');
			result = null;
			return;
		}
		result = res.data;
	}

	/** Debounced convert — waits 400ms after the last keystroke before
	 *  hitting the API (§4.1 performance — no API call per keystroke). */
	function debouncedConvert() {
		if (timer) clearTimeout(timer);
		timer = setTimeout(() => convert(), 400);
	}

	function swap() {
		[from, to] = [to, from];
		convert();
	}

	function fmtMoney(amount: number, currency: string): string {
		return formatMoney(amount, currency, data.currencies, { fractionDigits: 2 });
	}

	// On mount: normalize the default pair against the fetched picker list
	// (e.g. the configured base code absent, or 'NGN' unsupported → fall back
	// to the first listed codes) and run the initial conversion. onMount —
	// not $effect — so it fires exactly once.
	onMount(() => {
		if (currencies.length === 0) return;
		const codes = new Set(currencies.map((c) => c.code));
		if (!codes.has(from)) from = currencies[0].code;
		if (!codes.has(to) || to === from) {
			to = currencies.find((c) => c.code !== from)?.code ?? from;
		}
		convert();
	});

	// Clean up the debounce timer on destroy so a pending callback can't fire
	// after the component is gone (§4.1 performance / no leaked timers).
	onDestroy(() => {
		if (timer) clearTimeout(timer);
	});
</script>

<SeoHead seo={seo} ogType="website" twitterCard="summary" />

<div class="mx-auto max-w-2xl px-4 py-8">
	{#if !data.enabled}
		<ToolDisabled tool={data.tool} />
	{:else}
		<div class="mb-8">
			<h1 class="text-2xl sm:text-3xl font-bold text-text">
				{t('fx.title', 'Currency Converter')}
			</h1>
			<p class="mt-2 text-text-subtle">
				{t('fx.subtitle', 'Live mid-market rates across 160+ currencies.')}
			</p>
		</div>

		{#if currencies.length === 0}
			<EmptyState
				icon="coins"
				title={t('fx.unavailable', 'Currency data unavailable')}
				description={t(
					'fx.unavailable_desc',
					"Live exchange rates haven't been configured yet. Please check back later."
				)}
			/>
		{:else}
			<div class="p-4 sm:p-6 bg-surface border border-border rounded-lg space-y-4">
				<!-- Amount -->
				<label class="block">
					<span class="block text-sm font-medium text-text mb-1.5">{t('fx.amount', 'Amount')}</span>
					<div class="flex items-center gap-2">
						<input
							type="number"
							min="0"
							step="any"
							bind:value={amount}
							oninput={debouncedConvert}
							class="w-40 px-3 py-2 bg-surface border border-border rounded text-text focus:border-primary focus:outline-none"
						/>
						<span class="text-sm text-muted">{currencySymbol(from, data.currencies) || from}</span>
					</div>
				</label>

				<!-- From / swap / to -->
				<div class="grid grid-cols-1 sm:grid-cols-[1fr_auto_1fr] gap-3 items-end">
					<label class="block">
						<span class="block text-sm font-medium text-text mb-1.5">{t('fx.from', 'From')}</span>
						<AutocompleteSelect
							options={currencyOptions}
							bind:value={from}
							onValueChange={() => convert()}
							ariaLabel={t('fx.from', 'From')}
						/>
					</label>
					<button
						onclick={swap}
						class="p-2 border border-border rounded-lg text-muted hover:text-text hover:border-text transition-colors"
						aria-label={t('fx.swap', 'Swap currencies')}
					>
						<Icon name="arrow-left-right" size={18} />
					</button>
					<label class="block">
						<span class="block text-sm font-medium text-text mb-1.5">{t('fx.to', 'To')}</span>
						<AutocompleteSelect
							options={currencyOptions}
							bind:value={to}
							onValueChange={() => convert()}
							ariaLabel={t('fx.to', 'To')}
						/>
					</label>
				</div>

				<!-- Result -->
				<div class="min-h-28 pt-2 border-t border-border" aria-live="polite">
					{#if loading}
						<LoadingSkeleton height="56px" />
					{:else if convertError}
						<p class="text-sm text-red-500 py-4">{convertError}</p>
					{:else if result}
						<p class="text-2xl sm:text-3xl font-bold text-text">
							{fmtMoney(result.converted, result.to)}
						</p>
						<p class="mt-1 text-sm text-text-subtle">
							{fmtMoney(result.amount, result.from)} =
						</p>
						<p class="mt-2 text-sm text-muted">
							1 {result.from} = {result.rate.toFixed(4)} {result.to}
							{#if inverseRate}
								· 1 {result.to} = {inverseRate.toFixed(4)} {result.from}
							{/if}
						</p>
						<p class="mt-1 text-xs text-muted">
							{t('fx.updated', 'Updated')} {new Date(result.fetched_at).toLocaleString()}
							{#if result.stale}
								<span class="text-yellow-600">· {t('fx.stale', 'rates may be stale')}</span>
							{/if}
						</p>
					{/if}
				</div>
			</div>

			<!-- OXR attribution — required by the free-tier licence (§112). -->
			<footer class="mt-6 text-center text-xs text-muted">
				<a
					href="https://openexchangerates.org"
					target="_blank"
					rel="noopener noreferrer"
					class="hover:text-text hover:underline"
				>
					{t('fx.attribution', 'Rates by Open Exchange Rates')}
				</a>
			</footer>
		{/if}
	{/if}
</div>
