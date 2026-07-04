<script lang="ts">
	import { Tag, Hash, MessageSquare } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import autoAnimate from '@formkit/auto-animate';

	let { data } = $props();
</script>

<svelte:head>
	<title>Tags | JapaConnect</title>
	<meta name="description" content="Browse topics and find discussions by tag on JapaConnect." />
</svelte:head>

<main class="mx-auto max-w-6xl px-4 py-8">
	<header class="mb-8">
		<h1 class="text-2xl font-bold sm:text-3xl" style="color: var(--color-foreground)">Tags</h1>
		<p class="mt-2 text-sm" style="color: var(--color-muted)">
			Browse topics and discover discussions by tag.
		</p>
	</header>

	{#if data.tags.length === 0}
		<EmptyState
			icon={Tag}
			title="No tags yet"
			message="Tags will appear as forum threads are created."
		/>
	{:else}
		<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3" use:autoAnimate>
			{#each data.tags as tag (tag.id)}
				<a
					href="/forum/tags"
					class="group flex items-center gap-3 rounded-xl border p-4 transition-all hover:shadow-md"
					style="border-color: var(--color-border); background-color: var(--color-surface)"
				>
					<div
						class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg"
						style="background-color: color-mix(in srgb, var(--color-primary) 10%, transparent)"
					>
						<Hash class="h-5 w-5" style="color: var(--color-primary)" />
					</div>
					<div class="flex-1 min-w-0">
						<h3
							class="text-sm font-semibold group-hover:underline"
							style="color: var(--color-foreground)"
						>
							{tag.name}
						</h3>
						<p class="mt-0.5 flex items-center gap-1 text-xs" style="color: var(--color-muted)">
							<MessageSquare class="h-3 w-3" />
							{tag.thread_count} {tag.thread_count === 1 ? 'thread' : 'threads'}
						</p>
					</div>
				</a>
			{/each}
		</div>
	{/if}
</main>
