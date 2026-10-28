<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
	import { t } from '$lib/i18n';
	import { invalidateAll } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { calculateWithdrawal, createWithdrawal, createWithdrawalMethod } from '$lib/api/hub';
	import type { WalletData, Withdrawal } from './types';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import AppealButton from '$lib/components/appeals/AppealButton.svelte';

	interface Props {
		data: WalletData;
	}

	let { data }: Props = $props();

	const creditsBalance = $derived(data.wallet?.credits_balance ?? 0);
	const currencies = $derived(data.currencies ?? []);
	const methods = $derived(data.withdrawalMethods ?? []);
	// Withdrawal history is page_size/page_offset-paginated (SSR loads the
	// first page of 20); further pages append via load-more. Resyncs when
	// the loader data is replaced by invalidateAll.
	const withdrawalsPageSize = 20;
	let withdrawals = $state<Withdrawal[]>([]);
	let withdrawalsHasMore = $state(false);
	let withdrawalsLoading = $state(false);
	let withdrawalsError = $state<string | null>(null);
	// Reads only data.withdrawals so appended pages don't retrigger a reset.
	$effect(() => {
		const incoming = data.withdrawals ?? [];
		withdrawals = incoming;
		withdrawalsHasMore = incoming.length === withdrawalsPageSize;
	});

	async function loadMoreWithdrawals() {
		if (withdrawalsLoading || !withdrawalsHasMore) return;
		withdrawalsLoading = true;
		withdrawalsError = null;
		const res = await api.get<Withdrawal[]>(
			`/me/wallet/withdrawals?page_size=${withdrawalsPageSize}&page_offset=${withdrawals.length}`
		);
		if (res.error) {
			withdrawalsError = res.error.message;
		} else {
			const batch = res.data ?? [];
			const seen = new Set(withdrawals.map((w) => w.id));
			withdrawals = [...withdrawals, ...batch.filter((w) => !seen.has(w.id))];
			withdrawalsHasMore = batch.length === withdrawalsPageSize;
		}
		withdrawalsLoading = false;
	}
	const withdrawalProviders = $derived(data.withdrawalProviders ?? []);

	const preferredCode = $derived(data.preferredCurrency?.preferred_currency ?? '');
	const defaultCurrency = $derived(
		currencies.find((c) => c.code === preferredCode) ?? currencies.find((c) => c.code === 'USD') ?? currencies[0]
	);

	let amount = $state<string>('');
	let selectedCurrency = $state<string>('');
	let selectedMethodId = $state<string>('');

	$effect(() => {
		if (!selectedCurrency) {
			selectedCurrency = defaultCurrency?.code ?? '';
		}
	});
	let calcResult = $state<{
		credit_amount: number;
		currency_code: string;
		credits_per_unit: number;
		fiat_amount: number;
		fee_amount: number;
		fee_basis: string;
		fee_rate: number;
		net_amount: number;
	} | null>(null);
	let calculating = $state(false);
	let submitting = $state(false);
	let addingMethod = $state(false);
	let error = $state<string | null>(null);
	let success = $state<string | null>(null);

	// New payout method form.
	let newProvider = $state<string>('');
	let newAccount = $state<string>('');
	let newCurrency = $state<string>('');
	let newLabel = $state<string>('');

	$effect(() => {
		if (!newProvider && withdrawalProviders.length > 0) {
			newProvider = withdrawalProviders[0];
		}
	});

	// 0 = settings endpoint unavailable → no client-side minimum; the
	// backend validates the real configured minimum on submit.
	const minWithdraw = $derived(data.minWithdrawCredits ?? 0);

	function formatFiat(minor: number, code: string): string {
		const currency = currencies.find((c) => c.code === code);
		const symbol = currency?.symbol ?? '';
		return `${symbol}${(minor / 100).toFixed(2)} ${code}`;
	}

	async function calculate() {
		const creditAmount = parseInt(amount, 10) || 0;
		if (creditAmount <= 0) {
			error = t('wallet.withdraw.invalid_amount', 'Please enter a valid credit amount.');
			return;
		}
		if (minWithdraw > 0 && creditAmount < minWithdraw) {
			error = t('wallet.withdraw.minimum', 'Minimum withdrawal is {n} credits.').replace('{n}', String(minWithdraw));
			return;
		}
		if (creditAmount > creditsBalance) {
			error = t('wallet.withdraw.insufficient', 'You do not have enough credits.');
			return;
		}

		error = null;
		success = null;
		calculating = true;

		const res = await calculateWithdrawal({
			credit_amount: creditAmount,
			currency_code: selectedCurrency || undefined
		});

		calculating = false;
		if (res.error || !res.data) {
			error = res.error?.message ?? t('wallet.withdraw.calculate_failed', 'Calculation failed.');
			calcResult = null;
			return;
		}
		calcResult = res.data;
		selectedCurrency = res.data.currency_code;
	}

	async function submitWithdrawal() {
		if (!calcResult) {
			error = t('wallet.withdraw.calculate_first', 'Calculate the withdrawal first.');
			return;
		}
		if (methods.length === 0) {
			error = t('wallet.withdraw.add_method_first', 'Add a payout method first.');
			return;
		}
		if (!selectedMethodId) {
			error = t('wallet.withdraw.select_method', 'Select a payout method.');
			return;
		}

		error = null;
		success = null;
		submitting = true;

		const res = await createWithdrawal({
			credit_amount: calcResult.credit_amount,
			currency_code: calcResult.currency_code,
			payout_method_id: selectedMethodId || undefined
		});

		submitting = false;
		if (res.error || !res.data) {
			error = res.error?.message ?? t('wallet.withdraw.submit_failed', 'Withdrawal failed.');
			return;
		}

		success = t('wallet.withdraw.submitted', 'Withdrawal submitted. Status: {status}').replace('{status}', res.data.status);
		amount = '';
		calcResult = null;
		selectedMethodId = '';
		await invalidateAll();
	}

	async function addMethod() {
		if (!newAccount.trim() || !newCurrency.trim() || !newProvider.trim()) {
			error = t('wallet.withdraw.method_required', 'Provider, account and currency are required.');
			return;
		}

		error = null;
		success = null;
		addingMethod = true;

		const res = await createWithdrawalMethod({
			provider: newProvider,
			account: newAccount.trim(),
			currency_code: newCurrency.trim(),
			label: newLabel.trim() || undefined,
			is_default: true
		});

		addingMethod = false;
		if (res.error) {
			error = res.error.message;
			return;
		}

		success = t('wallet.withdraw.method_added', 'Payout method added.');
		newAccount = '';
		newCurrency = '';
		newLabel = '';
		await invalidateAll();
	}

	function statusClass(status: string): string {
		switch (status) {
			case 'completed':
				return 'text-[var(--color-success)]';
			case 'failed':
			case 'rejected':
			case 'cancelled':
				return 'text-[var(--color-danger)]';
			case 'pending_approval':
				return 'text-[var(--color-warning)]';
			default:
				return 'text-[var(--color-muted)]';
		}
	}
