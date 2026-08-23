<script lang="ts">
	import { page } from '$app/stores';
</script>

<main class="mx-auto max-w-2xl p-8 text-center">
	<h1 class="text-4xl font-bold">{$page.status}</h1>
	<p class="mt-2 text-[var(--color-muted)]">
		{$page.error?.message ?? 'Something went wrong'}
	</p>
	<a href="/" class="mt-6 inline-block text-sm underline text-[var(--color-muted)] hover:text-white">
		Go home
	</a>
</main>
