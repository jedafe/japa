<script lang="ts">
	import { untrack, onMount, onDestroy } from 'svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { Coins, ArrowRightLeft } from 'lucide-svelte';
	import { EmptyState, LoadingSkeleton } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import { formatMoney, currencySymbol } from '$lib/utils/currency';
	import type { FxCurrency, FxConvertResult } from '$lib/types';
	import type { PageData } from './$types';

	/**
	 * FX Currency Converter tool — §112, ADR-125.
	 * Any-to-any mid-market conversion: amount + from/to pickers → result card
	 * (converted amount, unit rate + inverse, fetched_at, stale warning).
	 * Picker codes come only from GET /fx/currencies (Rule 16 — no hardcoded
	 * currency lists). Conversion runs client-side, debounced per keystroke.
	 */
	let { data }: { data: PageData } = $props();

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

	const title = $derived(t('fx.title', 'Currency Converter'));
	const description = $derived(
		t('fx.subtitle', 'Live mid-market rates across 160+ currencies.')
	);
	const seoTitle = $derived(t('fx.seo_title', 'Currency Converter — {site}'));
	const seoDescription = $derived(
		t('fx.seo_description', 'Convert between 160+ currencies at live mid-market rates.')
	);

	const inverseRate = $derived(result && result.rate > 0 ? 1 / result.rate : null);

	function fxItem(c: FxCurrency): AutocompleteItem {
		return { id: c.code, label: `${c.code} — ${c.name}`, sublabel: c.symbol };
	}

	// Autocomplete fetcher — filters the SSR-loaded picker list client-side
	// (Rule 16 — the list itself comes from GET /fx/currencies).
	function fetchFxCurrencies(q: string): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase();
		const filtered =
			query === ''
				? currencies
				: currencies.filter(
						(c) =>
							c.code.toLowerCase().includes(query) ||
							c.name.toLowerCase().includes(query)
					);
		return Promise.resolve(filtered.map(fxItem));
	}
	function fetchFxCurrenciesInitial(): Promise<AutocompleteItem[]> {
		return fetchFxCurrencies('');
	}

	const fromLabel = $derived(
		from ? (currencies.find((c) => c.code === from) ? `${from} — ${currencies.find((c) => c.code === from)!.name}` : from) : null
	);
	const toLabel = $derived(
		to ? (currencies.find((c) => c.code === to) ? `${to} — ${currencies.find((c) => c.code === to)!.name}` : to) : null
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
	 *  hitting the API (no API call per keystroke). */
	function debouncedConvert() {
		if (timer) clearTimeout(timer);
		timer = setTimeout(() => convert(), 400);
	}

	function swap() {
		[from, to] = [to, from];
		convert();
	}

	function fmtMoney(value: number, currency: string): string {
		return formatMoney(value, currency, currencies, { fractionDigits: 2 });
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
	// after the component is gone (no leaked timers).
	onDestroy(() => {
		if (timer) clearTimeout(timer);
	});
</script>

<svelte:head>
	<title>{seoTitle}</title>
	<meta name="description" content={seoDescription} />
	<link rel="canonical" href={page.url.href} />
	<meta property="og:title" content={seoTitle} />
	<meta property="og:description" content={seoDescription} />
	<meta property="og:url" content={page.url.href} />
	<meta property="og:type" content="website" />
	<meta name="twitter:card" content="summary" />
	<meta name="twitter:title" content={seoTitle} />
	<meta name="twitter:description" content={seoDescription} />
</svelte:head>

<div class="mx-auto w-full max-w-3xl">
	<div class="mx-4 border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<Coins size={20} /> {title}
		</h1>
		<p class="mt-1 text-sm text-[var(--color-muted)]">{description}</p>
	</div>

	{#if currencies.length === 0}
		<div class="p-4">
			<EmptyState
				icon={Coins}
				title={t('fx.unavailable', 'Currency data unavailable')}
				body={t(
					'fx.unavailable_desc',
					"Live exchange rates haven't been configured yet. Please check back later."
				)}
			/>
		</div>
	{:else}
		<div class="p-4">
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 space-y-4">
				<!-- Amount -->
				<div>
					<label
						for="fx-amount"
						class="block text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]"
					>
						{t('fx.amount', 'Amount')}
					</label>
					<div class="mt-1 flex items-center gap-2">
						<input
							id="fx-amount"
							type="number"
							min="0"
							step="any"
							bind:value={amount}
							oninput={debouncedConvert}
							class="w-40 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]"
						/>
						<span class="text-sm text-[var(--color-muted)]">
							{currencySymbol(from, currencies) || from}
						</span>
					</div>
				</div>

				<!-- From / swap / to -->
				<div class="grid grid-cols-1 sm:grid-cols-[1fr_auto_1fr] gap-3 items-end">
					<div>
						<label
							for="fx-from"
							class="block text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]"
						>
							{t('fx.from', 'From')}
						</label>
						<div class="mt-1">
							<Autocomplete
								id="fx-from"
								placeholder={t('fx.from', 'From')}
								emptyMessage={t('common.no_results', 'No results found')}
								fetcher={fetchFxCurrencies}
								initialFetcher={fetchFxCurrenciesInitial}
								selectedId={from || null}
								selectedLabel={fromLabel}
								onSelect={(item) => {
									from = item.id;
									convert();
								}}
								onClear={() => (from = '')}
							/>
						</div>
					</div>
					<button
						type="button"
						onclick={swap}
						class="self-center rounded-[var(--radius)] border border-[var(--color-border)] p-2 text-[var(--color-muted)] transition-colors hover:text-[var(--color-text)] hover:border-[var(--color-text)]"
						aria-label={t('fx.swap', 'Swap currencies')}
					>
						<ArrowRightLeft size={18} />
					</button>
					<div>
						<label
							for="fx-to"
							class="block text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]"
						>
							{t('fx.to', 'To')}
						</label>
						<div class="mt-1">
							<Autocomplete
								id="fx-to"
								placeholder={t('fx.to', 'To')}
								emptyMessage={t('common.no_results', 'No results found')}
								fetcher={fetchFxCurrencies}
								initialFetcher={fetchFxCurrenciesInitial}
								selectedId={to || null}
								selectedLabel={toLabel}
								onSelect={(item) => {
									to = item.id;
									convert();
								}}
								onClear={() => (to = '')}
							/>
						</div>
					</div>
				</div>

				<!-- Result -->
				<div class="min-h-28 border-t border-[var(--color-border)] pt-3" aria-live="polite">
					{#if loading}
						<LoadingSkeleton height="56px" />
					{:else if convertError}
						<p class="py-4 text-sm text-red-500">{convertError}</p>
					{:else if result}
						<p class="text-2xl sm:text-3xl font-bold text-[var(--color-text)]">
							{fmtMoney(result.converted, result.to)}
						</p>
						<p class="mt-1 text-sm text-[var(--color-muted)]">
							{fmtMoney(result.amount, result.from)} =
						</p>
						<p class="mt-2 text-sm text-[var(--color-muted)]">
							1 {result.from} = {result.rate.toFixed(4)} {result.to}
							{#if inverseRate}
								· 1 {result.to} = {inverseRate.toFixed(4)} {result.from}
							{/if}
						</p>
						<p class="mt-1 text-xs text-[var(--color-muted)]">
							{t('fx.updated', 'Updated')} {new Date(result.fetched_at).toLocaleString()}
							{#if result.stale}
								<span class="text-yellow-600">· {t('fx.stale', 'rates may be stale')}</span>
							{/if}
						</p>
					{/if}
				</div>
			</div>

			<!-- OXR attribution — required by the free-tier licence (§112). -->
			<footer class="mt-6 text-center text-xs text-[var(--color-muted)]">
				<a
					href="https://openexchangerates.org"
					target="_blank"
					rel="noopener noreferrer"
					class="hover:text-[var(--color-text)] hover:underline"
				>
					{t('fx.attribution', 'Rates by Open Exchange Rates')}
				</a>
			</footer>
		</div>
	{/if}
</div>
