<script lang="ts">
	import { Select } from 'bits-ui';
	import { t } from '$lib/i18n';
	import { invalidateAll } from '$app/navigation';
	import { setCurrencyPreference } from '$lib/api/hub';
	import type { WalletData } from './types';

	interface Props {
		data: WalletData;
	}

	let { data }: Props = $props();

	const points = $derived(data.wallet?.points_balance ?? 0);
	const credits = $derived(data.wallet?.credits_balance ?? 0);
	const lifetimePoints = $derived(data.wallet?.lifetime_points ?? 0);
	const rate = $derived(data.settings?.settings?.points_to_credits_rate ?? 0);
	const preferredCode = $derived(data.preferredCurrency?.preferred_currency ?? '');
	const currency = $derived(data.currencies.find((c) => c.code === preferredCode) ?? null);

	let saving = $state(false);
	let error = $state<string | null>(null);

	async function changeCurrency(code: string) {
		if (saving || code === preferredCode) return;
		saving = true;
		error = null;
		const res = await setCurrencyPreference(code);
		saving = false;
		if (res.error) {
			error = res.error.message;
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

	{#if rate > 0}
		<p class="text-sm text-[var(--color-muted)]">
			{t('settings.wallet.conversion_rate', '1 credit = {rate} points').replace('{rate}', rate.toLocaleString())}
		</p>
	{/if}

	{#if data.currencies.length > 0}
		<div>
			<label for="wallet-currency-select" class="mb-1.5 block text-sm text-[var(--color-muted)]">
				{t('settings.wallet.currency', 'Preferred currency')}
			</label>
			<Select.Root type="single" value={preferredCode} onValueChange={(v) => v && changeCurrency(v)}>
				<Select.Trigger
					id="wallet-currency-select"
					class="inline-flex w-full items-center justify-between rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
				>
					{currency ? `${currency.symbol} ${currency.code}` : preferredCode || t('settings.wallet.currency_default', 'Default')}
				</Select.Trigger>
				<Select.Portal>
					<Select.Content class="z-50 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-1 shadow">
						<Select.Viewport>
							{#each data.currencies as c (c.code)}
								<Select.Item
									value={c.code}
									label={c.code}
									class="cursor-pointer rounded px-2 py-1.5 text-sm text-[var(--color-text)] data-[highlighted]:bg-[var(--color-surface-subtle)]"
								>
									{#snippet children({ selected })}
										{selected ? '✓ ' : ''}{c.symbol ? `${c.code} (${c.symbol})` : c.code}
									{/snippet}
								</Select.Item>
							{/each}
						</Select.Viewport>
					</Select.Content>
				</Select.Portal>
			</Select.Root>
			{#if error}<p class="mt-1 text-xs text-[var(--color-danger,#dc2626)]">{error}</p>{/if}
		</div>
	{/if}
</div>