</script>

<div class="space-y-6">
	<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<p class="text-sm text-[var(--color-muted)]">{t('wallet.withdraw.credits_balance', 'Credits Balance')}</p>
		<p class="text-2xl font-bold text-[var(--color-text)]">{creditsBalance.toLocaleString()}</p>
	</div>

	{#if currencies.length === 0}
		<p class="rounded-[var(--radius)] bg-[var(--color-warning)]/10 px-3 py-2 text-sm text-[var(--color-warning)]">
			{t('wallet.withdraw.no_currencies', 'No withdrawal currencies are configured.')}
		</p>
	{:else}
		<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">{t('wallet.withdraw.title', 'Withdraw Credits')}</h2>

			<div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
				<div>
					<label class="mb-1 block text-sm font-medium text-[var(--color-text)]" for="withdraw-amount">
						{t('wallet.withdraw.credit_amount', 'Credits to withdraw')}
					</label>
					<input
						id="withdraw-amount"
						type="number"
						min={minWithdraw || 1}
						bind:value={amount}
						oninput={() => (calcResult = null)}
						aria-describedby="withdraw-amount-hint"
						class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
						placeholder={t('wallet.withdraw.enter_amount', 'Enter credit amount')}
					/>
					{#if minWithdraw > 0}
						<p class="mt-1 text-xs text-[var(--color-muted)]">
							{t('wallet.withdraw.minimum', 'Minimum {n} credits').replace('{n}', minWithdraw.toLocaleString())}
						</p>
					{/if}
				</div>

				<div>
					<label class="mb-1 block text-sm font-medium text-[var(--color-text)]" for="withdraw-currency">
						{t('wallet.withdraw.currency', 'Currency')}
					
					<HintIcon text={t('wallet.withdraw.currency_hint', 'The currency you want to be paid in.')} /></label>
					<AutocompleteSelect
						id="withdraw-currency"
						bind:value={selectedCurrency}
						options={currencies.map((c) => ({ value: c.code, label: `${c.name ?? c.code} (${c.symbol})` }))}
					/>
					<HintIcon text={t('wallet.withdraw.currency_hint', 'The currency you want to be paid in.')} />
				</div>
			</div>

			{#if methods.length > 0}
			"withdraw-method">
						{t('wallet.withdraw.method', 'Payout Method')}
					</label>
					<AutocompleteSelect
						id="withdraw-method"
						bind:value={selectedMethodId}
						options={[
							{ value: '', label: t('wallet.withdraw.select_method_placeholder', 'Select a payout method') },
							...methods.map((m) => ({
								value: m.id,
								label: `${m.label || m.provider} - ${m.account}${m.currency_code ? ` (${m.currency_code})` : ''}${m.is_default ? ` (${t('wallet.withdraw.default', 'Default')})` : ''}`
							}))
						]}
					/>
					<HintIcon text={t('wallet.withdraw.method_hint', 'One of your saved payout methods.')} />
				</div>
			{:else}
				<p class="mt-4 rounded-[var(--rallet.withdraw.no_methods', 'Add a payout method before withdrawing.')}
				</p>
			{/if}

			{#if calcResult}
				<div class="mt-4 grid gap-2 rounded-[var(--radius)] bg-[var(--color-muted)]/5 p-3 text-sm">
					<div class="flex justify-between">
						<span class="text-[var(--color-muted)]">{t('wallet.withdraw.fiat_amount', 'Fiat amount')}</span>
						<span class="font-medium text-[var(--color-text)]">{formatFiat(calcResult.fiat_amount, calcResult.currency_code)}</span>
					</div>
					<div class="flex justify-between">
						<span class="text-[var(--color-muted)]">{t('wallet.withdraw.fee', 'Fee')}</span>
						<span class="font-medium text-[var(--color-text)]">{formatFiat(calcResult.fee_amount, calcResult.currency_code)}</span>
					</div>
					<div class="flex justify-between">
						<span class="text-[var(--color-muted)]">{t('wallet.withdraw.net_amount', 'Net amount')}</span>
						<span class="font-semibold text-[var(--color-text)]">{formatFiat(calcResult.net_amount, calcResult.currency_code)}</span>
					</div>
					<div class="flex justify-between">
						<span class="text-[var(--color-muted)]">{t('wallet.withdraw.rate', 'Rate')}</span>
						<span class="font-medium text-[var(--color-text)]">1 {calcResult.currency_code} = {calcResult.credits_per_unit.toLocaleString()} {t('wallet.withdraw.credits', 'credits')}</span>
					</div>
				</div>
			{/if}

			{#if error}
				<p class="mt-4 rounded-[var(--radius)] bg-[var(--color-danger)]/10 px-3 py-2 text-sm text-[var(--color-danger)]" role="alert">{error}</p>
			{/if}
			{#if success}
				<p class="mt-4 rounded-[var(--radius)] bg-[var(--color-success)]/10 px-3 py-2 text-sm text-[var(--color-success)]" role="status">{success}</p>
			{/if}

			<div class="mt-4 flex gap-3">
				<button
					type="button"
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-muted)]/5 disabled:cursor-not-allowed disabled:opacity-50"
					onclick={calculate}
					disabled={calculating || !amount}
				>
					{#if calculating}
						{t('wallet.withdraw.calculating', 'Calculating...')}
					{:else}
						{t('wallet.withdraw.calculate', 'Calculate')}
					{/if}
				</button>
				<button
					type="button"
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-6 py-2 text-sm font-semibold text-[var(--color-primary-contrast)] transition-colors hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
					onclick={submitWithdrawal}
					disabled={submitting || !calcResult || methods.length === 0 || !selectedMethodId}
				>
					{#if submitting}
						{t('wallet.withdraw.submitting', 'Submitting...')}
					{:else}
						{t('wallet.withdraw.submit', 'Withdraw')}
					{/if}
				</button>
			</div>
		</div>
	{/if}

	<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">{t('wallet.withdraw.methods_title', 'Payout Methods')}</h2>

		{#if methods.length > 0}
			<ul class="mb-4 space-y-2">
				{#each methods as m (m.id)}
					<li class="flex items-center justify-between rounded-[var(--radius)] border border-[var(--color-border)] p-3 text-sm">
						<div>
							<p class="font-medium text-[var(--color-text)]">{m.label || m.provider}</p>
							<p class="text-[var(--color-muted)]">{m.account}{m.currency_code ? ` (${m.currency_code})` : ''}</p>
						</div>
						{#if m.is_default}
							<span class="rounded-full bg-[var(--color-primary)]/10 px-2 py-0.5 text-xs font-medium text-[var(--color-primary)]">{t('wallet.withdraw.default', 'Default')}</span>
						{/if}
					</li>
				{/each}
			</ul>
		{:else}
			<p class="mb-4 text-sm text-[var(--color-muted)]">{t('wallet.withdraw.no_methods', 'Add a payout method before withdrawing.')}</p>
		{/if}

		<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
			<div>
				<label class="mb-1 block text-sm font-medium text-[var(--color-text)]" for="method-provider">{t('wallet.withdraw.method_provider', 'Provider')}</label>
				<AutocompleteSelect
					id="method-provider"
					bind:value={newProvider}
					options={withdrawalProviders.map((p) => ({ value: p, label: p }))}
				/>
				<HintIcon text={t('wallet.withdraw.method_provider_hint', 'The service or network that pays you out.')} />
			</div>
			<div>
				<label class="mb-1 block text-sm font-medium text-[var(--color-text)]" for="method-currency">{t('wallet.withdraw.method_currency', 'Currency / Coin')}</label>
				<input
					id="method-currency"
					bind:value={newCurrency}
					aria-describedby="method-currency-hint"
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
					placeholder={t('wallet.withdraw.method_currency_placeholder', 'e.g. usdttrc20')}
				/>
				<HintIcon id="method-currency-hint" text={t('wallet.withdraw.method_currency_hint', 'Currency code the provider expects, e.g. usdttrc20 or ngn.')} />
			</div>
			<div class="sm:col-span-2">
				<label class="mb-1 block text-sm font-medium text-[var(--color-text)]" for="method-account">{t('wallet.withdraw.method_account', 'Account / Address')}</label>
				<input
					id="method-account"
					bind:value={newAccount}
					aria-describedby="method-account-hint"
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
					placeholder={t('wallet.withdraw.method_account_placeholder', 'Wallet address, email, or account number')}
				/>
				<HintIcon id="method-account-hint" text={t('wallet.withdraw.method_account_hint', 'Where the payout goes — double-check it, withdrawals cannot be reversed.')} />
			</div>
			<div class="sm:col-span-2">
				<label class="mb-1 block text-sm font-medium text-[var(--color-text)]" for="method-label">{t('wallet.withdraw.method_label', 'Label')}</label>
				<input
					id="method-label"
					bind:value={newLabel}
					aria-describedby="method-label-hint"
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
					placeholder={t('wallet.withdraw.method_label_placeholder', 'My USDT TRC20 wallet')}
				/>
				<HintIcon id="method-label-hint" text={t('wallet.withdraw.method_label_hint', 'A name to recognize this payout method later.')} />
			</div>
		</div>

		<div class="mt-3 flex justify-end">
			<button
				type="button"
				class="inline-flex ite] transition-colors hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
				onclick={addMethod}
				disabled={addingMethod}
			>
				{#if addingMethod}
					{t('wallet.withdraw.adding_method', 'Adding...')}
				{:else}
					{t('wallet.withdraw.add_method', 'Add Payout Method')}
				{/if}
			</button>
		</div>
	</div>

	{#if withdrawals.length > 0}
		<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">{t('wallet.withdraw.history_title', 'Withdrawal History')}</h2>
			<ul class="space-y-2">
				{#each withdrawals as w (w.id)}
					<li class="flex flex-col gap-1 rounded-[var(--radius)] border border-[var(--color-border)] p-3 text-sm">
						<div class="flex items-center justify-between">
							<span class="font-medium text-[var(--color-text)]">{w.credit_amount.toLocaleString()} {t('wallet.withdraw.credits', 'credits')}</span>
							<span class="font-semibold {statusClass(w.status)}">{w.status}</span>
						</div>
						<div class="flex justify-between text-xs text-[var(--color-muted)]">
							<span>{formatFiat(w.net_amount, w.currency_code)}</span>
							<span>{new Date(w.created_at ?? '').toLocaleString()}</span>
						</div>
						{#if w.status === 'rejected'}
							<AppealButton entityType="withdrawal" entityId={w.id} />
						{/if}
					</li>
				{/each}
			</ul>
			{#if withdrawalsError}
				<p class="mt-3 rounded-[var(--radius)] bg-[var(--color-danger)]/10 px-3 py-2 text-sm text-[var(--color-danger)]" role="alert">{withdrawalsError}</p>
			{/if}
			{#if withdrawalsHasMore}
				<button
					type="button"
					onclick={loadMoreWithdrawals}
					disabled={withdrawalsLoading}
					class="mt-3 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-hover)] disabled:opacity-50"
				>
					{withdrawalsLoading ? t('settings.common.loading', 'Loading…') : t('settings.common.load_more', 'Load more')}
				</button>
			{/if}
		</div>
	{/if}
</div>
