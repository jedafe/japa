<script lang="ts">
	import { page } from '$app/state';
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

	// Preserve other query params (e.g. ?geo=) when switching sort.
	// Only set ?tab= on the home route (/); /explore has its own route.
	function hrefFor(value: SortValue): string {
		const url = new URL(page.url);
		if (url.pathname === '/') {
			url.searchParams.set('tab', tab);
		}
		url.searchParams.set('sort', value);
		return `${url.pathname}${url.search}`;
	}
</script>

{#if tab !== 'roadmap'}
<div
	class="sticky z-10 flex items-center gap-2 bg-[color-mix(in_srgb,var(--color-bg)_90%,transparent)] px-4 py-2 backdrop-blur"
	style="top: var(--nav-height)"
>
	{#each chips as chip (chip.value)}
		<a
			href={hrefFor(chip.value)}
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
