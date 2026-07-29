<script lang="ts">
	import type { SpacerDividerProps } from '../types';

	interface Props {
		props: SpacerDividerProps;
	}
	let { props }: Props = $props();

	const height = $derived(props.height_px ?? 32);
</script>

<div style="height: {height}px" class="mx-auto max-w-4xl px-6">
	{#if props.show_line}
		<div class="h-full flex items-center">
			<div class="w-full border-t border-[var(--color-border)]"></div>
		</div>
	{/if}
</div>
