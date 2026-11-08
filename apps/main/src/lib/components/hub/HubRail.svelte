<script lang="ts">
	import { Dialog } from 'bits-ui';
	import { Check, ChevronDown } from 'lucide-svelte';
	import { t } from '#lib/i18n/index.js';

	interface RailItem {
		slug: string;
		label: string;
		icon: string;
		backendReady: boolean;
		isRoute: boolean;
	}
	interface Props {
		groups: [string, RailItem[]][];
		basePath: string;
		active: string;
		labelKey: string;
	}
	let { groups, basePath, active, labelKey }: Props = $props();

	const activeItem = $derived(
		groups.flatMap(([, items]) => items).find((item) => item.slug === active)
	);
	const activeLabel = $derived(activeItem ? t(activeItem.label, activeItem.label) : t(labelKey));

	// Mobile: the rail collapses into a single trigger that opens a bottom-sheet
	// picker (same pattern as UrlTabs) — the vertical rail is md+ only.
	let pickerOpen = $state(false);
</script>

<!-- Mobile trigger: shows the active view, opens the picker sheet. -->
<button
	type="button"
	onclick={() => (pickerOpen = true)}
	aria-haspopup="dialog"
	class="mb-2 flex w-full shrink-0 cursor-pointer items-center justify-between gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2.5 text-sm font-medium text-[var(--color-text)] md:hidden"
>
	{activeLabel}
	<ChevronDown size={14} class="text-[var(--color-muted)]" />
</button>

<Dialog.Root bind:open={pickerOpen}>
	<Dialog.Portal>
		<Dialog.Overlay
			class="fixed inset-0 z-40 bg-black/50 backdrop-blur-sm data-[state=open]:animate-in data-[state=closed]:animate-out"
		/>
		<Dialog.Content
			class="fixed inset-x-0 bottom-0 z-50 max-h-[80dvh] overflow-y-auto rounded-t-[12px] border border-[var(--color-border)] bg-[var(--color-bg)] p-2 pb-[calc(0.5rem+env(safe-area-inset-bottom))] shadow-xl"
		>
			<Dialog.Title class="sr-only">{t(labelKey)}</Dialog.Title>
			<div role="navigation" aria-label={t(labelKey)}>
				{#each groups as [group, items] (group)}
					<p class="px-4 pb-1 pt-3 text-xs font-medium text-[var(--color-muted)]">
						{t(group, group)}
					</p>
					{#each items as item (item.slug)}
						{@const disabled = !item.backendReady}
						<a
							href={disabled ? undefined : `${basePath}/${item.slug}`}
							aria-current={active === item.slug ? 'page' : undefined}
							aria-disabled={disabled}
							onclick={() => (pickerOpen = false)}
							class="flex w-full items-center justify-between gap-2 rounded-[var(--radius)] px-4 py-3 text-sm font-medium transition-colors {active ===
							item.slug
								? 'text-[var(--color-text)]'
								: 'text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)]'} {disabled
								? 'pointer-events-none opacity-40'
								: ''}"
						>
							{t(item.label, item.label)}
							{#if active === item.slug}
								<Check size={16} class="text-[var(--color-primary-text)]" />
							{/if}
						</a>
					{/each}
				{/each}
			</div>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>

<nav class="sticky top-[var(--nav-height)] hidden self-start md:block" aria-label={t(labelKey)}>
	{#each groups as [group, items] (group)}
		<div class="mb-4">
			<p class="px-3 pb-1 text-xs font-medium text-[var(--color-muted)]">{t(group, group)}</p>
			{#each items as item (item.slug)}
				{@const disabled = !item.backendReady}
				<a
					href={disabled ? undefined : `${basePath}/${item.slug}`}
					aria-current={active === item.slug ? 'page' : undefined}
					aria-disabled={disabled}
					class="block rounded-[var(--radius)] px-3 py-1.5 text-sm {active === item.slug
						? 'bg-[var(--color-surface-subtle)] text-[var(--color-text)]'
						: 'text-[var(--color-muted)] hover:text-[var(--color-text)]'} {disabled
						? 'pointer-events-none opacity-40'
						: ''}"
				>
					{t(item.label, item.label)}
				</a>
			{/each}
		</div>
	{/each}
</nav>
