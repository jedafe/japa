<script lang="ts">
	// Thin wrapper over @japaconnect/ui injecting this app's i18n labels.
	import { MultiAutocomplete } from '@japaconnect/ui';
	import { t } from '#lib/stores/i18n.svelte.js';
	import type { ComponentProps } from 'svelte';

	let {
		clearLabel,
		loadingLabel,
		removeLabel,
		...rest
	}: ComponentProps<typeof MultiAutocomplete> = $props();
</script>

<MultiAutocomplete
	{...rest}
	clearLabel={clearLabel ?? t('common.clear', 'Clear')}
	loadingLabel={loadingLabel ?? t('common.loading', 'Loading...')}
	removeLabel={removeLabel ?? t('common.remove', 'Remove')}
/>
