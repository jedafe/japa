<script lang="ts">
	import { EmptyState } from '@japaconnect/ui';
	import type { ComponentProps } from 'svelte';

	let props: ComponentProps<typeof EmptyState> = $props();
</script>

<EmptyState {...props} />
