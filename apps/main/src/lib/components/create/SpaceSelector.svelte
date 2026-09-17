<script lang="ts">
	import { t } from '$lib/i18n';
	import { Plus, Star, X } from 'lucide-svelte';
	import Autocomplete from './Autocomplete.svelte';
	import type { AutocompleteItem } from './Autocomplete.svelte';
	import type { MeSpace } from '$lib/types';
	import { searchSpaces } from '$lib/api/create';
	import SpaceCreationModal from '$lib/components/spaces/SpaceCreationModal.svelte';

	// Section 14: content may belong to a primary space plus up to two
	// additional spaces. A named constant keeps the limit visible and
	// makes it easy to lift into a system setting later.
	const MAX_SPACES = 3;

	interface Props {
		spaces: MeSpace[];
		spaceIds: string[];
		onChange: (ids: string[]) => void;
		onfocus?: () => void;
	}

	let { spaces = $bindable(), spaceIds, onChange, onfocus }: Props = $props();

	let overLimitHint = $state(false);

	const selectedSpaces = $derived(
		spaceIds
			.map((id) => spaces.find((s) => s.id === id))
			.filter((s): s is MeSpace => s != null)
	);

	// Debounced fetcher — calls GET /me/spaces?q=… via the API client.
	async function fetcher(q: string): Promise<AutocompleteItem[]> {
		const res = await searchSpaces(q);
		const qLower = q.toLowerCase();
		return (res.data ?? [])
			.filter((s) => !spaceIds.includes(s.id))
			.map((s) => ({
				id: s.id,
				label: s.name,
				sublabel: s.is_primary ? '★' : undefined
			}))
			.sort((a, b) => {
				const aLabel = a.label.toLowerCase();
				const bLabel = b.label.toLowerCase();
				// Exact match first, then prefix, then contains; otherwise alpha.
				const aRank = aLabel === qLower ? 0 : aLabel.startsWith(qLower) ? 1 : aLabel.includes(qLower) ? 2 : 3;
				const bRank = bLabel === qLower ? 0 : bLabel.startsWith(qLower) ? 1 : bLabel.includes(qLower) ? 2 : 3;
				if (aRank !== bRank) return aRank - bRank;
				return aLabel.localeCompare(bLabel);
			});
	}

	// Initial suggestions — show up to 5 unselected spaces (primary first) when focused.
	async function initialFetcher(): Promise<AutocompleteItem[]> {
		return spaces
			.filter((s) => !spaceIds.includes(s.id))
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
		if (spaceIds.includes(item.id)) {
			// Selecting an already-selected space promotes it to primary.
			onChange([item.id, ...spaceIds.filter((id) => id !== item.id)]);
			return;
		}
		if (spaceIds.length >= MAX_SPACES) {
			overLimitHint = true;
			setTimeout(() => (overLimitHint = false), 2000);
			return;
		}
		onChange([...spaceIds, item.id]);
	}

	function handleRemove(id: string) {
		onChange(spaceIds.filter((s) => s !== id));
	}

	function handleSetPrimary(id: string) {
		onChange([id, ...spaceIds.filter((s) => s !== id)]);
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
		if (spaceIds.length >= MAX_SPACES) {
			// Newly created space becomes the primary, dropping the previous primary.
			onChange([newSpace.id, ...spaceIds.slice(1)]);
		} else {
			onChange([...spaceIds, newSpace.id]);
		}
	}
</script>

<div class="space-selector">
	<div class="space-selector-row">
		<Autocomplete
			placeholder={t('create.placeholder.space_search')}
			emptyMessage={t('create.placeholder.space_no_results')}
			{fetcher}
			{initialFetcher}
			selectedId={null}
			selectedLabel={null}
			onSelect={handleSelect}
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

	{#if overLimitHint}
		<p class="limit-hint" role="status">
			{t('create.space.limit_hint', `Select up to ${MAX_SPACES} spaces`, { count: MAX_SPACES })}
		</p>
	{/if}

	{#if selectedSpaces.length > 0}
		<div class="selected-spaces">
			{#each selectedSpaces as space, index (space.id)}
				<span class="space-chip" class:primary={index === 0}>
					<button
						type="button"
						class="primary-toggle"
						onclick={() => handleSetPrimary(space.id)}
						title={index === 0
							? t('create.space.primary', 'Primary space')
							: t('create.space.set_primary', 'Set as primary')}
						aria-pressed={index === 0}
					>
						<Star size={14} fill={index === 0 ? 'currentColor' : 'transparent'} />
					</button>
					<span class="chip-label">{space.name}</span>
					<button
						type="button"
						class="chip-remove"
						onclick={() => handleRemove(space.id)}
						title={t('create.space.remove', 'Remove')}
						aria-label={t('create.space.remove_aria', `Remove {name}`, { name: space.name })}
					>
						<X size={14} />
					</button>
				</span>
			{/each}
		</div>
	{/if}
</div>

<SpaceCreationModal bind:open={createModalOpen} onCreated={handleCreated} />

<style>
	.space-selector {
		display: flex;
		flex-direction: column;
		gap: 8px;
		width: 100%;
	}
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
		width: 2.375rem;
		min-width: 2.375rem;
		height: 2.375rem;
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
	.limit-hint {
		margin: 0;
		font-size: 0.75rem;
		color: var(--color-text-subtle);
	}
	.selected-spaces {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
	}
	.space-chip {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		padding: 4px 8px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		color: var(--color-text);
		font-size: 0.75rem;
	}
	.space-chip.primary {
		border-color: var(--color-primary);
		background: color-mix(in srgb, var(--color-primary) 8%, transparent);
	}
	.primary-toggle {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-width: 32px;
		min-height: 32px;
		padding: 0;
		border: none;
		border-radius: 50%;
		background: transparent;
		color: var(--color-primary);
		cursor: pointer;
	}
	.primary-toggle :global(svg) {
		width: 16px;
		height: 16px;
	}
	.chip-remove {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-width: 32px;
		min-height: 32px;
		padding: 0;
		border: none;
		border-radius: 50%;
		background: transparent;
		color: var(--color-text-subtle);
		cursor: pointer;
		transition: color 0.15s, background 0.15s;
	}
	.chip-remove:hover {
		color: var(--destructive);
		background: color-mix(in srgb, var(--destructive) 8%, transparent);
	}
	.chip-remove :global(svg) {
		width: 14px;
		height: 14px;
	}
	.chip-label {
		max-width: 120px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
</style>
