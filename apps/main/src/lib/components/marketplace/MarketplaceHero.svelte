<script lang="ts">
	// §114 marketplace hero — eyebrow badge, headline, sub-copy and a large
	// search field on the left; a 2×2 value-prop grid on the right (lg+).
	// Ref: docs/superpowers/specs/2026-10-07-services-marketplace-upgrade-design.md (§3.2)
	import { untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { BadgePercent, Headset, Search, ShieldCheck, Sparkles } from 'lucide-svelte';
	import SearchInput, { type SearchSuggestion } from '$lib/components/ui/SearchInput.svelte';
	import { t } from '$lib/i18n';

	interface Props {
		/** Current ?q= value — seeds the search input. */
		q: string;
		/** Live-suggestion fetcher for the shared SearchInput. */
		fetcher: (q: string) => Promise<SearchSuggestion[]>;
		/** Optional search callback; defaults to goto('/services?q='). */
		onSearch?: (q: string) => void;
	}
	let { q, fetcher, onSearch }: Props = $props();

	let term = $state(untrack(() => q));
	// Re-sync after client-side navigations (back/forward, shared links).
	$effect(() => {
		term = q;
	});

	function submit(e: SubmitEvent) {
		e.preventDefault();
		const v = term.trim();
		if (onSearch) {
			onSearch(v);
		} else {
			goto(v ? `/services?q=${encodeURIComponent(v)}` : '/services');
		}
	}

	const valueProps = $derived([
		{ icon: ShieldCheck, label: t('services.value_curated', 'Curated by experts') },
		{ icon: BadgePercent, label: t('services.value_savings', 'Cost savings') },
		{ icon: Sparkles, label: t('services.value_hassle_free', 'Hassle-free booking') },
		{ icon: Headset, label: t('services.value_support', 'Success support') }
	]);
</script>

<section
	class="border-b border-[var(--color-border)] bg-gradient-to-br from-[var(--color-primary)]/10 via-[var(--color-surface)] to-[var(--color-surface)]"
>
	<div
		class="mx-auto grid w-full max-w-[1400px] items-center gap-8 px-4 py-10 sm:py-12 lg:grid-cols-[1.15fr_1fr] lg:gap-12"
	>
		<div>
			<span
				class="mb-4 inline-flex items-center gap-1.5 rounded-full border border-[var(--color-primary)]/30 bg-[var(--color-primary)]/10 px-3 py-1 text-xs font-semibold text-[var(--color-primary)]"
			>
				<Sparkles size={13} />
				{t('services.hero_eyebrow', '{site} Services Marketplace')}
			</span>
			<h1 class="text-3xl font-bold tracking-tight text-[var(--color-text)] sm:text-4xl">
				{t('services.hero_title', 'Every service for your journey abroad, in one place')}
			</h1>
			<p class="mt-3 max-w-xl text-sm leading-relaxed text-[var(--color-text-subtle)] sm:text-base">
				{t(
					'services.hero_sub',
					'Compare verified providers, travel packages and deals — immigration, relocation, study abroad, work abroad and more.'
				)}
			</p>

			<form role="search" class="mt-6 flex max-w-xl items-stretch gap-2" onsubmit={submit}>
				<div class="min-w-0 flex-1">
					<SearchInput
						bind:value={term}
						placeholder={t('services.search_placeholder', 'Search services…')}
						ariaLabel={t('services.search_placeholder', 'Search services…')}
						{fetcher}
						inputClass="py-2.5"
					/>
				</div>
				<button
					type="submit"
					class="inline-flex shrink-0 items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-semibold text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
				>
					<Search size={15} />
					{t('common.search', 'Search')}
				</button>
			</form>
		</div>

		<div class="grid grid-cols-2 content-center gap-3 lg:gap-4">
			{#each valueProps as v (v.label)}
				{@const Icon = v.icon}
				<div
					class="flex flex-col gap-2.5 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
				>
					<span
						class="grid h-9 w-9 place-items-center rounded-full bg-[var(--color-primary)]/10 text-[var(--color-primary)]"
					>
						<Icon size={18} />
					</span>
					<p class="text-sm font-medium text-[var(--color-text)]">{v.label}</p>
				</div>
			{/each}
		</div>
	</div>
</section>
