<script lang="ts">
	/**
	 * Reusable tab bar built on Bits UI Tabs, styled to match the ProfileTabs
	 * underline pattern. Supports two modes:
	 *
	 * 1. Instant client-side switching (default): bind:value + replaceState URL
	 *    sync — use when all tab content is already loaded server-side.
	 * 2. Navigation-based switching: pass onValueChange callback — use when each
	 *    tab's data is loaded per-request via ?tab= (the callback typically calls
	 *    goto() to trigger SvelteKit navigation).
	 *
	 * Ref: §78, ADR-058 (unified app shell tab pattern).
	 */
	import { Tabs, Dialog } from 'bits-ui';
	import { Menu, MenuItem } from '@japaconnect/ui';
	import { ChevronDown, Check } from 'lucide-svelte';
	import { page } from '$app/state';
	import { replaceState } from '$app/navigation';
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';

	export interface UrlTab {
		value: string;
		label: string;
	}

	interface Props {
		tabs: UrlTab[];
		/** Active tab value (bind:value for two-way binding). */
		value?: string;
		/** URL query param name for tab state (default 'tab'). */
		urlParam?: string;
		/** Tab value to use when no URL param is present (default: first tab). */
		defaultTab?: string;
		/** Optional callback for navigation-based switching (overrides URL sync). */
		onValueChange?: (value: string) => void;
		/** Extra class for the Tabs.List element. */
		class?: string;
		/** When true, tabs that don't fit the list's width collapse into a "More" dropdown. */
		autoOverflow?: boolean;
	}

	let {
		tabs,
		value = $bindable(),
		urlParam = 'tab',
		defaultTab,
		onValueChange,
		class: className = '',
		autoOverflow = false
	}: Props = $props();

	const resolvedDefault = $derived(defaultTab ?? tabs[0]?.value ?? '');

	// Initialise from URL param or default (only used when value is not bound).
	let internalValue = $state<string>(
		untrack(() => {
			const param = page.url.searchParams.get(urlParam);
			if (param && tabs.some((t) => t.value === param)) return param;
			return resolvedDefault;
		})
	);

	// The effective active tab — uses bound value if provided, else internal state.
	const activeValue = $derived(value ?? internalValue);
	const activeLabel = $derived(
		tabs.find((tab) => tab.value === activeValue)?.label ?? tabs[0]?.label ?? ''
	);

	// Desktop overflow — container-aware. Triggers that don't fit the list's
	// width are rendered `absolute invisible` (still measurable) and exposed in
	// a "More" dropdown. A ResizeObserver recomputes the visible count.
	let rootEl = $state<HTMLElement | null>(null);
	let moreEl = $state<HTMLElement | null>(null);
	let visibleCount = $state(0);
	const effVisible = $derived(visibleCount === 0 ? tabs.length : Math.min(visibleCount, tabs.length));
	const overflowTabs = $derived(autoOverflow ? tabs.slice(effVisible) : []);
	const activeOverflowLabel = $derived(
		overflowTabs.find((tab) => tab.value === activeValue)?.label
	);

	$effect(() => {
		const list = rootEl;
		if (!list || !autoOverflow) return;
		void tabs;
		const compute = () => {
			if (!list.offsetWidth) return;
			const triggers = [...list.querySelectorAll<HTMLElement>('[role="tab"]')];
			const avail = list.clientWidth - 24; // px-3
			const GAP = 4; // gap-1
			const MORE = moreEl?.offsetWidth || 115; // measured once rendered; fallback estimate
			if (triggers.reduce((s, el) => s + el.offsetWidth + GAP, 0) - GAP <= avail) {
				visibleCount = triggers.length;
				return;
			}
			let n = 0, acc = 0;
			for (const el of triggers) {
				if (acc + el.offsetWidth + MORE > avail) break;
				acc += el.offsetWidth + GAP;
				n++;
			}
			visibleCount = Math.max(1, n);
		};
		const ro = new ResizeObserver(compute);
		ro.observe(list);
		compute();
		return () => ro.disconnect();
	});

	// Mobile: the tab row collapses into a single trigger that opens a modal
	// listing every option (they don't fit a narrow bar).
	let mobilePickerOpen = $state(false);

	function pickMobile(v: string) {
		handleChange(v);
		mobilePickerOpen = false;
	}

	function handleChange(newValue: string) {
		if (onValueChange) {
			onValueChange(newValue);
			return;
		}
		// Instant mode — update state + sync URL, preserving other params.
		if (value !== undefined) {
			value = newValue;
		} else {
			internalValue = newValue;
		}
		// Sync to URL via replaceState (no navigation).
		const url = new URL(page.url);
		if (newValue === resolvedDefault) {
			url.searchParams.delete(urlParam);
		} else {
			url.searchParams.set(urlParam, newValue);
		}
		replaceState(`${url.pathname}${url.search}`, {});
	}
