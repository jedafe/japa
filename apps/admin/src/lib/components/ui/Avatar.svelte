<script lang="ts">
	import clsx from 'clsx';

	let { src, alt = '', size = 'md', class: className = '' } = $props();

	const sizes: Record<string, string> = {
		sm: 'h-6 w-6',
		md: 'h-8 w-8',
		lg: 'h-10 w-10',
		xl: 'h-12 w-12',
	};
</script>

{#if src}
	<img {src} {alt} class={clsx('rounded-full object-cover', sizes[size], className)} />
{:else}
	<div class={clsx('flex items-center justify-center rounded-full', sizes[size], className)} style="background-color: var(--color-border)">
		<span class="text-sm font-medium" style="color: var(--color-muted)">{alt.charAt(0).toUpperCase()}</span>
	</div>
{/if}
