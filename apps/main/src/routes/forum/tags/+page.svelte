<script lang="ts">
	import { Tag, ArrowLeft, MessageSquare } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { api } from '$lib/api';
	import { onMount } from 'svelte';

	let tags = $state<Array<{ id: string; name: string; slug: string; thread_count: number }>>([]);
	let loading = $state(true);

	onMount(async () => {
		try {
			tags = await api.getForumTags();
		} catch {
			tags = [];
		} finally {
			loading = false;
		}
	});
</script>

<svelte:head>
	<title>Forum Tags | JapaConnect</title>
	<meta name="description" content="Browse forum tags and find discussions by topic." />
</svelte:head>

<main class="mx-auto max-w-4xl px-4 py-8">
	<header class="mb-8">
		<a
			href="/forum"
			class="mb-4 inline-flex items-center gap-1 text-sm transition-colors hover:underline"
			style="color: var(--color-muted)"
		>
			<ArrowLeft class="h-4 w-4" />
			Back to Forum
		</a>
		<h1 class="text-2xl font-bold sm:text-3xl" style="color: var(--color-foreground)">Forum Tags</h1>
		<p class="mt-2 text-sm" style="color: var(--color-muted)">
			Browse discussions by topic.
		</p>
	</header>

	{#if loading}
		<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
			{#each Array(6) as _}
				<div
					class="h-20 animate-pulse rounded-xl"
					style="background-color: var(--color-surface)"
				></div>
			{/each}
		</div>
	{:else if tags.length === 0}
		<EmptyState
			icon={Tag}
			title="No tags yet"
			message="Tags will appear as threads are created."
		/>
	{:else}
		<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
			{#each tags as tag (tag.id)}
				<a
					href="/forum?tag={tag.slug}"
					class="group flex items-center gap-3 rounded-xl border p-4 transition-all hover:shadow-md"
					style="border-color: var(--color-border); background-color: var(--color-surface)"
				>
					<div
						class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg"
						style="background-color: color-mix(in srgb, var(--color-primary) 10%, transparent)"
					>
						<Tag class="h-5 w-5" style="color: var(--color-primary)" />
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
