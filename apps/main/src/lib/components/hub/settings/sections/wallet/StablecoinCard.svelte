<script lang="ts">
	// §128 — minimal member stablecoin wallet card: address, status, and
	// the backed/bonus/pending balance tiers. Provision is member-driven;
	// signer activation is the recovery-approval ceremony the client wires
	// next (member_auth proof required). Top-up opens the provider-hosted
	// onramp checkout — fiat is collected there, USDC/USDT lands directly
	// at the member's address (no custody, no credits).
	import { invalidateAll } from '$app/navigation';
	import { t } from '#lib/i18n/index.js';
	import { provisionStablecoinWallet, createStablecoinOnramp, createIdemKeys } from '#lib/api/hub.js';
	import type { WalletData } from './types';

	interface Props {
		data: WalletData;
	}

	let { data }: Props = $props();

	const wallet = $derived(data.stablecoinWallet ?? null);
	const balance = $derived(wallet?.balance ?? null);
	let provisioning = $state(false);
	let provisionError = $state<string | null>(null);

	// Top-up state — the checkout runs provider-side; we just open it and
	// poll the summary until the deposit mirrors into `pending`/backed.
	let topupAsset = $state<'usdc' | 'usdt'>('usdc');
	let topupBusy = $state(false);
	let topupError = $state<string | null>(null);
	let topupAwaiting = $state(false);
	const idem = createIdemKeys();

	async function provision() {
		if (provisioning) return;
		provisioning = true;
		provisionError = null;
		const res = await provisionStablecoinWallet();
		provisioning = false;
		if (res.error) {
			provisionError = res.error.message;
			return;
		}
		await invalidateAll();
	}

	async function topup() {
		if (topupBusy) return;
		topupBusy = true;
		topupError = null;
		const fiat = data.preferredCurrency?.preferred_currency?.toLowerCase();
		const scope = `onramp|${topupAsset}|${fiat}`;
		const res = await createStablecoinOnramp({
			asset: topupAsset,
			fiat_currency: fiat
		}, idem.for(scope));
		topupBusy = false;
		if (res.error || !res.data?.url) {
			topupError = res.error?.message ?? t('settings.wallet.stablecoin_topup_unavailable', 'Top-up is unavailable right now.');
			return;
		}
		idem.drop(scope);
		window.open(res.data.url, '_blank', 'noopener');
		topupAwaiting = true;
	}
</script>

<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
	<div class="mb-2 flex items-center justify-between gap-2">
		<p class="text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">
			{t('settings.wallet.stablecoin_title', 'USDC / USDT wallet')}
		</p>
		{#if wallet?.status}
			<span class="rounded-full border border-[var(--color-border)] px-2 py-0.5 text-xs text-[var(--color-muted)]">
				{wallet.status}
			</span>
		{/if}
	</div>

	{#if wallet}
		{#if wallet.address}
			<p class="truncate font-mono text-xs text-[var(--color-muted)]" title={wallet.address}>{wallet.address}</p>
		{/if}
		<div class="mt-3 grid grid-cols-3 gap-3 text-sm">
			<div>
				<p class="text-xs text-[var(--color-muted)]">{t('settings.wallet.stablecoin_backed', 'Backed')}</p>
				<p class="font-semibold text-[var(--color-text)]">{(balance?.backed_credits ?? 0).toLocaleString()}</p>
			</div>
			<div>
				<p class="text-xs text-[var(--color-muted)]">{t('settings.wallet.stablecoin_bonus', 'Bonus')}</p>
				<p class="font-semibold text-[var(--color-text)]">{(balance?.bonus_credits ?? 0).toLocaleString()}</p>
			</div>
			<div>
				<p class="text-xs text-[var(--color-muted)]">{t('settings.wallet.stablecoin_pending', 'Pending')}</p>
				<p class="font-semibold text-[var(--color-text)]">{(balance?.pending ?? 0).toLocaleString()}</p>
			</div>
		</div>
		{#if wallet.needs_signer_rotation}
			<p class="mt-2 text-xs text-[var(--color-muted)]">
				{t('settings.wallet.stablecoin_rotation', 'Delegated signer scope is expiring — re-activation is required to keep spending seamless.')}
			</p>
		{/if}

		<!-- Top-up: fiat → USDC/USDT via the provider-hosted checkout. -->
		{#if wallet.status !== 'suspended'}
			<div class="mt-3 flex items-center gap-2">
				<select
					bind:value={topupAsset}
					class="rounded-[var(--radius-button,6px)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1.5 text-sm text-[var(--color-text)]"
					aria-label={t('settings.wallet.stablecoin_topup_asset', 'Top-up asset')}
				>
					<option value="usdc">USDC</option>
					<option value="usdt">USDT</option>
				</select>
				<button
					type="button"
					class="rounded-[var(--radius-button,6px)] border border-[var(--color-border)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-raised,transparent)] disabled:opacity-50"
					disabled={topupBusy}
					onclick={topup}
				>
					{topupBusy ? t('settings.wallet.stablecoin_topup_opening', 'Opening checkout…') : t('settings.wallet.stablecoin_topup', 'Top up')}
				</button>
				{#if topupAwaiting}
					<button
						type="button"
						class="text-xs text-[var(--color-muted)] underline underline-offset-2 hover:text-[var(--color-text)]"
						onclick={invalidateAll}
					>
						{t('settings.wallet.stablecoin_refresh', 'Refresh balance')}
					</button>
				{/if}
			</div>
			{#if topupAwaiting}
				<p class="mt-1 text-xs text-[var(--color-muted)]">
					{t('settings.wallet.stablecoin_topup_pending', 'Complete the checkout in the other tab — your deposit shows under Pending once it confirms onchain.')}
				</p>
			{/if}
			{#if topupError}
				<p class="mt-1 text-xs text-[var(--color-danger,#dc2626)]">{topupError}</p>
			{/if}
		{/if}
	{:else}
		<p class="text-sm text-[var(--color-muted)]">
			{t('settings.wallet.stablecoin_none', 'No stablecoin wallet yet. Provision one to hold USDC/USDT on Solana.')}
		</p>
		<button
			type="button"
			class="mt-3 rounded-[var(--radius-button,6px)] border border-[var(--color-border)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-raised,transparent)] disabled:opacity-50"
			disabled={provisioning}
			onclick={provision}
		>
			{provisioning ? t('settings.wallet.stablecoin_provisioning', 'Provisioning…') : t('settings.wallet.stablecoin_provision', 'Provision wallet')}
		</button>
		{#if provisionError}
			<p class="mt-1 text-xs text-[var(--color-danger,#dc2626)]">{provisionError}</p>
		{/if}
	{/if}
</div>
