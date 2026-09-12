<script lang="ts">
	import { Heart } from 'lucide-svelte';

	interface Props {
		count?: number;
		active?: boolean;
		onToggle?: (active: boolean) => void;
		class?: string;
		ariaLabel?: string;
		title?: string;
	}

	let {
		count = 0,
		active = $bindable(false),
		onToggle,
		class: classProp,
		ariaLabel,
		title
	}: Props = $props();

	function formatCount(n: number): string {
		if (n >= 1_000_000) return (n / 1_000_000).toFixed(1).replace(/\.0$/, '') + 'M';
		if (n >= 1_000) return (n / 1_000).toFixed(1).replace(/\.0$/, '') + 'k';
		return String(n);
	}

	function onClick(e: Event) {
		e.stopPropagation();
		active = !active;
		onToggle?.(active);
	}
</script>

<button
	type="button"
	onclick={onClick}
	title={title}
	class={classProp ??
		'inline-flex h-9 items-center gap-1.5 rounded-full border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 text-sm font-medium text-[var(--color-text)] transition hover:bg-[var(--color-surface-hover)]'}
	class:text-[var(--color-primary)]={active}
	aria-label={ariaLabel}
	aria-pressed={active}
>
	<Heart
		size={16}
		class={active ? 'fill-current' : ''}
		aria-hidden="true"
	/>
	<span class="min-w-[1.25ch] tabular-nums">{formatCount(count)}</span>
</button>
