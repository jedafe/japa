<script lang="ts">
	import { goto } from '$app/navigation';
	import { t } from '$lib/i18n';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import type { DocAudience } from '$lib/types';
	import { AUDIENCE_LABEL_KEY } from '$lib/types';

	interface Props {
		accessibleAudiences: DocAudience[];
		activeAudience: DocAudience | null;
	}

	let { accessibleAudiences, activeAudience }: Props = $props();

	// Dropdown field — rendered through AutocompleteSelect (the shared
	// Autocomplete component) per the app-wide dropdown contract.
	const options = $derived(
		accessibleAudiences.map((a) => ({
			value: a,
			label: t(AUDIENCE_LABEL_KEY[a], a)
		}))
	);

	function pick(v: string) {
		if (!v || v === activeAudience) return;
		goto(`/docs/${v}`);
	}
</script>

<div class="switcher">
	<span class="label">{t('nav.audience')}</span>
	<AutocompleteSelect
		value={activeAudience ?? ''}
		{options}
		onValueChange={pick}
		placeholder={t('audience.pick_audience')}
		ariaLabel={t('nav.audience')}
		class="audience-ac"
	/>
</div>

<style>
	.switcher {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}
	.label {
		font-size: 0.6875rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--color-muted);
		font-weight: 500;
		white-space: nowrap;
	}
	.switcher :global(.audience-ac) {
		flex: 1;
		min-width: 0;
	}
	.switcher :global(.audience-ac .autocomplete-input-wrapper) {
		height: 2.375rem;
	}
</style>
