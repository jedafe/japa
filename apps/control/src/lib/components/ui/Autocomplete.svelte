<script module lang="ts">
	export type { AutocompleteItem } from '@japaconnect/ui';
</script>

<script lang="ts">
	// Thin wrapper over @japaconnect/ui injecting this app's i18n labels.
	import { Autocomplete } from '@japaconnect/ui';
	import { t } from '$lib/stores/i18n.svelte';
	import type { ComponentProps } from 'svelte';

	let {
		clearLabel,
		loadingLabel,
		...rest
	}: ComponentProps<typeof Autocomplete> = $props();
</script>

<Autocomplete
	{...rest}
	clearLabel={clearLabel ?? t('common.clear', 'Clear')}
	loadingLabel={loadingLabel ?? t('common.loading', 'Loading...')}
/>
