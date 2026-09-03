<script lang="ts">
	import { t, i18n } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { invalidateAll } from '$app/navigation';
	import DashboardPanel from '$lib/components/hub/DashboardPanel.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { Wallet, Coins, Gift, ArrowRightLeft, ShoppingCart, Plus } from 'lucide-svelte';
	import type { WalletEntry } from './+page.server';

	let { data } = $props();

	// Local mutable copies so conversion can update the UI without a reload.
	// Sync from the server data on initial render / invalidate.
	let pointsBalance = $state(0);
	let creditsBalance = $state(0);
	let pointsEntries = $state([] as WalletEntry[]);
	let creditsEntries = $state([] as WalletEntry[]);
	let activeTab = $state<'points' | 'credits'>('points');

	$effect.pre(() => {
		pointsBalance = data.pointsBalance ?? 0;
		creditsBalance = data.creditsBalance ?? 0;
		pointsEntries = [...(data.pointsEntries ?? [])];
		creditsEntries = [...(data.creditsEntries ?? [])];
	});

	// Conversion form
	let convertAmount = $state('');
	let converting = $state(false);
	let convertError = $state('');
	let convertSuccess = $state('');

	const minConvert = $derived(data.minConvertPoints ?? 100);
	const rate = $derived(data.settings?.settings?.points_to_credits_rate ?? 0);
	// Conversion must yield at least 1 credit, so the practical floor is the
	// larger of the admin minimum and the current rate.
	const effectiveMin = $derived(Math.max(minConvert, rate));
	const estimatedCredits = $derived(() => {
		const n = parseInt(convertAmount, 10);
		if (!n || n <= 0 || rate <= 0) return 0;
		return Math.floor(n / rate);
	});
	const canConvert = $derived(() => {
		const n = parseInt(convertAmount, 10);
		return !converting && n >= effectiveMin && n <= pointsBalance;
	});

	function idempotencyKey(): string {
		if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) {
			return crypto.randomUUID();
		}
		return `${Date.now()}-${Math.random().toString(36).slice(2)}`;
	}

	function userMessage(code: string | undefined, fallback: string): string {
		switch (code) {
			case 'insufficient_balance':
				return t('wallet.convert.insufficient_points', 'Not enough points');
			case 'forbidden':
				return t('wallet.convert.forbidden', 'You do not have permission to convert points');
			case 'validation_error':
				return t('wallet.convert.min_amount', 'Minimum {effectiveMin} points required', {
					effectiveMin: effectiveMin.toLocaleString()
				});
			default:
				return fallback;
		}
	}

	async function convertPoints(e: SubmitEvent) {
		e.preventDefault();
		convertError = '';
		convertSuccess = '';
		const points = parseInt(convertAmount, 10);
		if (!points || points <= 0) {
			convertError = t('wallet.convert.invalid_amount', 'Enter a positive amount');
			return;
		}
		if (points < effectiveMin) {
			convertError = t('wallet.convert.min_amount', 'Minimum {effectiveMin} points required', {
				effectiveMin: effectiveMin.toLocaleString()
			});
			return;
		}
		if (points > pointsBalance) {
			convertError = t('wallet.convert.insufficient_points', 'Not enough points');
			return;
		}
		converting = true;
		try {
			const res = await api.post<{ points_balance: number; credits_balance: number }>(
				'/me/wallet/convert',
				{ points_to_convert: points },
				{ 'Idempotency-Key': idempotencyKey() }
			);
			if (res.error) {
				convertError = userMessage(res.error.code, res.error.message ?? t('wallet.convert.failed', 'Conversion failed'));
			} else if (res.data) {
				pointsBalance = res.data.points_balance;
				creditsBalance = res.data.credits_balance;
				convertAmount = '';
				convertSuccess = t('wallet.convert.success', 'Converted successfully');
				await invalidateAll();
			}
		} catch {
			convertError = t('wallet.convert.failed', 'Conversion failed');
		} finally {
			converting = false;
		}
	}

	function formatDate(iso: string): string {
		return new Date(iso).toLocaleDateString(i18n.locale);
	}

	const displayedEntries = $derived(activeTab === 'points' ? pointsEntries : creditsEntries);
</script>

<svelte:head>
	<title>{t('dashboard.nav.wallet', 'Wallet')} — {t('dashboard.title', 'Dashboard')}</title>
</svelte:head>

