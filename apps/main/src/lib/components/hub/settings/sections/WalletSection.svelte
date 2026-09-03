<script lang="ts">
	import { t } from '$lib/i18n';
	import type { WalletData } from './wallet/types';
	import WalletDetailsTab from './wallet/WalletDetailsTab.svelte';
	import WalletConvertTab from './wallet/WalletConvertTab.svelte';
	import WalletPurchaseTab from './wallet/WalletPurchaseTab.svelte';
	import WalletHistoryTab from './wallet/WalletHistoryTab.svelte';

	interface Props {
		data: unknown;
	}

	let { data }: Props = $props();
	const walletData = $derived(data as WalletData | null);

	type Tab = 'details' | 'conversion' | 'purchase' | 'history';
	let activeTab = $state<Tab>('details');

	const tabs: { key: Tab; labelKey: string; default: string }[] = [
		{ key: 'details', labelKey: 'settings.wallet.tab_details', default: 'Details' },
		{ key: 'conversion', labelKey: 'settings.wallet.tab_conversion', default: 'Convert' },
		{ key: 'purchase', labelKey: 'settings.wallet.tab_purchase', default: 'Purchase' },
		{ key: 'history', labelKey: 'settings.wallet.tab_history', default: 'History' }
	];
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
				onclick={() => (activeTab = tab.key)}
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
		{:else if activeTab === 'conversion'}
			<WalletConvertTab data={walletData} />
		{:else if activeTab === 'purchase'}
			<WalletPurchaseTab data={walletData} />
		{:else if activeTab === 'history'}
			<WalletHistoryTab data={walletData} />
		{/if}
	{:else}
		<p class="text-sm text-[var(--color-muted)]">{t('settings.wallet.unavailable', 'Wallet data is unavailable.')}</p>
	{/if}
</section>
