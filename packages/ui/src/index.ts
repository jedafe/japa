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
