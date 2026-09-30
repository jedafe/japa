<script module lang="ts">
	export type { AutocompleteItem } from '@japaconnect/ui';
</script>

<script lang="ts">
	import { Autocomplete } from '@japaconnect/ui';
	import type { ComponentProps } from 'svelte';

	let props: ComponentProps<typeof Autocomplete> = $props();
</script>

<Autocomplete {...props} />
