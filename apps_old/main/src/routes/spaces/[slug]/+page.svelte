<script lang="ts">
	import { ArrowLeft, Users, Shield, Loader2, Rss } from 'lucide-svelte';
	import { api } from '$lib/api';
	import Reviews from '$lib/components/Reviews.svelte';

	let { data } = $props();
	let space = $derived(data.space);

	let joining = $state(false);
	let isJoined = $state(false);
	// svelte-ignore state_referenced_locally
	let memberCount = $state(space.member_count);
	let errorMessage = $state('');

	async function handleJoinLeave() {
		joining = true;
		errorMessage = '';
		try {
			const result = await api.joinSpace(space.slug);
			isJoined = result.joined;
			memberCount = result.member_count ?? memberCount;
		} catch {
			errorMessage = 'Failed to update space membership. Please try again.';
		} finally {
			joining = false;
		}
	}
</script>

<svelte:head>
	<title>{space.name} | JapaConnect</title>
	<meta name="description" content={space.description} />
</svelte:head>

<main class="mx-auto max-w-3xl px-4 py-8">
	<a
		href="/spaces"
		class="mb-6 inline-flex items-center gap-1.5 text-sm font-medium hover:underline"
		style="color: var(--color-muted)"
	>
		<ArrowLeft class="h-4 w-4" />
		Back to spaces
	</a>

	<div class="rounded-xl border p-6" style="border-color: var(--color-border); background: var(--color-surface);">
		<header class="flex items-start justify-between">
			<div class="flex-1">
				<h1 class="text-2xl font-bold" style="color: var(--color-foreground)">
					{space.name}
				</h1>
				<div class="mt-2 flex items-center gap-3 text-sm" style="color: var(--color-muted)">
					<span class="flex items-center gap-1">
						<Users class="h-4 w-4" />
						{memberCount} members
					</span>
					<span
						class="rounded-full px-2 py-0.5 text-xs"
						style="background-color: var(--color-background)"
					>
						{space.space_type}
					</span>
					<a
						href="/api/v1/spaces/{space.slug}/feed.xml"
						class="flex items-center gap-1 rounded-lg border px-2 py-1 text-xs font-medium transition-colors hover:opacity-80"
						style="border-color: var(--color-border); color: var(--color-muted)"
						target="_blank"
						rel="noopener noreferrer"
						title="Subscribe via RSS"
					>
						<Rss class="h-3 w-3" />
						RSS
					</a>
				</div>
			</div>

			<button
				onclick={handleJoinLeave}
				disabled={joining}
				class="flex items-center gap-1.5 rounded-lg px-4 py-2 text-sm font-medium transition-colors disabled:opacity-50"
				style="background-color: {isJoined ? 'var(--color-surface)' : 'var(--color-primary)'}; color: {isJoined ? 'var(--color-foreground)' : 'white'}; border: 1px solid {isJoined ? 'var(--color-border)' : 'transparent'}"
			>
				{#if joining}
					<Loader2 class="h-4 w-4 animate-spin" />
				{:else if isJoined}
					Leave
				{:else}
					Join
				{/if}
			</button>
		</header>

		<p class="mt-4 text-sm leading-relaxed" style="color: var(--color-foreground)">
			{space.description}
		</p>

		{#if errorMessage}
			<p class="mt-3 text-sm" style="color: var(--color-danger)">{errorMessage}</p>
		{/if}
	</div>

	{#if space.rules && space.rules.length > 0}
		<section class="mt-6 rounded-xl border p-6" style="border-color: var(--color-border); background: var(--color-surface);">
			<h2 class="flex items-center gap-2 text-lg font-semibold" style="color: var(--color-foreground)">
				<Shield class="h-5 w-5" style="color: var(--color-primary)" />
				Space Rules
			</h2>
			<ul class="mt-4 space-y-3">
				{#each space.rules as rule (rule.id)}
					<li class="rounded-lg px-4 py-3 text-sm" style="background-color: var(--color-background);">
						<span class="font-medium" style="color: var(--color-foreground)">{rule.title}</span>
						{#if rule.description}
							<p class="mt-1" style="color: var(--color-muted)">{rule.description}</p>
						{/if}
					</li>
				{/each}
			</ul>
		</section>
	{/if}

	<Reviews entityType="space" entityId={space.id} />
</main>
