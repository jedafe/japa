<script lang="ts">
	// AI settings — one tabbed section hosting AI privacy controls and the
	// BYOK provider-keys manager. /settings/ai?tab=privacy|keys; the legacy
	// /settings/ai-privacy and /settings/ai-keys slugs redirect here (see
	// [section]/+page.server.ts). The keys tab is hidden when the
	// feature.ai_byok flag is off.
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import AIPrivacySection from './AIPrivacySection.svelte';
	import AIKeysSection from './AIKeysSection.svelte';

	interface AiSectionData {
		privacy: unknown;
		keys: unknown;
	}

	let { data }: { data: unknown } = $props();
	const d = $derived((data as AiSectionData | null) ?? { privacy: null, keys: null });

	const byokEnabled = $derived(page.data.features?.['feature.ai_byok'] === true);

	type Tab = 'privacy' | 'keys';
	const allTabs: { key: Tab; labelKey: string; default: string; byokOnly: boolean }[] = [
		{ key: 'privacy', labelKey: 'settings.nav.ai_privacy', default: 'AI privacy', byokOnly: false },
		{ key: 'keys', labelKey: 'settings.nav.ai_provider_keys', default: 'AI provider keys', byokOnly: true }
	];
	const tabs = $derived(allTabs.filter((tab) => !tab.byokOnly || byokEnabled));

	// Seed from ?tab= for redirected legacy links; clamp to visible tabs.
	let activeTab = $state<Tab>(
		untrack(() => (page.url.searchParams.get('tab') === 'keys' ? 'keys' : 'privacy'))
	);
	$effect(() => {
		if (tabs.length > 0 && !tabs.some((tab) => tab.key === activeTab)) {
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
		<h1 class="text-lg font-medium text-[var(--color-text)]">{t('settings.nav.ai', 'AI')}</h1>
		<p class="text-sm text-[var(--color-muted)]">{t('settings.ai.desc', 'AI assistant privacy and your own provider keys.')}</p>
	</header>

	<div class="mb-4 flex flex-wrap gap-1 border-b border-[var(--color-border)]" role="tablist" aria-label={t('settings.ai.tabs_label', 'AI settings')}>
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

	{#if activeTab === 'privacy'}
		<AIPrivacySection data={d.privacy} embedded />
	{:else if activeTab === 'keys' && byokEnabled}
		<AIKeysSection data={d.keys} embedded />
	{/if}
</section>
