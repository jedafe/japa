<script lang="ts">
	import '../app.css';
	import Sidebar from '$lib/components/Sidebar.svelte';

	let { children } = $props();
</script>

<svelte:head>
	<title>JapaConnect Admin</title>
</svelte:head>

<div class="flex h-screen overflow-hidden">
	<Sidebar />

	<main class="flex-1 overflow-y-auto">
		<div class="mx-auto max-w-7xl px-4 py-6 sm:px-6 md:px-8">
			{@render children()}
		</div>
	</main>
</div>
