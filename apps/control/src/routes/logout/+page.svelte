<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { api, setToken } from '$lib/api';
	import { clearUser } from '$lib/stores/auth.svelte';

	onMount(async () => {
		const token = localStorage.getItem('jc_token');
		if (token) {
			try {
				await api('/api/v1/auth/logout', {
					method: 'POST'
				});
			} catch {
				// Ignore — proceed to clear token regardless
			}
		}
		localStorage.removeItem('jc_token');
		setToken(null);
		clearUser();
		goto('/login');
	});
</script>

<p class="text-muted-foreground text-sm">Signing out...</p>
