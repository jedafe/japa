<script lang="ts">
	import { ShieldAlert } from 'lucide-svelte';
	let { requiredPermission = '' }: { requiredPermission?: string } = $props();
</script>

<div class="flex flex-col items-center justify-center py-16 text-center">
	<ShieldAlert class="mb-4 h-12 w-12 text-red-500" />
	<h2 class="text-xl font-semibold text-gray-900 dark:text-gray-100">Access Denied</h2>
	<p class="mt-2 text-sm text-gray-600 dark:text-gray-400">
		{#if requiredPermission}
			You need the <code class="rounded bg-gray-100 px-1 dark:bg-gray-700">{requiredPermission}</code> permission to access this page.
		{:else}
			You do not have permission to access this page.
		{/if}
	</p>
	<a href="/" class="mt-6 text-sm font-medium text-blue-600 hover:underline dark:text-blue-400">
		Return to Dashboard
	</a>
</div>
