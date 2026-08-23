<script lang="ts">
	import '../app.css';
	import { page } from '$app/stores';
	import { seedCsrfToken } from '$lib/api/csrf.svelte';

	let { children } = $props();

	// Seed the browser-side CSRF store from server-rendered data.
	$effect(() => {
		seedCsrfToken($page.data.csrfToken ?? null);
	});
</script>

<div class="min-h-screen bg-[var(--color-bg)] text-[var(--color-text)]">
	{@render children()}
</div>
