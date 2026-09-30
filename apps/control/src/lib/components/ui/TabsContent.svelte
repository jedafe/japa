<script lang="ts">
	import { Tabs } from 'bits-ui';
	import type { Snippet } from 'svelte';

	interface Props {
		value: string;
		class?: string;
		children?: Snippet;
	}

	let { value, class: className = '', children }: Props = $props();
</script>

<Tabs.Content {value} class="mt-4 {className}">
	{@render children?.()}
</Tabs.Content>
