<script lang="ts">
	import { t } from '$lib/i18n';
	import type { FeedTab, SortValue } from '$lib/feed/endpoint';

	interface Props {
		tab: FeedTab;
		sort: SortValue;
		authed: boolean;
	}
	let { tab, sort, authed }: Props = $props();

	const tabs = $derived(
		authed
			? [
					{ value: 'for_you' as const, label: t('feed.tab.for_you') },
					{ value: 'following' as const, label: t('feed.tab.following') }
				]
			: [{ value: 'explore' as const, label: t('feed.tab.explore', 'Explore') }]
	);
</script>

<div
	class="sticky top-0 z-10 flex gap-1 border-b border-[var(--color-border)] bg-[color-mix(in_srgb,var(--color-bg)_90%,transparent)] px-3 backdrop-blur"
	style="min-height: var(--nav-height)"
>
	{#each tabs as tabItem (tabItem.value)}
		<a
			href="?tab={tabItem.value}&sort={sort}"
			data-sveltekit-noscroll
			aria-current={tab === tabItem.value ? 'page' : undefined}
			class="relative flex items-center px-4 text-sm font-medium transition-colors {tab === tabItem.value
				? 'text-[var(--color-text)]'
				: 'text-[var(--color-muted)] hover:text-[var(--color-text)]'}"
		>
			{tabItem.label}
			{#if tab === tabItem.value}
				<span class="absolute inset-x-3 -bottom-px h-0.5 rounded-full bg-[var(--color-primary)]"></span>
			{/if}
		</a>
	{/each}
</div>