</script>

<!-- Mobile: single trigger showing the active option; opens a picker modal. -->
<button
	type="button"
	onclick={() => (mobilePickerOpen = true)}
	aria-haspopup="dialog"
	class="flex shrink-0 cursor-pointer items-center gap-1.5 px-4 text-sm font-medium text-[var(--color-text)] sm:hidden"
	style="min-height: var(--nav-height)"
>
	{activeLabel}
	<ChevronDown size={14} class="text-[var(--color-muted)]" />
</button>

<Dialog.Root bind:open={mobilePickerOpen}>
	<Dialog.Portal>
		<Dialog.Overlay
			class="fixed inset-0 z-40 bg-black/50 backdrop-blur-sm data-[state=open]:animate-in data-[state=closed]:animate-out"
		/>
		<Dialog.Content
			class="fixed inset-x-0 bottom-0 z-50 rounded-t-[12px] border border-[var(--color-border)] bg-[var(--color-bg)] p-2 pb-[calc(0.5rem+env(safe-area-inset-bottom))] shadow-xl"
		>
			<Dialog.Title class="sr-only">{activeLabel}</Dialog.Title>
			<div role="listbox" aria-label={activeLabel}>
				{#each tabs as tab (tab.value)}
					<button
						type="button"
						role="option"
						aria-selected={tab.value === activeValue}
						onclick={() => pickMobile(tab.value)}
						class="flex w-full items-center justify-between gap-2 rounded-[var(--radius)] px-4 py-3 text-sm font-medium transition-colors {tab.value ===
						activeValue
							? 'text-[var(--color-text)]'
							: 'text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)]'}"
					>
						{tab.label}
						{#if tab.value === activeValue}
							<Check size={16} class="text-[var(--color-primary)]" />
						{/if}
					</button>
				{/each}
			</div>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>

<Tabs.Root value={activeValue} onValueChange={handleChange}>
	<Tabs.List
		bind:ref={rootEl}
		class="relative hidden gap-1 overflow-y-hidden border-b border-[var(--color-border)] bg-[color-mix(in_srgb,var(--color-bg)_90%,transparent)] px-3 backdrop-blur sm:flex {className} {autoOverflow &&
		visibleCount === 0
			? 'overflow-x-hidden'
			: 'overflow-x-auto'}"
		style="min-height: var(--nav-height)"
	>
		{#each tabs as tab, i (tab.value)}
			<Tabs.Trigger
				value={tab.value}
				class="flex shrink-0 cursor-pointer items-center px-4 text-sm font-medium transition-colors after:absolute after:inset-x-3 after:-bottom-px after:h-0.5 after:rounded-full after:bg-transparent after:transition-colors data-[state=active]:text-[var(--color-text)] data-[state=active]:after:bg-[var(--color-primary)] data-[state=inactive]:text-[var(--color-muted)] data-[state=inactive]:hover:text-[var(--color-text)] {autoOverflow &&
				i >= effVisible
					? 'invisible absolute left-0'
					: 'relative'}"
			>
				{#snippet child({ props })}
					<span {...props}>{tab.label}</span>
				{/snippet}
			</Tabs.Trigger>
		{/each}
		{#if autoOverflow}
			<div
				class="absolute right-3 top-0 flex h-full items-center bg-[color-mix(in_srgb,var(--color-bg)_90%,transparent)] backdrop-blur {visibleCount !==
					0 && overflowTabs.length === 0
					? 'hidden'
					: ''}"
			>
			<Menu>
				{#snippet trigger(props)}
					<button
						type="button"
						{...props}
						bind:this={moreEl}
						class="relative flex shrink-0 cursor-pointer items-center gap-1 px-4 text-sm font-medium transition-colors after:absolute after:inset-x-3 after:-bottom-px after:h-0.5 after:rounded-full after:bg-transparent after:transition-colors {activeOverflowLabel
							? 'text-[var(--color-text)] after:bg-[var(--color-primary)]'
							: 'text-[var(--color-muted)] hover:text-[var(--color-text)]'}"
					>
						<span class="max-w-20 truncate">{activeOverflowLabel ?? t('common.more', 'More')}</span>
						<ChevronDown size={14} />
					</button>
				{/snippet}
				{#each overflowTabs as tab (tab.value)}
					<MenuItem onSelect={() => handleChange(tab.value)}>{tab.label}</MenuItem>
				{/each}
			</Menu>
			</div>
		{/if}
	</Tabs.List>
</Tabs.Root>
