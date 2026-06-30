<script lang="ts">
	import { Users, Search, User } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { api, type UserProfile } from '$lib/api';
	import autoAnimate from '@formkit/auto-animate';
	import { onMount } from 'svelte';

	let members = $state<UserProfile[]>([]);
	let loading = $state(true);
	let searchQuery = $state('');

	async function fetchMembers(query?: string) {
		loading = true;
		try {
			members = await api.listUsers({ limit: 50, search: query });
		} catch {
			members = [];
		} finally {
			loading = false;
		}
	}

	function handleSearch(e: Event) {
		e.preventDefault();
		fetchMembers(searchQuery.trim() || undefined);
	}

	onMount(() => fetchMembers());
</script>

<svelte:head>
	<title>Members | JapaConnect</title>
	<meta name="description" content="Discover members of the JapaConnect community." />
</svelte:head>

<main class="mx-auto max-w-6xl px-4 py-8">
	<header class="mb-8">
		<h1 class="text-2xl font-bold sm:text-3xl" style="color: var(--color-foreground)">Members</h1>
		<p class="mt-2 text-sm" style="color: var(--color-muted)">
			Discover people in the JapaConnect community.
		</p>
	</header>

	<form onsubmit={handleSearch} class="mb-6 flex gap-2">
		<div class="relative flex-1">
			<Search class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2" style="color: var(--color-muted)" />
			<input
				type="text"
				bind:value={searchQuery}
				placeholder="Search by name or username..."
				class="w-full rounded-lg border py-2 pl-10 pr-4 text-sm outline-none focus:border-[var(--color-primary)]"
				style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
			/>
		</div>
		<button
			type="submit"
			class="rounded-lg px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90"
			style="background-color: var(--color-primary)"
		>
			Search
		</button>
	</form>

	{#if loading}
		<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
			{#each Array(6) as _}
				<div
					class="h-32 animate-pulse rounded-xl"
					style="background-color: var(--color-surface)"
				></div>
			{/each}
		</div>
	{:else if members.length === 0}
		<EmptyState
			icon={Users}
			title="No members found"
			message={searchQuery ? 'Try a different search term.' : 'Members will appear here.'}
		/>
	{:else}
		<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3" use:autoAnimate>
			{#each members as member (member.id)}
				<a
					href="/profile/{member.username}"
					class="group rounded-xl border p-5 transition-all hover:shadow-md"
					style="border-color: var(--color-border); background: var(--color-surface)"
				>
					<div class="flex items-center gap-3">
						{#if member.avatar_url}
							<img
								src={member.avatar_url}
								alt={member.display_name}
								class="h-10 w-10 rounded-full object-cover"
							/>
						{:else}
							<div
								class="flex h-10 w-10 items-center justify-center rounded-full text-sm font-bold text-white"
								style="background-color: var(--color-primary)"
							>
								{(member.display_name || member.username)?.charAt(0)?.toUpperCase() ?? '?'}
							</div>
						{/if}
						<div class="min-w-0 flex-1">
							<h3
								class="truncate text-base font-semibold group-hover:underline"
								style="color: var(--color-foreground)"
							>
								{member.display_name || member.username}
							</h3>
							<p class="truncate text-xs" style="color: var(--color-muted)">
								@{member.username}
							</p>
						</div>
					</div>
					{#if member.bio}
						<p class="mt-3 text-sm line-clamp-2" style="color: var(--color-muted)">
							{member.bio}
						</p>
					{/if}				</a>
			{/each}
		</div>
	{/if}
</main>
