<script lang="ts">
	import { ShieldAlert } from 'lucide-svelte';

	let { message = 'You do not have permission to access this page.' }: { message?: string } = $props();
</script>

<div class="flex flex-col items-center justify-center py-16 text-center">
	<div class="rounded-full p-4" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent)">
		<ShieldAlert class="h-10 w-10" style="color: var(--color-danger)" />
	</div>
	<h2 class="mt-4 text-lg font-semibold" style="color: var(--color-foreground)">Access Denied</h2>
	<p class="mt-2 max-w-sm text-sm" style="color: var(--color-muted)">{message}</p>
	<a href="/" class="mt-6 rounded-md px-4 py-2 text-sm font-medium text-white" style="background-color: var(--color-primary)">
		Back to Dashboard
	</a>
</div>
