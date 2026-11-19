<script lang="ts">
	// TipDialog (§125) — send credits/fiat tips to a user, org, or the
	// platform. Fiat methods redirect to the provider checkout URL returned
	// by POST /tips; a fiat result without a checkout URL (manual_transfer)
	// shows a pending-instructions state. External links and Open Collective
	// render as outbound CTAs per the public tip-config.
	//
	// Ref: japaconnect-engineering/sections/125-donations-tips.md
	import { page } from '$app/state';
	import { t } from '#lib/i18n/index.js';
	import { Modal } from '#lib/components/ui/index.js';
	import { sendTip, newIdempotencyKey, fiatExponent, fiatMajor, getWalletBalance, getContentTipSummary, type TipConfig, type TipSummary } from '#lib/api/tips.js';
	import { getAccessToken } from '#lib/api/token.svelte.js';
	import { AlertCircle, CheckCircle2, Clock, Coins, CreditCard, ExternalLink, Heart, Loader2, LogIn } from 'lucide-svelte';

	interface Props {
		open: boolean;
		recipientType: 'user' | 'organization' | 'platform' | 'space';
		recipientId?: string; // uuid for user/org
		contentId?: string;
		recipientName: string;
		config: TipConfig;
		onclose?: () => void;
	}
	let { open = $bindable(), recipientType, recipientId, contentId, recipientName, config, onclose }: Props = $props();

	let method = $state('credits');
	let amount = $state('5');
	let message = $state('');
	let isAnonymous = $state(false);
	let submitting = $state(false);
	let error = $state<string | null>(null);
	let succeeded = $state(false);
	let pendingTransfer = $state(false);
	let idemKey = newIdempotencyKey();
	let balance = $state<{ credits?: number }>({});
	// §125 tip-summary — the public aggregate for the tipped content item
	// (no sender identity; is_anonymous donors still count toward it).
	let summary = $state<TipSummary | null>(null);

	const authed = $derived(!!page.data.isAuthed);
	const returnUrl = $derived(`/login?redirect=${encodeURIComponent(page.url.pathname)}`);

	// Credits-denominated methods settle from a wallet rail (credits ledger
	// or the §128 stablecoin rail) — raw credit amounts, no currency. Fiat
	// provider methods convert major→minor; opencollective renders as an
	// outbound link, never a selectable method.
	const WALLET_METHODS = ['credits', 'stablecoin'];
	const methods = $derived(config.methods ?? []);
	const internal = $derived(methods.filter((m) => WALLET_METHODS.includes(m)));
	const fiat = $derived(methods.filter((m) => !WALLET_METHODS.includes(m) && m !== 'opencollective'));
	const sendable = $derived(internal.length + fiat.length > 0);

	const currency = $derived(config.fiat_currency ?? 'USD');
	const isFiat = $derived(fiat.includes(method));
	// ISO-4217 minor-unit exponents — shared helper mirrors the backend's
	// currencyExponent() so the input stays in major units.
	const exponent = $derived(fiatExponent(currency));
	const unit = $derived(isFiat ? currency : 'credits');

	// §125 frontend spec — amount presets + custom input. Fiat presets come
	// from the public config (minor units, currency-aware) so a weak-
	// currency recipient doesn't offer amounts that always fail the
	// credit-equivalent minimum; wallet methods keep stable credit presets.
	const presets = $derived(
		isFiat ? (config.fiat_presets ?? []).map((minor) => fiatMajor(minor, currency)) : [5, 10, 25, 50]
	);
	// Effective minimum in the method's own units — for fiat that is the
	// stricter of the recipient minimum and the platform currency floor.
	const minMethodUnits = $derived(
		isFiat ? Math.max(config.min_tip_amount ?? 0, config.fiat_min_minor ?? 0) : (config.min_tip_amount ?? 0)
	);
	const minMajor = $derived(
		minMethodUnits ? (isFiat ? minMethodUnits / Math.pow(10, exponent) : minMethodUnits) : 0
	);
	const balanceHint = $derived(
		method === 'credits' && balance.credits != null
			? t('tips.balance_credits', 'Balance: {n} cr').replace('{n}', String(balance.credits))
			: ''
	);

	const providerLabels: Record<string, string> = {
		stripe: 'Stripe',
		paystack: 'Paystack',
		nowpayments: 'Crypto',
		manual_transfer: ''
	};

	$effect(() => {
		if (open) {
			method = internal[0] ?? fiat[0] ?? 'credits';
			amount = '5';
			message = '';
			isAnonymous = false;
			error = null;
			succeeded = false;
			pendingTransfer = false;
			idemKey = newIdempotencyKey();
			summary = null;
			if (contentId) {
				getContentTipSummary(contentId).then((res) => {
					if (!res.error && res.data && res.data.tip_count > 0) summary = res.data;
				});
			}
			if (authed) {
				getWalletBalance().then((res) => {
					if (!res.error && res.data) {
						balance = { credits: res.data.credits_balance };
					}
				});
			}
		}
	});

	async function submit() {
		const raw = Number(amount);
		if (!raw || raw <= 0) {
			error = t('tips.invalid_amount', 'Enter a valid amount');
			return;
		}
		// Fiat: convert major units → minor units. Math.round (not floor) —
		// 9.99*100 is 998.9999… in float64, floor would shave a unit off.
		const amt = isFiat ? Math.round(raw * Math.pow(10, exponent)) : Math.floor(raw);
		if (amt <= 0) {
			error = t('tips.invalid_amount', 'Enter a valid amount');
			return;
		}
		if (minMethodUnits && amt < minMethodUnits) {
			error = t('tips.below_minimum', 'Below the recipient minimum');
			return;
		}
		submitting = true;
		error = null;
		const res = await sendTip(
			{
				recipient_type: recipientType,
				recipient_id: recipientId || undefined,
				content_id: contentId || undefined,
				method,
				amount: amt,
				currency: isFiat ? currency : undefined,
				message: message || undefined,
				is_anonymous: isAnonymous
			},
			idemKey
		);
		submitting = false;
		if (res.error || !res.data) {
			error = res.error?.message ?? t('tips.failed', 'Tip failed — try again');
			return;
		}
		// Redirect only when the tip is still awaiting payment — a replayed
		// completed tip can carry a stored checkout_url from the first attempt.
		if (res.data.checkout_url && res.data.tip?.status === 'pending') {
			window.location.href = res.data.checkout_url; // provider checkout
			return;
		}
		if (res.data.tip?.status === 'failed' || res.data.tip?.status === 'refunded') {
			// Dead replay — rotate the key so the next attempt is a fresh tip.
			idemKey = newIdempotencyKey();
			error = t('tips.failed', 'Tip failed — try again');
			return;
		}
		if (isFiat) {
			// Fiat method with no checkout URL (manual_transfer): the payment
			// is pending off-platform settlement — show instructions state.
			pendingTransfer = true;
		} else {
			succeeded = true;
		}
		idemKey = newIdempotencyKey();
	}

	$effect(() => {
		if (!open) onclose?.();
	});
