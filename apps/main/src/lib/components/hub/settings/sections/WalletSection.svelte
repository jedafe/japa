<script lang="ts">
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import type { WalletData } from './wallet/types';
	import WalletDetailsTab from './wallet/WalletDetailsTab.svelte';
	import WalletPurchaseTab from './wallet/WalletPurchaseTab.svelte';
	import WalletHistoryTab from './wallet/WalletHistoryTab.svelte';
	import WalletWithdrawTab from './wallet/WalletWithdrawTab.svelte';

	interface Props {
		data: unknown;
	}

	let { data }: Props = $props();
	const walletData = $derived(data as WalletData | null);

	type Tab = 'details' | 'purchase' | 'history' | 'withdraw';
	// Seed from ?tab= so deep links (e.g. /dashboard/wallet/purchase →
	// /settings/wallet?tab=purchase) land on the right tab.
	const VALID_TABS: Tab[] = ['details', 'purchase', 'withdraw', 'history'];
	let activeTab = $state<Tab>(
		untrack(() => {
			const q = page.url.searchParams.get('tab') as Tab | null;
			return q && VALID_TABS.includes(q) ? q : 'details';
		})
	);

	const allTabs: { key: Tab; labelKey: string; default: string; requiresWithdraw: boolean; requiresWallet: boolean }[] = [
		{ key: 'details', labelKey: 'settings.wallet.tab_details', default: 'Details', requiresWithdraw: false, requiresWallet: false },
		{ key: 'purchase', labelKey: 'settings.wallet.tab_purchase', default: 'Purchase', requiresWithdraw: false, requiresWallet: true },
		{ key: 'withdraw', labelKey: 'settings.wallet.tab_withdraw', default: 'Withdraw', requiresWithdraw: true, requiresWallet: true },
		{ key: 'history', labelKey: 'settings.wallet.tab_history', default: 'History', requiresWithdraw: false, requiresWallet: false }
	];

	const tabs = $derived(
		allTabs.filter(
			(t) =>
				(!t.requiresWallet || walletData?.walletEnabled === true) &&
				(!t.requiresWithdraw || walletData?.withdrawEnabled === true)
		)
	);

	$effect(() => {
		if (tabs.length > 0 && !tabs.some((t) => t.key === activeTab)) {
			activeTab = tabs[0].key;
		}
	});

	function selectTab(tab: Tab) {
		activeTab = tab;
		void goto(`?tab=${tab}`, { replaceState: true, keepFocus: true, noScroll: true });
	}
</script>

<section class="max-w-4xl">
	<header class="mb-4">
		<h1 class="text-lg font-medium text-[var(--color-text)]">{t('settings.nav.wallet')}</h1>
		<p class="text-sm text-[var(--color-muted)]">{t('settings.wallet.desc')}</p>
	</header>

	<div class="mb-4 flex flex-wrap gap-1 border-b border-[var(--color-border)]" role="tablist" aria-label={t('settings.wallet.tabs_label', 'Wallet sections')}>
		{#each tabs as tab (tab.key)}
			<button
				type="button"
				role="tab"
				aria-selected={activeTab === tab.key}
				onclick={() => selectTab(tab.key)}
				class="inline-flex items-center gap-1.5 border-b-2 px-3 py-2 text-sm font-medium transition-colors {activeTab === tab.key
					? 'border-[var(--color-primary)] text-[var(--color-primary)]'
					: 'border-transparent text-[var(--color-muted)] hover:text-[var(--color-text)]'}"
			>
				{t(tab.labelKey, tab.default)}
			</button>
		{/each}
	</div>

	{#if walletData}
		{#if activeTab === 'details'}
			<WalletDetailsTab data={walletData} />
		{:else if activeTab === 'purchase'}
			<WalletPurchaseTab data={walletData} />
		{:else if activeTab === 'withdraw'}
			<WalletWithdrawTab data={walletData} />
		{:else if activeTab === 'history'}
			<WalletHistoryTab data={walletData} />
		{/if}
	{:else}
		<p class="text-sm text-[var(--color-muted)]">{t('settings.wallet.unavailable', 'Wallet data is unavailable.')}</p>
	{/if}
</section>
