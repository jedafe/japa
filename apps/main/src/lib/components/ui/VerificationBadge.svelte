<script lang="ts">
	import { BadgeCheck } from 'lucide-svelte';

	interface Props {
		size?: number;
	}

	let { size = 14 }: Props = $props();
</script>

<BadgeCheck
	{size}
	class="inline-block shrink-0 text-[var(--color-primary)]"
	aria-label="Verified"
/>
