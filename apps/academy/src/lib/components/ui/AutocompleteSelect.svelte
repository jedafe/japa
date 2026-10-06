<script lang="ts">
	// Thin wrapper over @japaconnect/ui injecting this app's i18n labels.
	import { AutocompleteSelect } from '@japaconnect/ui';
	import { t } from '$lib/i18n';
	import type { ComponentProps } from 'svelte';

	let {
		value = $bindable(''),
		noResultsLabel,
		clearLabel,
		loadingLabel,
		...rest
	}: ComponentProps<typeof AutocompleteSelect> = $props();
</script>

<AutocompleteSelect
	bind:value
	{...rest}
	noResultsLabel={noResultsLabel ?? t('common.no_results', 'No results found')}
	clearLabel={clearLabel ?? t('common.clear', 'Clear')}
	loadingLabel={loadingLabel ?? t('common.loading', 'Loading...')}
/>
