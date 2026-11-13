<script lang="ts">
	import { t } from '#lib/i18n/index.js';
	import { ChevronDown, X } from 'lucide-svelte';
	import type { Component } from 'svelte';

	interface Props {
		typeName: string;
		typeIcon?: Component;
		browseOnly: boolean;
		selectSummary: string;
		onChangeType: () => void;
		/** Opens the "All categories" modal — the landing's tile grid reused
		 *  as the in-workspace type picker. */
		onAllCategories?: () => void;
	}

	let {
		typeName,
		typeIcon: TypeIcon,
		browseOnly,
		selectSummary,
		onChangeType,
		onAllCategories
	}: Props = $props();
</script>

<nav
	class="flex flex-wrap items-center gap-1.5 text-sm"
	aria-label={t('jce.compare.heading')}
>
	{#if onAllCategories}
		<button
			type="button"
			onclick={onAllCategories}
			class="inline-flex items-center gap-1.5 rounded-full border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1 font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
		>
			{t('jce.compare.landing.all_categories', 'All categories')}
			<ChevronDown size={13} class="text-[var(--color-muted)]" />
		</button>
	{/if}

	<!-- Step 1: type — opens the categories modal, same as "All categories" -->
	{#if onAllCategories}
		<button
			type="button"
			onclick={onAllCategories}
			class="inline-flex items-center gap-1.5 rounded-full border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-1 font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface)]"
			aria-label={t('jce.compare.step.switch_type')}
		>
			{#if TypeIcon}<TypeIcon size={13} />{/if}
			{typeName}
			<ChevronDown size={13} class="text-[var(--color-muted)]" />
		</button>
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

	<!-- Step 2: select / browse — pushed to the end of the row -->
	<span
		class="ms-auto inline-flex items-center rounded-full border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1 font-medium text-[var(--color-text)]"
	>
		{#if browseOnly}
			{t('jce.compare.step.browse')}
		{:else}
			{selectSummary || t('jce.compare.step.browse')}
		{/if}
	</span>

</nav>
