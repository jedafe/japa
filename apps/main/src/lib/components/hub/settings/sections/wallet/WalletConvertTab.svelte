<script lang="ts">
	import { t } from '$lib/i18n';
	import { invalidateAll } from '$app/navigation';
	import { convertWallet } from '$lib/api/hub';
	import type { WalletData } from './types';

	interface Props {
		data: WalletData;
	}

	let { data }: Props = $props();

	const points = $derived(data.wallet?.points_balance ?? 0);
	const rate = $derived(data.settings?.settings?.points_to_credits_rate ?? 0);
	// null = settings endpoint unavailable → no client-side floor beyond
	// the conversion rate itself (backend still enforces the real minimum).
	const minConvert = $derived(data.minConvertPoints ?? 0);
	// Conversion must yield at least 1 credit, so the practical floor is the
	// larger of the admin minimum and the current rate.
	const effectiveMin = $derived(Math.max(minConvert, rate, 1));

	let amount = $state('');
	let converting = $state(false);
	let error = $state<string | null>(null);
	let success = $state(false);

	const estimatedCredits = $derived(() => {
		const n = parseInt(amount, 10);
		if (!n || n <= 0 || rate <= 0) return 0;
		return Math.floor(n / rate);
	});

	const canConvert = $derived(() => {
		const n = parseInt(amount, 10);
		return !converting && n >= effectiveMin && n <= points;
	});

	function userMessage(code: string | undefined, fallback: string): string {
		switch (code) {
			case 'insufficient_balance':
				return t('settings.wallet.error_insufficient_balance');
			case 'forbidden':
				return t('settings.wallet.error_forbidden');
			case 'validation_error':
				return t('settings.wallet.error_min_convert', 'Minimum {amount} points required for conversion.').replace('{amount}', effectiveMin.toLocaleString());
			default:
				return fallback;
		}
	}

	async function doConvert() {
		const pts = parseInt(amount, 10);
		if (!pts || pts <= 0) {
			error = t('wallet.convert.invalid_amount', 'Enter a positive amount');
			return;
		}
		if (pts < effectiveMin) {
			error = t('settings.wallet.error_min_convert', 'Minimum {amount} points required for conversion.').replace('{amount}', effectiveMin.toLocaleString());
			return;
		}
		if (pts > points) {
			error = t('settings.wallet.error_insufficient_balance');
			return;
		}
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
		amount = '';
		await invalidateAll();
	}
</script>

<div class="space-y-4">
	{#if rate > 0}
		<p class="text-sm text-[var(--color-muted)]">
			{t('wallet.convert.rate', 'Rate: {rate} points = 1 credit', { rate: rate.toLocaleString() })}
		</p>
		<p class="text-sm text-[var(--color-muted)]">
			{t('wallet.convert.minimum', 'Minimum: {effectiveMin} points', { effectiveMin: effectiveMin.toLocaleString() })}
		</p>
	{/if}

	<form onsubmit={(e) => { e.preventDefault(); doConvert(); }} class="space-y-4">
		<div>
			<label for="convert-amount" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
				{t('wallet.convert.amount', 'Points to convert')}
			</label>
			<input
				id="convert-amount"
				type="number"
				min={effectiveMin}
				max={points}
				step={1}
				bind:value={amount}
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
			/>
		</div>

		{#if estimatedCredits() > 0}
			<p class="text-sm text-[var(--color-text)]">
				{t('wallet.convert.estimate', 'Estimated credits')}: {estimatedCredits().toLocaleString()}
			</p>
		{/if}

		{#if error}<p class="text-sm text-[var(--color-danger)]">{error}</p>{/if}
		{#if success}<p class="text-sm text-[var(--color-success)]">{t('wallet.convert.success', 'Converted successfully')}</p>{/if}

		<button
			type="submit"
			disabled={!canConvert()}
			class="inline-flex items-center gap-2 rounded-lg bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
		>
			{converting ? t('wallet.convert.converting', 'Converting...') : t('wallet.convert.button', 'Convert')}
		</button>
	</form>
</div>
