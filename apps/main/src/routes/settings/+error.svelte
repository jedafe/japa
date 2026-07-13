<script lang="ts">
	import { page } from '$app/state';
	import { AlertCircle, RefreshCw } from 'lucide-svelte';
</script>

<svelte:head>
	<title>Error | JapaConnect</title>
</svelte:head>

<main class="mx-auto max-w-md px-6 py-16 text-center">
	<div
		class="rounded-xl border p-8"
		style="border-color: var(--color-border); background: var(--color-surface)"
	>
		<div
			class="mx-auto flex h-12 w-12 items-center justify-center rounded-full"
			style="background-color: color-mix(in srgb, var(--color-danger) 10%, transparent)"
		>
			<AlertCircle class="h-6 w-6" style="color: var(--color-danger)" />
		</div>
		
		<h1 class="mt-4 text-xl font-bold" style="color: var(--color-foreground)">
			Unable to load settings
		</h1>
		
		<p class="mt-2 text-sm" style="color: var(--color-muted)">
			{#if page.status === 404}
				The settings page you're looking for doesn't exist.
			{:else if page.status === 403}
				You don't have permission to view these settings.
			{:else if page.status === 500}
				Something went wrong on our end. Please try again later.
			{:else}
				An unexpected error occurred. Please try again.
			{/if}
		</p>
		
		{#if page.error?.message}
			<p class="mt-2 text-xs" style="color: var(--color-muted)">
				{page.error.message}
			</p>
		{/if}
		
		<div class="mt-6 flex justify-center gap-3">
			<button
				onclick={() => window.location.reload()}
				class="btn btn-primary"
			>
				<RefreshCw class="h-4 w-4" />
				Try Again
			</button>
			<a href="/" class="btn btn-secondary">
				Go Home
			</a>
		</div>
	</div>
</main>