<section class="space-y-6">
	<header>
		<h1 class="text-lg font-medium text-[var(--color-text)]">{t('dashboard.nav.wallet', 'Wallet')}</h1>
	</header>

	<div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
		<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<div class="flex items-center gap-3">
				<span class="grid h-10 w-10 place-items-center rounded-full bg-[var(--color-primary)]/10 text-[var(--color-primary)]">
					<Wallet size={20} />
				</span>
				<div>
					<p class="text-xs text-[var(--color-muted)]">{t('wallet.points', 'Points')}</p>
					<p class="text-xl font-semibold text-[var(--color-text)]">{pointsBalance.toLocaleString()}</p>
				</div>
			</div>
		</div>

		<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<div class="flex items-center gap-3">
				<span class="grid h-10 w-10 place-items-center rounded-full bg-[var(--color-success)]/10 text-[var(--color-success)]">
					<Coins size={20} />
				</span>
				<div>
					<p class="text-xs text-[var(--color-muted)]">{t('wallet.credits', 'Credits')}</p>
					<p class="text-xl font-semibold text-[var(--color-text)]">{creditsBalance.toLocaleString()}</p>
				</div>
			</div>
		</div>

		<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<div class="flex items-center gap-3">
				<span class="grid h-10 w-10 place-items-center rounded-full bg-[var(--color-info)]/10 text-[var(--color-info)]">
					<Gift size={20} />
				</span>
				<div>
					<p class="text-xs text-[var(--color-muted)]">{t('wallet.lifetime_points', 'Lifetime Points')}</p>
					<p class="text-xl font-semibold text-[var(--color-text)]">{data.lifetimePoints?.toLocaleString() ?? 0}</p>
				</div>
			</div>
		</div>
	</div>

	<div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
		<DashboardPanel titleKey="wallet.convert.title" titleFallback="Convert">
			<form onsubmit={convertPoints} class="space-y-4">
				{#if rate > 0}
					<p class="text-sm text-[var(--color-muted)]">
						{t('wallet.convert.rate', 'Rate: {rate} points = 1 credit', { rate: rate.toLocaleString() })}
						<br />
						{t('wallet.convert.minimum', 'Minimum: {effectiveMin} points', { effectiveMin: effectiveMin.toLocaleString() })}
					</p>
				{/if}

				<div>
					<label for="convert-amount" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
						{t('wallet.convert.amount', 'Points to convert')}
					</label>
					<input
						id="convert-amount"
						type="number"
						min={effectiveMin}
						max={pointsBalance}
						step={1}
						bind:value={convertAmount}
						class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
					/>
				</div>

				{#if estimatedCredits() > 0}
					<p class="text-sm text-[var(--color-text)]">
						{t('wallet.convert.estimate', 'Estimated credits')}: {estimatedCredits().toLocaleString()}
					</p>
				{/if}

				{#if convertError}
					<p class="text-sm text-[var(--color-danger)]">{convertError}</p>
				{/if}
				{#if convertSuccess}
					<p class="text-sm text-[var(--color-success)]">{convertSuccess}</p>
				{/if}

				<div class="flex items-center gap-3">
					<button
						type="submit"
						disabled={!canConvert()}
						class="inline-flex items-center gap-2 rounded-lg bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
					>
						<ArrowRightLeft class="h-4 w-4" />
						{converting ? t('wallet.convert.converting', 'Converting...') : t('wallet.convert.button', 'Convert')}
					</button>
					<a
						href="/dashboard/wallet/purchase"
						class="inline-flex items-center gap-2 rounded-lg border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface)]"
					>
						<ShoppingCart class="h-4 w-4" />
						{t('wallet.purchase_credits', 'Purchase Credits')}
					</a>
				</div>
			</form>
		</DashboardPanel>

		<DashboardPanel titleKey="wallet.history.title" titleFallback="History">
			<div class="mb-3 flex gap-2">
				<button
					type="button"
					class="rounded-[var(--radius)] px-3 py-1.5 text-sm {activeTab === 'points'
						? 'bg-[var(--color-surface-subtle)] text-[var(--color-text)]'
						: 'text-[var(--color-muted)] hover:text-[var(--color-text)]'}"
					onclick={() => (activeTab = 'points')}
				>
					{t('wallet.points', 'Points')}
				</button>
				<button
					type="button"
					class="rounded-[var(--radius)] px-3 py-1.5 text-sm {activeTab === 'credits'
						? 'bg-[var(--color-surface-subtle)] text-[var(--color-text)]'
						: 'text-[var(--color-muted)] hover:text-[var(--color-text)]'}"
					onclick={() => (activeTab = 'credits')}
				>
					{t('wallet.credits', 'Credits')}
				</button>
			</div>

			{#if displayedEntries.length === 0}
				<EmptyState icon={Plus} title={t('wallet.history.empty', 'No transactions yet')} />
			{:else}
				<ul class="max-h-80 space-y-2 overflow-auto">
					{#each displayedEntries as entry (entry.id)}
						<li class="flex items-center justify-between rounded-[var(--radius)] bg-[var(--color-surface-subtle)] px-3 py-2 text-sm">
							<div class="min-w-0 flex-1">
								<p class="truncate text-[var(--color-text)]">
									{entry.description?.trim() || (entry.amount >= 0 ? t('wallet.earned', 'Earned') : t('wallet.spent', 'Spent'))}
								</p>
								<p class="text-xs text-[var(--color-muted)]">{formatDate(entry.created_at)}</p>
							</div>
							<span class="font-medium {entry.amount >= 0 ? 'text-[var(--color-success)]' : 'text-[var(--color-danger)]'}">
								{entry.amount >= 0 ? '+' : ''}{entry.amount.toLocaleString()}
							</span>
						</li>
					{/each}
				</ul>
			{/if}
		</DashboardPanel>
	</div>
</section>
