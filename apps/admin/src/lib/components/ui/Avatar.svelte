<script lang="ts">
	import { Avatar } from '@japaconnect/ui';
	import type { ComponentProps } from 'svelte';

	let props: ComponentProps<typeof Avatar> = $props();
</script>

<Avatar {...props} />
