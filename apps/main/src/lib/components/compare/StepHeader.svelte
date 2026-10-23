<script lang="ts">
	import { t } from '$lib/i18n';
	import { ChevronRight, SlidersHorizontal, X } from 'lucide-svelte';
	import type { Component } from 'svelte';

	interface Props {
		typeName: string;
		typeIcon?: Component;
		showNarrow: boolean;
		narrowOpen: boolean;
		narrowSummary: string;
		narrowRequired: boolean;
		browseOnly: boolean;
		selectSummary: string;
		onChangeType: () => void;
		onToggleNarrow: () => void;
	}

	let {
		typeName,
		typeIcon: TypeIcon,
		showNarrow,
		narrowOpen,
		narrowSummary,
		narrowRequired,
		browseOnly,
		selectSummary,
		onChangeType,
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
