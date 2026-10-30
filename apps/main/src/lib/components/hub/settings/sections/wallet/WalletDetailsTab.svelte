<script lang="ts">
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { invalidateAll } from '$app/navigation';
	import { setCurrencyPreference } from '$lib/api/hub';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import type { WalletData } from './types';

	interface Props {
		data: WalletData;
	}

	let { data }: Props = $props();

	const points = $derived(data.wallet?.points_balance ?? 0);
	const credits = $derived(data.wallet?.credits_balance ?? 0);
	const lifetimePoints = $derived(data.wallet?.lifetime_points ?? 0);
	const preferredCode = $derived(data.preferredCurrency?.preferred_currency ?? '');
	const currency = $derived(data.currencies.find((c) => c.code === preferredCode) ?? null);

	let saving = $state(false);
	let error = $state<string | null>(null);

	// Bound picker value — kept in sync with the server preference via the
	// effect below, so a failed save can revert the chip (an unbound value
	// would stay stuck on the attempted currency while showing an error).
	let choice = $state(untrack(() => preferredCode));
	$effect(() => {
		choice = preferredCode;
	});

	async function changeCurrency(code: string) {
		if (saving || code === preferredCode) return;
		saving = true;
		error = null;
		const res = await setCurrencyPreference(code);
		saving = false;
		if (res.error) {
			error = res.error.message;
			choice = preferredCode;
			return;
		}
		await invalidateAll();
	}
</script>

<div class="space-y-6">
	<div class="mb-6 grid grid-cols-1 gap-4 sm:grid-cols-3">
		<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<p class="text-xs text-[var(--color-muted)]">{t('settings.wallet.points')}</p>
			<p class="text-2xl font-semibold text-[var(--color-text)]">{points.toLocaleString()}</p>
		</div>
		<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<p class="text-xs text-[var(--color-muted)]">{t('settings.wallet.credits')}</p>
			<p class="text-2xl font-semibold text-[var(--color-text)]">{credits.toLocaleString()}</p>
		</div>
		<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<p class="text-xs text-[var(--color-muted)]">{t('settings.wallet.lifetime_points')}</p>
			<p class="text-2xl font-semibold text-[var(--color-text)]">{lifetimePoints.toLocaleString()}</p>
		</div>
	</div>


	{#if data.currencies.length > 0}
		<div>
			<label for="wallet-currency-select" class="mb-1.5 block text-sm text-[var(--color-muted)]">
				{t('settings.wallet.currency', 'Preferred currency')}
			</label>
			<AutocompleteSelect
				id="wallet-currency-select"
				bind:value={choice}
				disabled={saving}
				options={data.currencies.map((c) => ({
					value: c.code,
					label: c.symbol ? `${c.code} (${c.symbol})` : c.code,
					sublabel: c.name
				}))}
				placeholder={currency ? `${currency.symbol} ${currency.code}` : preferredCode || t('settings.wallet.currency_default', 'Default')}
				onValueChange={(v) => v && changeCurrency(v)}
			/>
			{#if error}<p class="mt-1 text-xs text-[var(--color-danger,#dc2626)]">{error}</p>{/if}
		</div>
	{/if}
</div>
