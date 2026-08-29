<script lang="ts">
	import { Select } from 'bits-ui';
	import { t } from '$lib/i18n';
	import { convertWallet, setCurrencyPreference, listCreditPacks, purchaseCredits } from '$lib/api/hub';
	import { invalidateAll } from '$app/navigation';

	interface Currency {
		code: string;
		name?: string;
		symbol?: string;
	}

	interface CreditPack {
		id: string;
		name: string;
		description?: string | null;
		credit_amount: number;
		bonus_credits?: number;
	}

	interface WalletData {
		wallet: { points?: number; credits?: number };
		ledger: Array<{ id: string; type: string; amount?: number; description?: string; created_at?: string }>;
		settings?: {
			settings?: { points_to_credits_rate?: number };
			min_convert_points?: number;
		};
		currencies?: Currency[];
		creditPacks?: CreditPack[];
		preferredCurrency?: { preferred_currency?: string } | null;
	}

	let { data }: { data: unknown } = $props();
	const walletData = $derived(data as WalletData | null);
	const points = $derived(walletData?.wallet?.points ?? 0);
	const credits = $derived(walletData?.wallet?.credits ?? 0);
	const ledger = $derived(walletData?.ledger ?? []);
	const rate = $derived(walletData?.settings?.settings?.points_to_credits_rate ?? 100);
	const minConvert = $derived(walletData?.settings?.min_convert_points ?? 100);
	// Effective floor: conversion can never yield fewer than 1 credit, so the
	// minimum the user can actually convert is max(min_convert_points, rate).
	const effectiveMin = $derived(Math.max(minConvert, rate));

	let convertAmount = $state('');
	let converting = $state(false);
	let error = $state<string | null>(null);
	let success = $state(false);

	const currencies = $derived(walletData?.currencies ?? []);
	const preferredCurrency = $derived(walletData?.preferredCurrency?.preferred_currency ?? '');
	let currencySaving = $state(false);
	let currencyError = $state<string | null>(null);

	const creditPacks = $derived(walletData?.creditPacks ?? []);
	let purchasingPack = $state<string | null>(null);
	let purchaseError = $state<string | null>(null);

	async function changeCurrency(code: string) {
		if (currencySaving || code === preferredCurrency) return;
		currencySaving = true;
		currencyError = null;
		const res = await setCurrencyPreference(code);
		currencySaving = false;
		if (res.error) {
			currencyError = res.error.message;
			return;
		}
		await invalidateAll();
	}

	async function buyPack(pack: CreditPack) {
		if (purchasingPack) return;
		purchasingPack = pack.id;
		purchaseError = null;
		const res = await purchaseCredits({
			pack_id: pack.id,
			currency: preferredCurrency,
			provider: 'paystack'
		});
		purchasingPack = null;
		if (res.error) {
			purchaseError = res.error.message;
			return;
		}
		if (res.data?.checkout_url) {
			window.location.href = res.data.checkout_url;
			return;
		}
		// No checkout URL — re-run the loader to show new balance.
		await invalidateAll();
	}

	function userMessage(code: string | undefined, fallback: string): string {
		switch (code) {
			case 'insufficient_balance':
				return t('settings.wallet.error_insufficient_balance');
			case 'internal_error':
				return t('settings.wallet.error_internal');
			case 'forbidden':
				return t('settings.wallet.error_forbidden');
			default:
				return fallback;
		}
	}

	async function doConvert() {
		const pts = parseInt(convertAmount, 10);
		if (!pts || pts <= 0) return;
		converting = true;
		error = null;
		success = false;
		const res = await convertWallet(pts);
		converting = false;
		if (res.error) {
			error = userMessage(res.error.code, res.error.message);
			return;
		}
		success = true;
		convertAmount = '';
		await invalidateAll();
	}
</script>

