<script lang="ts">
	import { AutocompleteSelect } from '@japaconnect/ui';
	import type { ComponentProps } from 'svelte';

	let { value = $bindable(''), ...rest }: ComponentProps<typeof AutocompleteSelect> = $props();
</script>

<AutocompleteSelect bind:value {...rest} />
