<script lang="ts">
	import { t } from '$lib/i18n';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import type { ComposerOrganization } from '$lib/types';

	interface Props {
		/** Orgs the current user is a member of. Empty = no post-as-org available. */
		orgs: ComposerOrganization[];
		/** Selected org id, or null for "post as yourself" (personal). */
		selectedId: string | null;
		onSelect: (id: string | null) => void;
		onfocus?: () => void;
	}

	let { orgs, selectedId, onSelect, onfocus }: Props = $props();

	const selectedOrg = $derived(orgs.find((o) => o.id === selectedId) ?? null);

	// "Post as yourself" is a special item with id="" (personal). It is
	// included in the Autocomplete item list so the user can search for it.
	const SELF_ID = '__self__';

	// Autocomplete fetchers — filter the already-loaded orgs client-side.
	// The "Post as yourself" option is always included at the top.
	function fetchOrgs(q: string): Promise<AutocompleteItem[]> {
		const selfLabel = t('create.post_as_yourself', 'Post as yourself');
		const query = q.toLowerCase();
		const selfItem: AutocompleteItem = { id: SELF_ID, label: selfLabel };
		if (query === '') {
			return Promise.resolve([
				selfItem,
				...orgs.map((o) => ({ id: o.id, label: o.name, sublabel: o.member_role }))
			]);
		}
		const matchesSelf = selfLabel.toLowerCase().includes(query);
		const filtered = orgs.filter((o) => o.name.toLowerCase().includes(query));
		const items = filtered.map((o) => ({ id: o.id, label: o.name, sublabel: o.member_role }));
		return Promise.resolve(matchesSelf ? [selfItem, ...items] : items);
	}
	function fetchOrgsInitial(): Promise<AutocompleteItem[]> {
		return fetchOrgs('');
	}

	const selectedLabel = $derived(
		selectedOrg
			? selectedOrg.name
			: t('create.post_as_yourself', 'Post as yourself')
	);
	// Map the internal SELF_ID back to null (personal) on select/clear.
	function handleSelect(item: AutocompleteItem) {
		onfocus?.();
		onSelect(item.id === SELF_ID ? null : item.id);
	}
</script>

{#if orgs.length > 0}
	<div class="org-selector">
		<span class="org-label" id="org-selector-label">{t('create.author_label', 'Author')}</span>
		<Autocomplete
			placeholder={t('create.post_as_yourself', 'Post as yourself')}
			emptyMessage={t('common.no_results', 'No results found')}
			fetcher={fetchOrgs}
			initialFetcher={fetchOrgsInitial}
			selectedId={selectedOrg ? selectedId : SELF_ID}
			selectedLabel={selectedLabel}
			onSelect={handleSelect}
			onClear={() => { onfocus?.(); onSelect(null); }}
			onfocus={onfocus}
		/>
	</div>
{/if}

<style>
	.org-selector {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}
	.org-label {
		font-size: 0.75rem;
		font-weight: 500;
		color: var(--color-muted);
	}
</style>