</script>

{#snippet tipSummary()}
	{#if summary}
		<p class="mb-3 flex items-center gap-1.5 text-xs text-muted-foreground">
			<Heart class="size-3.5 shrink-0" />
			{t('tips.content_summary', '{n} supporter(s) · {credits} credits')
				.replace('{n}', String(summary.tip_count))
				.replace('{credits}', String(summary.credits_total))}
		</p>
	{/if}
{/snippet}

<Modal bind:open title="{t('tips.title')} — {recipientName}">
	{#if succeeded}
		<div class="flex flex-col items-center gap-3 py-6 text-center">
			<CheckCircle2 class="size-10 text-green-500" />
			<p class="font-medium">{config.thank_you_message || t('tips.thanks')}</p>
		</div>
	{:else if pendingTransfer}
		<div class="flex flex-col items-center gap-3 py-6 text-center">
			<Clock class="size-10 text-amber-500" />
			<p class="font-medium">{t('tips.transfer_pending', 'Tip created — complete the payment to settle it.')}</p>
			<p class="text-sm text-muted-foreground">{t('tips.transfer_pending_hint', 'The tip is recorded once the payment is confirmed.')}</p>
		</div>
	{:else if !authed}
		<!-- Guests can see the dialog surface but must sign in to transact —
		     POST /tips requires auth (BUG-014). -->
		{@render tipSummary()}
		<div class="flex flex-col items-center gap-3 py-6 text-center">
			<LogIn class="size-10 text-[var(--color-primary-text)]" />
			<p class="text-sm text-muted-foreground">{t('tips.sign_in_hint', 'Sign in to send a tip.')}</p>
			<a href={returnUrl}
				class="inline-flex items-center gap-2 rounded-lg bg-primary px-6 py-2.5 text-sm font-semibold text-primary-foreground transition hover:bg-primary/90">
				{t('tips.sign_in', 'Sign in')}
			</a>
		</div>
	{:else}
		{@render tipSummary()}
		{#if error}
			<div class="mb-3 flex items-center gap-2 rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">
				<AlertCircle class="size-4" />{error}
			</div>
		{/if}

		{#if internal.length > 0}
			<p class="mb-1 text-xs font-semibold uppercase text-muted-foreground">{t('tips.with_wallet')}</p>
			<div class="mb-4 grid grid-cols-2 gap-2">
				{#each internal as m}
					<button type="button" onclick={() => (method = m)}
						class="flex items-center gap-2 rounded-lg border px-3 py-2 text-sm font-medium transition {method === m ? 'border-primary bg-primary/5' : 'border-border hover:bg-muted'}">
						<Coins class="size-4" />{m === 'stablecoin' ? t('tips.stablecoin', 'Stablecoin (onchain)') : t('tips.credits')}
					</button>
				{/each}
			</div>
		{/if}

		{#if fiat.length > 0}
			<p class="mb-1 text-xs font-semibold uppercase text-muted-foreground">{t('tips.with_card')}</p>
			<div class="mb-4 grid grid-cols-2 gap-2">
				{#each fiat as m}
					<button type="button" onclick={() => (method = m)}
						class="flex items-center gap-2 rounded-lg border px-3 py-2 text-sm font-medium capitalize transition {method === m ? 'border-primary bg-primary/5' : 'border-border hover:bg-muted'}">
						<CreditCard class="size-4" />{m === 'manual_transfer' ? t('tips.bank_transfer') : (providerLabels[m] ?? m)}
					</button>
				{/each}
			</div>
		{/if}

		{#if sendable}
			<div class="mb-2 flex flex-wrap gap-2">
				{#each presets as p}
					<button type="button" onclick={() => (amount = String(p))}
						class="rounded-full border px-3 py-1 text-xs font-medium transition {Number(amount) === p ? 'border-primary bg-primary/5' : 'border-border hover:bg-muted'}">
						{p} {unit}
					</button>
				{/each}
			</div>
			<label class="mb-3 block">
				<span class="mb-1 flex items-center justify-between text-sm font-medium">
					<span>
						{t('tips.amount')} ({unit})
						{#if minMajor > 0}
							<span class="font-normal text-muted-foreground">
								— min {minMajor} {unit}
							</span>
						{/if}
					</span>
					{#if balanceHint}
						<span class="text-xs font-normal text-muted-foreground">{balanceHint}</span>
					{/if}
				</span>
				<input bind:value={amount} type="number" min="0" step={isFiat && exponent > 0 ? '0.01' : '1'} inputmode="decimal"
					class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm" />
			</label>
			<label class="mb-3 block">
				<span class="mb-1 block text-sm font-medium">{t('tips.message_label')}</span>
				<textarea bind:value={message} rows="2" maxlength="280" placeholder={t('tips.message_placeholder')}
					class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm"></textarea>
			</label>
			<label class="mb-1 flex items-center gap-2 text-sm">
				<input bind:checked={isAnonymous} type="checkbox" class="size-4 rounded border-border" />
				{t('tips.anonymous')}
			</label>
			<p class="mb-4 text-xs text-muted-foreground">{t('tips.anonymous_hint', 'The recipient still sees your name — anonymity only hides you from public stats.')}</p>
			<button type="button" onclick={submit} disabled={submitting}
				class="flex w-full items-center justify-center gap-2 rounded-lg bg-primary px-4 py-2.5 text-sm font-semibold text-primary-foreground transition hover:bg-primary/90 disabled:opacity-50">
				{#if submitting}<Loader2 class="size-4 animate-spin" />{:else}<Heart class="size-4" />{/if}
				{t('tips.send')}
			</button>
		{/if}

		{#if config.oc_collective_slug || (config.external_links ?? []).length > 0}
			<div class="mt-4 border-t border-border pt-3">
				<p class="mb-2 text-xs font-semibold uppercase text-muted-foreground">{t('tips.external')}</p>
				<div class="flex flex-col gap-2">
					{#if config.oc_collective_slug}
						<a href="https://opencollective.com/{config.oc_collective_slug}/donate" target="_blank" rel="noopener noreferrer"
							class="flex items-center gap-2 rounded-lg border border-border px-3 py-2 text-sm hover:bg-muted">
							<ExternalLink class="size-4" />Open Collective
						</a>
					{/if}
					{#each config.external_links ?? [] as link}
						<a href={link.url} target="_blank" rel="noopener noreferrer"
							class="flex items-center gap-2 rounded-lg border border-border px-3 py-2 text-sm hover:bg-muted">
							<ExternalLink class="size-4" />{link.label}
						</a>
					{/each}
				</div>
			</div>
		{/if}
	{/if}
</Modal>
