<script lang="ts">
	// Star rating display — uses --level-intermediate token (amber), never hex.
	import { Star } from 'lucide-svelte';

	interface Props {
		value: number;
		/** Max stars (default 5). */
		max?: number;
		small?: boolean;
	}
	let { value, max = 5, small = false }: Props = $props();

	const size = $derived(small ? 12 : 16);
	// Round to nearest half for display.
	const rounded = $derived(Math.round(value * 2) / 2);
</script>

<div class="inline-flex items-center gap-0.5" aria-label={`${rounded} out of ${max}`}>
	{#each Array(max) as _, i}
		<Star
			size={size}
			class={i + 1 <= rounded ? 'text-[var(--level-intermediate)]' : 'text-[var(--color-border)]'}
			fill={i + 1 <= rounded ? 'currentColor' : 'none'}
		/>
	{/each}
</div>
