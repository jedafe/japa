<script lang="ts">
	import { LoadingSkeleton } from '@japaconnect/ui';
	import type { ComponentProps } from 'svelte';

	let { class: klass = 'h-4 w-full', ...rest }: ComponentProps<typeof LoadingSkeleton> = $props();
</script>

<LoadingSkeleton class={klass} {...rest} />
