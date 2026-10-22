<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	import { t } from '$lib/stores/i18n.svelte';
	import { goto, invalidateAll } from '$app/navigation';
	import { clearUser } from '$lib/stores/auth.svelte';
	import { clearAccessToken } from '$lib/api/token';
	import { PUBLIC_JC_API_URL } from '$env/static/public';
	import { buildApiUrl } from '@japaconnect/api-url';

	(async () => {
		try {
			await fetch(buildApiUrl('/auth/logout', PUBLIC_JC_API_URL), {
				method: 'POST',
				credentials: 'include'
			});
		} catch (err) {
			console.warn('[logout] backend logout failed', err);
		}

		clearUser();
		clearAccessToken();
		await goto('/login', { invalidateAll: true });
	})();
</script>

<svelte:head>
	<title>Logging out... — {siteName()} Admin</title>
</svelte:head>

<div class="flex min-h-screen items-center justify-center">
	<p class="text-sm" style="color: var(--color-muted)">{t('auth.logout.logging_out')}</p>
</div>
