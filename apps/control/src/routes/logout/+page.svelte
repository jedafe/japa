<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { api, setToken } from '$lib/api';
	import { clearUser } from '$lib/stores/auth.svelte';
	import { t } from '$lib/stores/i18n.svelte';

	onMount(async () => {
		// Call logout to revoke the refresh token server-side. The
		// httpOnly jc_refresh cookie is cleared by the backend. We
		// don't need to read from localStorage — the token is in-memory.
		try {
			await api('/api/v1/auth/logout', {
				method: 'POST'
			});
		} catch {
			// Ignore — proceed to clear token regardless
		}
		setToken(null);
		clearUser();
		goto('/login');
	});
</script>

<p class="text-muted-foreground text-sm">{t('common.signing_out', 'Signing out...')}</p>
