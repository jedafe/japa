<script lang="ts">
	import { t } from '$lib/i18n';
	import { Plus } from 'lucide-svelte';
	import Autocomplete from './Autocomplete.svelte';
	import type { AutocompleteItem } from './Autocomplete.svelte';
	import type { MeSpace } from '$lib/types';
	import { searchSpaces } from '$lib/api/create';
	import SpaceCreationModal from '$lib/components/spaces/SpaceCreationModal.svelte';

	interface Props {
		spaces: MeSpace[];
		selectedId: string | null;
		onSelect: (id: string) => void;
		onfocus?: () => void;
	}

	let { spaces = $bindable(), selectedId, onSelect, onfocus }: Props = $props();

	// The selected space object — used to display the label without a re-fetch.
	const selectedSpace = $derived(spaces.find((s) => s.id === selectedId) ?? null);

	// Debounced fetcher — calls GET /me/spaces?q=… via the API client.
	async function fetcher(q: string): Promise<AutocompleteItem[]> {
		const res = await searchSpaces(q);
		return (res.data ?? []).map((s) => ({
			id: s.id,
			label: s.name,
			sublabel: s.is_primary ? '★' : undefined
		}));
	}

	// Initial suggestions — show up to 5 spaces (primary first) when the field is focused.
	async function initialFetcher(): Promise<AutocompleteItem[]> {
		return spaces
			.slice()
			.sort((a, b) => (a.is_primary ? -1 : b.is_primary ? 1 : 0))
			.slice(0, 5)
			.map((s) => ({
				id: s.id,
				label: s.name,
				sublabel: s.is_primary ? '★' : undefined
			}));
	}

	function handleSelect(item: AutocompleteItem) {
		onSelect(item.id);
	}

	function handleClear() {
		onSelect('');
	}

	// ---- "Create new space" modal ----
	let createModalOpen = $state(false);

	function openCreateModal() {
		createModalOpen = true;
	}

	function handleCreated(newSpace: { id: string; slug: string; name: string }) {
		const meSpace: MeSpace = {
			id: newSpace.id,
			slug: newSpace.slug,
			name: newSpace.name,
			is_primary: false
		};
		spaces = [...spaces, meSpace];
		onSelect(newSpace.id);
	}
</script>

<div class="space-selector-row">
	<Autocomplete
		placeholder={t('create.placeholder.space_search')}
		emptyMessage={t('create.placeholder.space_no_results')}
		{fetcher}
		{initialFetcher}
		{selectedId}
		selectedLabel={selectedSpace?.name ?? null}
		selectedBadge={selectedSpace?.is_primary ? '★' : null}
		onSelect={handleSelect}
		onClear={handleClear}
		{onfocus}
	/>
	<button
		type="button"
		class="space-create-btn"
		onclick={openCreateModal}
		title={t('create.space.create_new', 'Create new space')}
		aria-label={t('create.space.create_new', 'Create new space')}
	>
		<Plus size={16} />
	</button>
</div>

<SpaceCreationModal bind:open={createModalOpen} onCreated={handleCreated} />

<style>
	.space-selector-row {
		display: flex;
		gap: 6px;
		align-items: stretch;
	}
	.space-selector-row > :global(*) {
		min-width: 0;
	}
	.space-create-btn {
		display: flex;
		align-items: center;
		justify-content: center;
		flex-shrink: 0;
		width: 40px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		color: var(--color-text-subtle);
		cursor: pointer;
		transition: border-color 0.15s, color 0.15s, background 0.15s;
	}
	.space-create-btn:hover {
		border-color: var(--color-primary);
		color: var(--color-primary);
		background: color-mix(in srgb, var(--color-primary) 8%, transparent);
	}
</style>
