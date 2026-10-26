<script lang="ts">
	// Renders a lucide icon by name, resolving the chunk lazily so blocks
	// never ship the full icon set. Renders a fixed-size placeholder until
	// the icon arrives — same footprint, no layout shift.
	import type { Component } from 'svelte';
	import { resolveIcon } from './icons';

	interface Props {
		name?: string;
		size?: number;
		strokeWidth?: number;
		class?: string;
	}

	let { name, size = 16, strokeWidth, class: cls = '' }: Props = $props();

	let Icon = $state<Component | undefined>();
	$effect(() => {
		Icon = undefined;
		const n = name;
		if (!n) return;
		let cancelled = false;
		resolveIcon(n).then((c) => {
			if (!cancelled) Icon = c;
		});
		return () => {
			cancelled = true;
		};
	});
</script>

{#if Icon}
	<Icon {size} {strokeWidth} class={cls} />
{:else}
	<span style="display:inline-block;width:{size}px;height:{size}px" class={cls} aria-hidden="true"></span>
{/if}
