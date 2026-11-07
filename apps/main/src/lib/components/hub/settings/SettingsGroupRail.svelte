<script lang="ts">
	import { Dialog } from 'bits-ui';
	import { Check, ChevronDown } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import type { HubGroup } from '$lib/types';

	interface GroupItem {
		group: HubGroup;
		/** First section slug in this group — clicking the group navigates here. */
		firstSlug: string;
		count: number;
	}

	interface Props {
		groups: GroupItem[];
		basePath: string;
		activeGroup: HubGroup;
		labelKey: string;
	}
	let { groups, basePath, activeGroup, labelKey }: Props = $props();

	// Mobile: the rail collapses into a single trigger that opens a bottom-sheet
	// picker (same pattern as UrlTabs) — the vertical rail is md+ only.
	let pickerOpen = $state(false);
</script>

<!-- Mobile trigger: shows the active group, opens the picker sheet. -->
<button
	type="button"
	onclick={() => (pickerOpen = true)}
	aria-haspopup="dialog"
	class="mb-2 flex w-full shrink-0 cursor-pointer items-center justify-between gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2.5 text-sm font-medium text-[var(--color-text)] md:hidden"
>
	{t(`settings.group.${activeGroup}`)}
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
				{#each groups as item (item.group)}
					<a
						href={`${basePath}/${item.firstSlug}`}
						aria-current={activeGroup === item.group ? 'page' : undefined}
						onclick={() => (pickerOpen = false)}
						class="flex w-full items-center justify-between gap-2 rounded-[var(--radius)] px-4 py-3 text-sm font-medium transition-colors {activeGroup ===
						item.group
							? 'text-[var(--color-text)]'
							: 'text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)]'}"
					>
						{t(`settings.group.${item.group}`)}
						{#if activeGroup === item.group}
							<Check size={16} class="text-[var(--color-primary-text)]" />
						{/if}
					</a>
				{/each}
			</div>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>

<nav class="sticky top-[var(--nav-height)] hidden self-start md:block" aria-label={t(labelKey)}>
	{#each groups as item (item.group)}
		<a
			href={`${basePath}/${item.firstSlug}`}
			aria-current={activeGroup === item.group ? 'page' : undefined}
			class="block rounded-[var(--radius)] px-3 py-2 text-sm {activeGroup === item.group
				? 'bg-[var(--color-surface-subtle)] font-medium text-[var(--color-text)]'
				: 'text-[var(--color-muted)] hover:text-[var(--color-text)]'}"
		>
			{t(`settings.group.${item.group}`)}
		</a>
	{/each}
</nav>
