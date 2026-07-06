<script lang="ts">
	let { online = false, size = 'md' }: { online?: boolean; size?: 'sm' | 'md' | 'lg' } = $props();

	const sizes: Record<string, string> = {
		sm: 'h-2 w-2',
		md: 'h-2.5 w-2.5',
		lg: 'h-3 w-3'
	};
</script>

<span class="relative inline-flex" title={online ? 'Online' : 'Offline'}>
	<span
		class="{sizes[size]} inline-block rounded-full {online ? 'online-pulse' : ''}"
		style="background-color: {online ? 'var(--color-success)' : 'var(--color-muted)'}"
	></span>
</span>

<style>
	.online-pulse {
		box-shadow: 0 0 0 0 color-mix(in srgb, var(--color-success) 40%, transparent);
		animation: online-ping 2s cubic-bezier(0, 0, 0.2, 1) infinite;
	}

	@keyframes online-ping {
		0% {
			box-shadow: 0 0 0 0 color-mix(in srgb, var(--color-success) 40%, transparent);
		}
		75%, 100% {
			box-shadow: 0 0 0 6px color-mix(in srgb, var(--color-success) 0%, transparent);
		}
	}
</style>
