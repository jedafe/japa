<script lang="ts">
	import { page } from '$app/stores';

	const user = $derived($page.data.user);
</script>

<svelte:head>
	<title>Japa Comparison Engine</title>
	<meta name="description" content="Compare countries, pathways, schools, scholarships, jobs, and more on JapaConnect." />
</svelte:head>

<main class="mx-auto max-w-4xl p-8">
	<h1 class="text-3xl font-bold">Japa Comparison Engine</h1>
	<p class="mt-4 text-[var(--color-muted)]">
		Compare countries, immigration pathways, schools, scholarships, jobs, costs, salaries, and settlement opportunities.
	</p>
	{#if user}
		<p class="mt-4 text-sm text-[var(--color-muted)]">Signed in as {user.username}</p>
	{:else}
		<p class="mt-4 text-sm text-[var(--color-muted)]">Sign in to create comparisons.</p>
	{/if}

	<div class="mt-8">
		<a href="/compare" class="inline-flex items-center rounded-[var(--radius-button)] bg-white px-4 py-2 text-sm font-medium text-black hover:bg-neutral-200">
			Start comparing
		</a>
	</div>
</main>
