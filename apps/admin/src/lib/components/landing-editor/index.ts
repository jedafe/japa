// Ref: japaconnect-engineering/sections/16-page-builder.md (dnd-kit foundation)
// Shared dnd-kit editor components for the Page Builder. Used by
// /page-builder/pages/[id] (and, until removal, parity-tested against the
// legacy /landing/[slug] editor). Built on @dnd-kit/svelte + the
// @japaconnect/landing-blocks block library.
export { default as SortableSection } from './SortableSection.svelte';
export { default as DraggableBlockType } from './DraggableBlockType.svelte';
export { default as ContainerChildren } from './ContainerChildren.svelte';
export { default as ResizeHandle } from './ResizeHandle.svelte';
