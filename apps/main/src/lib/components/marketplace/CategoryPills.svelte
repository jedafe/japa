<script lang="ts">
	// §114 category pills row — 'All' + dynamic service_categories entries,
	// each with its Lucide icon and admin-picked accent chip. Clicking writes
	// ?category= while preserving the rest of the query string (real anchors,
	// so SSR + keyboard navigation work). Beyond MAX_VISIBLE the remainder
	// collapse into a "More…" Bits UI dropdown.
	// Ref: docs/superpowers/specs/2026-10-07-services-marketplace-upgrade-design.md (§3.2)
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { DropdownMenu } from 'bits-ui';
	import { Check, ChevronDown, Grid2x2 } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { accentClasses } from '$lib/marketplace';
	import { serviceCategoryIcon } from './serviceCategoryIcon';
	import type { ServiceCategoryInfo } from '$lib/types';

	interface Props {
		categories: ServiceCategoryInfo[];
		/** Current ?category= slug ('' = all). */
		active: string;
		/** Current ?kind= filter (''|'all'|'service'|'mentorship'). */
		kind: string;
	}
	let { categories, active, kind }: Props = $props();

	const MAX_VISIBLE = 8;

	// 'both' applies to every kind; kind-scoped categories only show on their
	// own tab. No filter when kind is ''/'all'. On the mentorship tab,
	// service/package-scoped categories are hidden — clicking one would
	// land on an empty mentorship-filtered list.
	const filtered = $derived(
		kind === 'service' || kind === 'mentorship'
			? categories.filter(
					(c) => !c.applies_to || c.applies_to === 'both' || c.applies_to === kind
				)
			: categories
	);

	// Keep the active category pinned in the visible strip even when it would
	// otherwise land inside the More… overflow.
	const visible = $derived.by(() => {
		const head = filtered.slice(0, MAX_VISIBLE);
		if (active && !head.some((c) => c.value === active)) {
			const idx = filtered.findIndex((c) => c.value === active);
			if (idx >= MAX_VISIBLE) head[MAX_VISIBLE - 1] = filtered[idx];
		}
		return head;
	});
	const overflow = $derived(filtered.filter((c) => !visible.includes(c)));

	// Build a pill href off the live URL so sort/kind/geo filters survive a
	// category change; pagination always resets and the one-shot
	// ?submitted=1 notice never propagates into new URLs.
	function categoryHref(slug: string): string {
		const params = new URLSearchParams(page.url.searchParams);
		if (slug) params.set('category', slug);
		else params.delete('category');
		params.delete('page');
		params.delete('submitted');
		const qs = params.toString();
		return `/services${qs ? `?${qs}` : ''}`;
	}

	const pillBase =
		'inline-flex items-center gap-1.5 rounded-full border px-3 py-1.5 text-xs font-medium transition-colors';
	const pillActive =
		'border-[var(--color-primary)] bg-[var(--color-primary)] text-[var(--color-primary-contrast)]';
</script>

<nav class="flex flex-wrap items-center gap-2" aria-label={t('services.categories_aria', 'Service categories')}>
	<a
		href={categoryHref('')}
		aria-current={!active ? 'page' : undefined}
		class="{pillBase} {!active
			? pillActive
			: 'border-[var(--color-border)] bg-[var(--color-surface)] text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]'}"
	>
		<Grid2x2 size={13} />
		{t('services.category_all', 'All')}
	</a>

	{#each visible as cat (cat.value)}
		{@const Icon = serviceCategoryIcon(cat.icon)}
		{@const ac = accentClasses(cat.accent_color)}
		<a
			href={categoryHref(cat.value)}
			aria-current={active === cat.value ? 'page' : undefined}
			class="{pillBase} {active === cat.value ? pillActive : ac.chip}"
		>
			<Icon size={13} />
			{cat.label}
		</a>
	{/each}

	{#if overflow.length > 0}
		<DropdownMenu.Root>
			<DropdownMenu.Trigger
				class="{pillBase} border-[var(--color-border)] bg-[var(--color-surface)] text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
			>
				{t('services.categories_more', 'More…')}
				<ChevronDown size={13} class="text-[var(--color-muted)]" />
			</DropdownMenu.Trigger>
			<DropdownMenu.Portal>
				<DropdownMenu.Content
					align="start"
					class="z-50 max-h-72 w-56 overflow-y-auto rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] py-1 shadow-lg"
				>
					{#each overflow as cat (cat.value)}
						{@const Icon = serviceCategoryIcon(cat.icon)}
						{@const ac = accentClasses(cat.accent_color)}
						<DropdownMenu.Item
							onclick={() => goto(categoryHref(cat.value))}
							class="flex w-full cursor-pointer items-center gap-2 px-3 py-1.5 text-left text-xs data-[highlighted]:bg-[var(--color-surface-subtle)] {active ===
							cat.value
								? 'font-medium text-[var(--color-primary)]'
								: 'text-[var(--color-text-subtle)]'}"
						>
							<Icon size={13} class={ac.icon} />
							<span class="flex-1 truncate">{cat.label}</span>
							{#if active === cat.value}
								<Check size={13} class="text-[var(--color-primary)]" />
							{/if}
						</DropdownMenu.Item>
					{/each}
				</DropdownMenu.Content>
			</DropdownMenu.Portal>
		</DropdownMenu.Root>
	{/if}
</nav>
