<script lang="ts">
	import { t } from '$lib/i18n';
	import { convertWallet } from '$lib/api/hub';
	import { invalidateAll } from '$app/navigation';

	interface WalletData {
		wallet: { points?: number; credits?: number };
		ledger: Array<{ id: string; type: string; amount?: number; description?: string; created_at?: string }>;
		settings?: {
			settings?: { points_to_credits_rate?: number };
			min_convert_points?: number;
		};
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
