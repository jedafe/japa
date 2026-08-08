<script lang="ts">
	// A drag handle for resizing adjacent panes. Supports both horizontal
	// (col-resize, left/right panes) and vertical (row-resize, top/bottom
	// panes) orientations.
	//
	// `direction` controls how mouse movement maps to size changes:
	//   'right' — the pane is ABOVE or to the LEFT; dragging down/right grows it.
	//   'left'  — the pane is BELOW or to the RIGHT; dragging down/right shrinks it.
	import { GripVertical, GripHorizontal } from 'lucide-svelte';

	let {
		paneId,
		size,
		orientation = 'horizontal',
		direction = 'right',
		minSize = 200,
		maxSize = 600,
		onResize,
	}: {
		paneId: string;
		/** Current size of the pane being resized (px). */
		size: number;
		/** 'vertical' = handle between top/bottom panes (drag up/down).
		 *  'horizontal' = handle between left/right panes (drag left/right). */
		orientation?: 'vertical' | 'horizontal';
		direction?: 'left' | 'right';
		minSize?: number;
		maxSize?: number;
		onResize: (paneId: string, newSize: number) => void;
	} = $props();

	let dragging = $state(false);

	function onPointerDown(e: PointerEvent) {
		e.preventDefault();
		dragging = true;
		(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
	}

	function onPointerMove(e: PointerEvent) {
		if (!dragging) return;
		// For vertical orientation we use movementY; for horizontal, movementX.
		const movement = orientation === 'vertical' ? e.movementY : e.movementX;
		// direction='right': dragging in the positive direction grows the pane.
		// direction='left': dragging in the positive direction shrinks the pane.
		const delta = direction === 'right' ? movement : -movement;
		const next = Math.max(minSize, Math.min(maxSize, size + delta));
		if (next !== size) onResize(paneId, next);
	}

	function onPointerUp(e: PointerEvent) {
		dragging = false;
		(e.currentTarget as HTMLElement).releasePointerCapture(e.pointerId);
	}

	const isVertical = $derived(orientation === 'vertical');
</script>

<button
	type="button"
	aria-label="Resize panel"
	onpointerdown={onPointerDown}
	onpointermove={onPointerMove}
	onpointerup={onPointerUp}
	onpointercancel={onPointerUp}
	class="flex shrink-0 items-center justify-center border-0 p-0 transition-colors hover:bg-[var(--color-primary)]/20 {isVertical ? 'h-1 w-full cursor-row-resize' : 'h-full w-1 cursor-col-resize'}"
	style={dragging ? 'background: color-mix(in srgb, var(--color-primary) 30%, transparent)' : 'background: var(--color-border)'}
>
	{#if isVertical}
		<GripHorizontal class="h-3 w-4 opacity-30" />
	{:else}
		<GripVertical class="h-4 w-3 opacity-30" />
	{/if}
</button>
