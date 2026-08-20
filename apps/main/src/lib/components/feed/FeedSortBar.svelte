<script lang="ts">
	import { t } from '$lib/i18n';
	import type { FeedTab, SortValue } from '$lib/feed/endpoint';

	interface Props {
		tab: FeedTab;
		sort: SortValue;
	}
	let { tab, sort }: Props = $props();

	// Note the label→value mapping: "Popular" → top.
	const chips: { value: SortValue; label: string }[] = [
		{ value: 'relevant', label: t('sort.relevant') },
		{ value: 'latest', label: t('sort.latest') },
		{ value: 'top', label: t('sort.popular') },
		{ value: 'rising', label: t('sort.rising') }
	];
</script>

{#if tab !== 'roadmap'}
<div
	class="sticky z-10 flex items-center gap-2 bg-[color-mix(in_srgb,var(--color-bg)_90%,transparent)] px-4 py-2 backdrop-blur"
	style="top: var(--nav-height)"
>
	{#each chips as chip (chip.value)}
		<a
			href="?tab={tab}&sort={chip.value}"
			data-sveltekit-noscroll
			aria-current={sort === chip.value ? 'true' : undefined}
			class="rounded-full px-3 py-1 text-xs font-medium transition-colors {sort === chip.value
				? 'bg-[var(--color-surface-subtle)] text-[var(--color-text)]'
				: 'text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]'}"
		>
			{chip.label}
		</a>
	{/each}
</div>
{/if}
