<script lang="ts">
	import { Grid3X3, Plus, Loader2 } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { getToken } from '$lib/api';
	import { onMount } from 'svelte';

	interface Space {
		id: string;
		name: string;
		slug: string;
		description: string;
		space_type: string;
		member_count: number;
	}

	let spaces = $state<Space[]>([]);
	let loading = $state(true);
	let filter = $state('all');

	const spaceTypes = [
		{ value: 'all', label: 'All Spaces' },
		{ value: 'country', label: 'Countries' },
		{ value: 'interest', label: 'Interests' },
		{ value: 'profession', label: 'Professions' },
		{ value: 'university', label: 'Universities' }
	];

	onMount(async () => {
		try {
			const headers: Record<string, string> = { 'Content-Type': 'application/json' };
			const token = getToken();
			if (token) headers['Authorization'] = `Bearer ${token}`;
			const res = await fetch('/api/v1/spaces?limit=50', { headers });
			const body = await res.json();
			if (body.data) {
				spaces = body.data;
			}
		} catch {
			spaces = [];
		} finally {
			loading = false;
		}
	});

	const filteredSpaces = $derived(
		filter === 'all' ? spaces : spaces.filter((s) => s.space_type === filter)
	);
</script>

<svelte:head>
	<title>Spaces | JapaConnect</title>
	<meta name="description" content="Join communities and connect with others on JapaConnect." />
</svelte:head>

<main class="mx-auto max-w-6xl px-4 py-8">
	<header class="mb-8 flex items-start justify-between">
		<div>
			<h1 class="text-2xl font-bold sm:text-3xl" style="color: var(--color-foreground)">Spaces</h1>
			<p class="mt-2 text-sm" style="color: var(--color-muted)">
				Join communities that matter to you.
			</p>
		</div>
		<a
			href="/spaces/new"
			class="flex items-center gap-1.5 rounded-lg px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90"
			style="background-color: var(--color-primary)"
		>
			<Plus class="h-4 w-4" />
			Create space
		</a>
	</header>

	<!-- Filters -->
	<div class="mb-6 flex gap-2 overflow-x-auto pb-2">
		{#each spaceTypes as type}
			<button
				onclick={() => (filter = type.value)}
				class="shrink-0 rounded-lg px-3 py-1.5 text-sm font-medium transition-colors"
				style="background-color: {filter === type.value ? 'var(--color-primary)' : 'var(--color-surface)'}; color: {filter === type.value ? 'white' : 'var(--color-foreground)'}; border: 1px solid {filter === type.value ? 'transparent' : 'var(--color-border)'}"
			>
				{type.label}
			</button>
		{/each}
	</div>

	{#if loading}
		<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
			{#each Array(6) as _}
				<div
					class="h-40 animate-pulse rounded-xl"
					style="background-color: var(--color-surface)"
				></div>
			{/each}
		</div>
	{:else if filteredSpaces.length === 0}
		<EmptyState
			icon={Grid3X3}
			title="No spaces found"
			message="Be the first to create a space in this category."
		/>
	{:else}
		<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
			{#each filteredSpaces as space (space.id)}
				<a
					href="/spaces/{space.slug}"
					class="group rounded-xl border p-5 transition-all hover:shadow-md"
					style="border-color: var(--color-border); background: var(--color-surface)"
				>
					<h3
						class="text-base font-semibold group-hover:underline"
						style="color: var(--color-foreground)"
					>
						{space.name}
					</h3>
					<p class="mt-2 text-sm line-clamp-2" style="color: var(--color-muted)">
						{space.description}
					</p>
					<div class="mt-4 flex items-center justify-between text-xs" style="color: var(--color-muted)">
						<span class="rounded-full px-2 py-0.5" style="background-color: var(--color-background)">
							{space.space_type}
						</span>
						<span>{space.member_count} members</span>
					</div>
				</a>
			{/each}
		</div>
	{/if}
</main>
