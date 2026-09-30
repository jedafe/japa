<script lang="ts">
	// Thin wrapper over @japaconnect/ui injecting this app's i18n labels.
	import { Modal } from '@japaconnect/ui';
	import { t } from '$lib/i18n';
	import type { ComponentProps } from 'svelte';

	let {
		open = $bindable(false),
		closeLabel,
		...rest
	}: ComponentProps<typeof Modal> = $props();
</script>

<Modal bind:open {...rest} closeLabel={closeLabel ?? t('common.close', 'Close')} />
