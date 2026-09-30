<script lang="ts">
	// Avatar — circular image or initials fallback.
	interface Props {
		src?: string | null;
		alt?: string;
		size?: 'sm' | 'md' | 'lg' | 'xl';
		class?: string;
	}

	let { src, alt = '', size = 'md', class: className = '' }: Props = $props();

	const sizes: Record<string, string> = {
		sm: 'h-6 w-6',
		md: 'h-8 w-8',
		lg: 'h-10 w-10',
		xl: 'h-12 w-12'
	};
</script>

{#if src}
	<img {src} {alt} class="rounded-full object-cover {sizes[size] ?? sizes.md} {className}" />
{:else}
	<div
		class="flex items-center justify-center rounded-full {sizes[size] ?? sizes.md} {className}"
		style="background-color: var(--color-border, var(--border, #e5e7eb))"
	>
		<span class="text-sm font-medium" style="color: var(--color-muted-foreground, var(--muted-foreground, var(--color-muted, var(--muted, #6b7280))))">
			{alt.charAt(0).toUpperCase()}
		</span>
	</div>
{/if}
