// @japaconnect/ui — shared Svelte 5 component library for all JapaConnect apps.
// Per-app ui/ directories keep thin wrappers that inject their i18n `t()`.

export { default as Autocomplete } from './Autocomplete.svelte';
export type { AutocompleteItem } from './Autocomplete.svelte';
export { default as AutocompleteSelect } from './AutocompleteSelect.svelte';
export { default as EmptyState } from './EmptyState.svelte';
export { default as LoadingSkeleton } from './LoadingSkeleton.svelte';
export { default as Avatar } from './Avatar.svelte';
export { default as Modal } from './Modal.svelte';
export { default as Menu } from './Menu.svelte';
export { default as MenuItem } from './MenuItem.svelte';
export { default as MenuSeparator } from './MenuSeparator.svelte';

export { staticAutocompleteFetcher, autocompleteItems } from './autocomplete';
export { resolveLucideIcon } from './icon';
export type { IconComponent } from './icon';

// Reusable TipTap rich-text editor (extracted from apps/main PostEditor —
// sections/15-tiptap-rich-text.md). Feature nodes, suggestion pickers, and
// modals are opt-in via props so each app keeps its own API surface.
export { default as RichTextEditor } from './editor/RichTextEditor.svelte';
export { default as EmojiPicker } from './editor/EmojiPicker.svelte';
export { default as EditorInsertModal } from './editor/EditorInsertModal.svelte';
export { default as LinkPreviewCard } from './editor/LinkPreviewCard.svelte';
export { buildEditorExtensions } from './editor/extensions';
export type { EditorExtensionOptions } from './editor/extensions';
export { createSuggestionRenderer } from './editor/suggestionRenderer';
export { SLASH_BLOCKS } from './editor/slashMenu';
export type { SlashBlock, SlashBlockContext } from './editor/slashMenu';
export { LinkPreviewNode } from './editor/linkPreviewNode';
export { VideoNode } from './editor/nodes/VideoNode';
export type {
	EditorFeatures,
	EditorMode,
	EditorSearchHit,
	EditorTranslator,
	LinkPreviewData
} from './editor/types';
