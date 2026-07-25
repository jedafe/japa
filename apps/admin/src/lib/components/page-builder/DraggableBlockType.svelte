<script lang="ts">
	import { createDraggable } from '@dnd-kit/svelte';
	import { t } from '$lib/stores/i18n.svelte';

	let { blockType }: { blockType: { type: string; name: string; description: string; icon: string; category: string; default_props: Record<string, unknown> } } = $props();

	const draggableInput = $state({
		get id() { return `palette-${blockType.type}`; },
		get data() { return { source: 'palette', blockType: blockType.type, defaultProps: blockType.default_props }; }
	});
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	const draggable = createDraggable(draggableInput as any);
</script>

<div
	{@attach draggable.attach}
	class="flex cursor-grab items-center gap-3 rounded-lg border px-3 py-2.5 transition-colors select-none hover:border-primary/50"
	class:opacity-50={draggable.isDragging}
	class:ring-2={draggable.isDragging}
	style="border-color: var(--color-border); background-color: var(--color-background)"
	role="button"
	tabindex="0"
>
	<div class="flex h-8 w-8 shrink-0 items-center justify-center rounded-md text-xs font-bold" style="background: color-mix(in srgb, var(--color-primary) 12%, transparent); color: var(--color-primary)">
		{blockType.name.charAt(0).toUpperCase()}
	</div>
	<div class="min-w-0 flex-1">
		<p class="truncate text-sm font-medium" style="color: var(--color-foreground)">{blockType.name}</p>
		<p class="truncate text-xs" style="color: var(--color-muted)">{blockType.description}</p>
	</div>
</div>
