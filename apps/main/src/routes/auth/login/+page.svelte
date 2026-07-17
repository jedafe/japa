<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';

	onMount(() => {
		goto('/login', { replaceState: true });
	});
</script>

<svelte:head>
	<title>Redirecting... | JapaConnect</title>
</svelte:head>

<main class="flex min-h-screen items-center justify-center">
	<p style="color: var(--color-muted)">{t('auth.login.redirecting_to_login')}</p>
</main>
