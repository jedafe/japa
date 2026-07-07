<script lang="ts">
	import { Building2, Search, BadgeCheck } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { api } from '$lib/api';
	import autoAnimate from '@formkit/auto-animate';

	let { data } = $props();

	let organisations = $state(data.organisations);
	let loading = $state(false);
	let searchQuery = $state('');

	async function fetchOrganisations(_query?: string) {
		loading = true;
		try {
			const result = await api.listOrganizations(50, 1);
			organisations = result.organizations;
		} catch {
			organisations = data.organisations;
		} finally {
			loading = false;
		}
	}

	function handleSearch(e: Event) {
		e.preventDefault();
		fetchOrganisations(searchQuery.trim() || undefined);
	}
</script>

<svelte:head>
	<title>Organisations | JapaConnect</title>
	<meta name="description" content="Browse professional organisations and verified profiles on JapaConnect." />
</svelte:head>

<main class="mx-auto max-w-6xl px-4 py-8">
	<header class="mb-8">
		<h1 class="text-2xl font-bold sm:text-3xl" style="color: var(--color-foreground)">Organisations</h1>
		<p class="mt-2 text-sm" style="color: var(--color-muted)">
			Browse professional organisations and verified profiles.
		</p>
	</header>

	<form onsubmit={handleSearch} class="mb-6 flex gap-2">
		<div class="relative flex-1">
			<Search class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2" style="color: var(--color-muted)" />
			<input
				type="text"
				bind:value={searchQuery}
				placeholder="Search organisations..."
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
					class="h-36 animate-pulse rounded-xl"
					style="background-color: var(--color-surface)"
				></div>
			{/each}
		</div>
	{:else if organisations.length === 0}
		<EmptyState
			icon={Building2}
			title="No organisations found"
			message={searchQuery ? 'Try a different search term.' : 'Organisations will appear here.'}
		/>
	{:else}
		<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3" use:autoAnimate>
			{#each organisations as org (org.id)}
				<a
					href="/organisations/{org.slug}"
					class="group rounded-xl border p-5 transition-all hover:shadow-md"
					style="border-color: var(--color-border); background: var(--color-surface)"
				>
					<div class="flex items-center gap-3">
						<div
							class="flex h-10 w-10 items-center justify-center rounded-full text-sm font-bold text-white"
							style="background-color: var(--color-primary)"
						>
							{org.name?.charAt(0)?.toUpperCase() ?? '?'}
						</div>
						<div class="min-w-0 flex-1">
							<div class="flex items-center gap-1.5">
								<h3
									class="truncate text-base font-semibold group-hover:underline"
									style="color: var(--color-foreground)"
								>
									{org.name}
								</h3>
								{#if org.is_public}
									<BadgeCheck class="h-4 w-4 shrink-0" style="color: var(--color-primary)" />
								{/if}
							</div>
							<p class="truncate text-sm" style="color: var(--color-muted)">
								{org.headline}
							</p>
						</div>
					</div>
					{#if org.member_count !== undefined}
						<div class="mt-3 flex flex-wrap gap-1">
							<span
								class="rounded-full px-2 py-0.5 text-xs"
								style="background-color: var(--color-background); color: var(--color-muted)"
							>
								{org.member_count.toLocaleString()} members
							</span>
						</div>
					{/if}
				</a>
			{/each}
		</div>
	{/if}
</main>