<section class="max-w-2xl">
	<header class="mb-4">
		<h1 class="text-lg font-medium text-[var(--color-text)]">{t('settings.nav.wallet')}</h1>
		<p class="text-sm text-[var(--color-muted)]">{t('settings.wallet.desc')}</p>
	</header>

	{#if currencies.length > 0}
		{@const currencySelectId = 'wallet-currency-select'}
		<div class="mb-4">
			<label for={currencySelectId} class="mb-1.5 block text-sm text-[var(--color-muted)]">{t('settings.wallet.currency', 'Preferred currency')}</label>
			<Select.Root type="single" value={preferredCurrency} onValueChange={(v) => v && changeCurrency(v)}>
				<Select.Trigger
					id={currencySelectId}
					class="inline-flex w-full items-center justify-between rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
					aria-label={t('settings.wallet.currency', 'Preferred currency')}
				>
					{(currencies.find((c) => c.code === preferredCurrency)?.symbol ?? preferredCurrency) || t('settings.wallet.currency_default', 'Default')}
				</Select.Trigger>
				<Select.Portal>
					<Select.Content class="z-50 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-1 shadow">
						<Select.Viewport>
							{#each currencies as currency (currency.code)}
								<Select.Item
									value={currency.code}
									label={currency.code}
									class="cursor-pointer rounded px-2 py-1.5 text-sm text-[var(--color-text)] data-[highlighted]:bg-[var(--color-surface-subtle)]"
								>
									{#snippet children({ selected })}
										{selected ? '✓ ' : ''}{currency.symbol ? `${currency.code} (${currency.symbol})` : currency.code}
									{/snippet}
								</Select.Item>
							{/each}
						</Select.Viewport>
					</Select.Content>
				</Select.Portal>
			</Select.Root>
			{#if currencyError}<p class="mt-1 text-xs text-[var(--color-danger,#dc2626)]">{currencyError}</p>{/if}
		</div>
	{/if}

	<div class="mb-6 grid grid-cols-2 gap-4">
		<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<p class="text-xs text-[var(--color-muted)]">{t('settings.wallet.points')}</p>
			<p class="text-2xl font-medium text-[var(--color-text)]">{Math.round(points)}</p>
		</div>
		<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<p class="text-xs text-[var(--color-muted)]">{t('settings.wallet.credits')}</p>
			<p class="text-2xl font-medium text-[var(--color-text)]">{Math.round(credits)}</p>
		</div>
	</div>

	<p class="mb-4 text-xs text-[var(--color-muted)]">
		{t('settings.wallet.conversion_rate', '1 credit = {rate} points').replace('{rate}', String(rate))}
	</p>

	<form class="mb-6" onsubmit={(e) => { e.preventDefault(); doConvert(); }}>
		<label class="mb-2 block">
			<span class="mb-1 block text-sm text-[var(--color-muted)]">{t('settings.wallet.convert_label')}</span>
			<span class="mb-1 block text-xs text-[var(--color-muted)]">
				{t('settings.wallet.minimum', 'Minimum {minConvert} points').replace('{minConvert}', String(effectiveMin))}
			</span>
			<input
				type="number"
				min={effectiveMin}
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
				value={convertAmount}
				oninput={(e) => {
					convertAmount = e.currentTarget.value;
					success = false;
					error = null;
				}}
			/>
		</label>
		{#if error}<p class="mb-2 text-xs text-[var(--color-danger)]">{error}</p>{/if}
		{#if success}<p class="mb-2 text-xs text-[var(--color-success)]">{t('settings.wallet.convert_success')}</p>{/if}
		<button
			type="submit"
			disabled={converting || !convertAmount}
			class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
		>
			{converting ? t('settings.wallet.converting') : t('settings.wallet.convert')}
		</button>
	</form>

	{#if creditPacks.length > 0}
		<h2 class="mb-2 text-sm font-medium text-[var(--color-text)]">{t('settings.wallet.credit_packs', 'Buy credits')}</h2>
		<ul class="mb-6 space-y-2">
			{#each creditPacks as pack (pack.id)}
				<li class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
					<div class="flex items-center justify-between gap-3">
						<div>
							<p class="text-sm font-medium text-[var(--color-text)]">{pack.name}</p>
							<p class="text-xs text-[var(--color-muted)]">{pack.description ?? ''}</p>
						</div>
						<div class="shrink-0 text-right">
							<p class="text-sm text-[var(--color-text)]">{pack.credit_amount} {t('settings.wallet.credits')}</p>
							{#if pack.bonus_credits}<p class="text-xs text-[var(--color-success)]">+{pack.bonus_credits} {t('settings.wallet.bonus_credits', 'bonus')}</p>{/if}
						</div>
						<button
							type="button"
							disabled={purchasingPack === pack.id}
							class="shrink-0 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-white disabled:opacity-50"
							onclick={() => buyPack(pack)}
						>
							{purchasingPack === pack.id ? t('settings.wallet.purchasing', '...') : t('settings.wallet.buy', 'Buy')}
						</button>
					</div>
				</li>
			{/each}
		</ul>
		{#if purchaseError}<p class="mb-4 text-xs text-[var(--color-danger,#dc2626)]">{purchaseError}</p>{/if}
	{/if}

	{#if ledger.length > 0}
		<h2 class="mb-2 text-sm font-medium text-[var(--color-text)]">{t('settings.wallet.ledger')}</h2>
		<table class="w-full text-sm">
			<thead>
				<tr class="border-b border-[var(--color-border)] text-left text-xs text-[var(--color-muted)]">
					<th class="py-2 pr-4">{t('settings.wallet.ledger_type')}</th>
					<th class="py-2 pr-4">{t('settings.wallet.ledger_amount')}</th>
					<th class="py-2 pr-4">{t('settings.wallet.ledger_desc')}</th>
					<th class="py-2">{t('settings.wallet.ledger_date')}</th>
				</tr>
			</thead>
			<tbody>
				{#each ledger as entry (entry.id)}
					<tr class="border-b border-[var(--color-border)]">
						<td class="py-2 pr-4 text-[var(--color-text)]">{entry.type}</td>
						<td class="py-2 pr-4 text-[var(--color-text)]">{entry.amount ?? '—'}</td>
						<td class="py-2 pr-4 text-[var(--color-muted)]">{entry.description ?? '—'}</td>
						<td class="py-2 text-[var(--color-muted)]">{entry.created_at ?? '—'}</td>
					</tr>
				{/each}
			</tbody>
		</table>
	{/if}
</section>
