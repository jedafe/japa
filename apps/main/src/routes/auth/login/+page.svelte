<script lang="ts">
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
	<p style="color: var(--color-muted)">Redirecting to login...</p>
</main>
