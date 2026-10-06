<script lang="ts">
	// Difficulty badge — uses CSS custom properties, never hardcoded hex.
	// Ref: ADR-060 (difficulty tokens), skill (--level-beginner/intermediate/advanced).
	import { t } from '$lib/i18n';
	import type { TrackLevel } from '$lib/types';

	interface Props {
		level: TrackLevel;
		/** Compact mode for card corners (no background, just colored text). */
		compact?: boolean;
	}
	let { level, compact = false }: Props = $props();

	const colorVar = $derived(
		level === 'beginner'
			? 'var(--level-beginner)'
			: level === 'intermediate'
				? 'var(--level-intermediate)'
				: 'var(--level-advanced)'
	);
	const label = $derived(t(`level.${level}`));
</script>

{#if compact}
	<span
		class="inline-flex items-center gap-1 text-xs font-medium"
		style="color: {colorVar}"
	>
		<span class="h-1.5 w-1.5 rounded-full" style="background-color: {colorVar}"></span>
		{label}
	</span>
{:else}
	<span
		class="inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-medium"
		style="color: {colorVar}; background-color: color-mix(in srgb, {colorVar} 12%, transparent)"
	>
		<span class="h-1.5 w-1.5 rounded-full" style="background-color: {colorVar}"></span>
		{label}
	</span>
{/if}
