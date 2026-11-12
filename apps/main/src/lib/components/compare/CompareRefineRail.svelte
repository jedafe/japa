<script lang="ts">
	/** Refine rail — search + counted category links (mockup 2 left column).
	 *  Mobile collapses behind a toggle (spec: no persistent rail on small
	 *  screens); desktop shows the full rail. The themes block only renders
	 *  when the caller supplies real facets — never hardcoded placeholders. */
	import { ChevronDown, Search } from 'lucide-svelte';
	import { t } from '#lib/i18n/index.js';

	interface RailCategory {
		label: string;
		count?: number;
		href: string;
		active: boolean;
	}

	let {
		query = $bindable(''),
		categories,
		onQuery
	}: {
		query?: string;
		categories: RailCategory[];
		onQuery: (q: string) => void;
	} = $props();

	let open = $state(false);
	let timeout: ReturnType<typeof setTimeout> | null = null;
	function input(e: Event) {
		query = (e.currentTarget as HTMLInputElement).value;
		if (timeout) clearTimeout(timeout);
		timeout = setTimeout(() => onQuery(query), 300);
	}
</script>

<aside class="w-full shrink-0 lg:w-60">
	<!-- Mobile toggle: aria-expanded mirrors visual state; hidden on lg where
		 the rail body is always shown via lg:block below. -->
	<button
		type="button"
		aria-expanded={open}
		onclick={() => (open = !open)}
		class="flex w-full items-center justify-between rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2.5 text-sm font-semibold text-[var(--color-text)] lg:hidden"
	>
		{t('jce.compare.category.refine', 'Refine results')}
		<ChevronDown size={16} class="motion-safe:transition-transform {open ? 'rotate-180' : ''}" />
	</button>

	<div class="{open ? 'block' : 'hidden'} space-y-5 pt-4 lg:block lg:pt-0">
		<h2 class="hidden text-sm font-semibold text-[var(--color-text)] lg:block">
			{t('jce.compare.category.refine', 'Refine results')}
		</h2>
		<div class="relative">
			<Search size={15} class="absolute start-3 top-1/2 -translate-y-1/2 text-[var(--color-muted)]" />
			<input
				type="text"
				value={query}
				oninput={input}
				placeholder={t('jce.compare.category.search_placeholder', 'Search comparisons...')}
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] py-2 ps-9 pe-3 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
			/>
		</div>

		<nav class="space-y-1" aria-label={t('jce.compare.category.categories', 'Categories')}>
			<p class="pb-1 text-xs font-semibold uppercase tracking-wide text-[var(--color-muted)]">
				{t('jce.compare.category.categories', 'Categories')}
			</p>
			{#each categories as cat (cat.href)}
				<a
					href={cat.href}
					aria-current={cat.active ? 'page' : undefined}
					class="flex items-center justify-between rounded-[var(--radius)] px-3 py-2 text-sm transition-colors
						{cat.active
							? 'bg-[var(--color-primary)]/10 font-medium text-[var(--color-primary-text)]'
							: 'text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]'}"
				>
					<span>{cat.label}</span>
					{#if cat.count !== undefined}
						<span class="text-xs text-[var(--color-muted)]">{cat.count}</span>
					{/if}
				</a>
			{/each}
		</nav>
	</div>
</aside>
