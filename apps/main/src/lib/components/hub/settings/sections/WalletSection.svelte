<script lang="ts">
	import { t } from '$lib/i18n';
	import { convertWallet } from '$lib/api/hub';
	import { invalidate } from '$app/navigation';

	interface WalletData {
		wallet: { points?: number; credits?: number };
		ledger: Array<{ id: string; type: string; amount?: number; description?: string; created_at?: string }>;
	}

	let { data }: { data: unknown } = $props();
	const walletData = $derived(data as WalletData | null);
	const points = $derived(walletData?.wallet?.points ?? 0);
	const credits = $derived(walletData?.wallet?.credits ?? 0);
	const ledger = $derived(walletData?.ledger ?? []);

	let convertAmount = $state('');
	let converting = $state(false);
	let error = $state<string | null>(null);
	let success = $state(false);

	async function doConvert() {
		const pts = parseInt(convertAmount, 10);
		if (!pts || pts <= 0) return;
		converting = true;
		error = null;
		success = false;
		const res = await convertWallet(pts);
		converting = false;
		if (res.error) {
			error = res.error.message;
			return;
		}
		success = true;
		convertAmount = '';
		await invalidate('/api/v1/me/wallet');
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

	<form class="mb-6" onsubmit={(e) => { e.preventDefault(); doConvert(); }}>
		<label class="mb-2 block">
			<span class="mb-1 block text-sm text-[var(--color-muted)]">{t('settings.wallet.convert_label')}</span>
			<input
				type="number"
				min="1"
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
				value={convertAmount}
				oninput={(e) => (convertAmount = e.currentTarget.value)}
			/>
		</label>
		{#if error}<p class="mb-2 text-xs text-[var(--color-danger,#dc2626)]">{error}</p>{/if}
		{#if success}<p class="mb-2 text-xs text-green-600">{t('settings.wallet.convert_success')}</p>{/if}
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
