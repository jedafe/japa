<script lang="ts">
	import { t } from '#lib/i18n/index.js';
	import { DropdownMenu } from 'bits-ui';
	import { ChevronDown, ChevronRight, SlidersHorizontal, X } from 'lucide-svelte';
	import type { Component } from 'svelte';

	interface SiblingType {
		slug: string;
		name: string;
		icon?: Component;
	}

	interface Props {
		typeName: string;
		typeIcon?: Component;
		siblings?: SiblingType[];
		showNarrow: boolean;
		narrowOpen: boolean;
		narrowSummary: string;
		narrowRequired: boolean;
		browseOnly: boolean;
		selectSummary: string;
		onChangeType: () => void;
		onSwitchType?: (slug: string) => void;
		onToggleNarrow: () => void;
	}

	let {
		typeName,
		typeIcon: TypeIcon,
		siblings = [],
		showNarrow,
		narrowOpen,
		narrowSummary,
		narrowRequired,
		browseOnly,
		selectSummary,
		onChangeType,
		onSwitchType,
		onToggleNarrow
	}: Props = $props();

	let narrowChipLabel = $derived(
		narrowSummary ||
			(narrowRequired ? t('jce.compare.step.narrow') : t('jce.compare.step.narrow_optional'))
	);
</script>

<nav
	class="flex flex-wrap items-center gap-1.5 text-sm"
	aria-label={t('jce.compare.heading')}
>
	<!-- Step 1: type -->
	{#if siblings.length > 0 && onSwitchType}
		<!-- Same-family switcher: pick a sibling type inline; the footer
			item keeps the old "change type" path back to the landing. -->
		<DropdownMenu.Root>
			<DropdownMenu.Trigger
				class="inline-flex items-center gap-1.5 rounded-full border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-1 font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface)]"
				aria-label={t('jce.compare.step.switch_type')}
			>
				{#if TypeIcon}<TypeIcon size={13} />{/if}
				{typeName}
				<ChevronDown size={13} class="text-[var(--color-muted)]" />
			</DropdownMenu.Trigger>
			<DropdownMenu.Portal>
				<DropdownMenu.Content
					align="start"
					class="z-50 w-56 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] py-1 shadow-lg"
				>
					{#each siblings as sibling (sibling.slug)}
						<DropdownMenu.Item
							onclick={() => onSwitchType?.(sibling.slug)}
							class="flex w-full cursor-pointer items-center gap-2 px-3 py-1.5 text-start text-sm text-[var(--color-text-subtle)] data-[highlighted]:bg-[var(--color-surface-subtle)] data-[highlighted]:text-[var(--color-text)]"
						>
							{#if sibling.icon}
								{@const SiblingIcon = sibling.icon}
								<SiblingIcon size={13} />
							{/if}
							{sibling.name}
						</DropdownMenu.Item>
					{/each}
					<DropdownMenu.Item
						onclick={onChangeType}
						class="mt-1 flex w-full cursor-pointer items-center border-t border-[var(--color-border)] px-3 py-1.5 text-start text-sm text-[var(--color-primary-text)] data-[highlighted]:bg-[var(--color-surface-subtle)]"
					>
						{t('jce.compare.step.all_types')}
					</DropdownMenu.Item>
				</DropdownMenu.Content>
			</DropdownMenu.Portal>
		</DropdownMenu.Root>
	{:else}
		<span
			class="inline-flex items-center gap-1.5 rounded-full border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-1 font-medium text-[var(--color-text)]"
		>
			{#if TypeIcon}<TypeIcon size={13} />{/if}
			{typeName}
			<button
				type="button"
				onclick={onChangeType}
				class="rounded-full p-0.5 text-[var(--color-muted)] transition-colors hover:text-[var(--color-text)]"
				aria-label={t('jce.compare.step.change_type')}
			>
				<X size={13} />
			</button>
		</span>
	{/if}

	{#if showNarrow}
		<ChevronRight size={14} class="text-[var(--color-muted)] rtl:-scale-x-100" />
		<!-- Step 2: narrow -->
		<button
			type="button"
			onclick={onToggleNarrow}
			class="inline-flex items-center gap-1.5 rounded-full border border-[var(--color-border)] px-3 py-1 font-medium text-[var(--color-text)] transition-colors {narrowOpen ? 'bg-[var(--color-surface-subtle)]' : 'bg-[var(--color-surface)] hover:bg-[var(--color-surface-subtle)]'}"
			aria-expanded={narrowOpen}
			aria-label={t('jce.compare.step.edit_narrow')}
		>
			<SlidersHorizontal size={13} />
			{narrowChipLabel}
		</button>
	{/if}

	<ChevronRight size={14} class="text-[var(--color-muted)] rtl:-scale-x-100" />
	<!-- Step 3: select / browse -->
	<span
		class="inline-flex items-center rounded-full border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1 font-medium text-[var(--color-text)]"
	>
		{#if browseOnly}
			{t('jce.compare.step.browse')}
		{:else}
			{selectSummary || t('jce.compare.step.browse')}
		{/if}
	</span>
</nav>
