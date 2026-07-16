<script lang="ts">
	import { onMount } from 'svelte';
	import { clearUser } from '$lib/stores/auth.svelte';

	onMount(async () => {
		try {
			// Call logout endpoint
			await fetch('/api/v1/auth/logout', {
				method: 'POST',
				credentials: 'include'
			}).catch(() => {});
		} catch {
			// Ignore errors
		}
		
		// Clear client state
		clearUser();
		
		// Clear cookie by setting it to expire
		document.cookie = 'jc_token=; path=/; expires=Thu, 01 Jan 1970 00:00:00 GMT';
		
		// Redirect to login
		window.location.href = '/login';
	});
</script>

<svelte:head>
	<title>Logging out... — JapaConnect Admin</title>
</svelte:head>

<div class="flex min-h-screen items-center justify-center">
	<p class="text-sm" style="color: var(--color-muted)">Logging out...</p>
</div>
