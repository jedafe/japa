<script lang="ts">
	import { LoadingSkeleton } from '@japaconnect/ui';
	import type { ComponentProps } from 'svelte';

	let props: ComponentProps<typeof LoadingSkeleton> = $props();
</script>

<LoadingSkeleton {...props} />
