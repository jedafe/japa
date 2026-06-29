<script lang="ts">
	import { Briefcase, Plus, Search } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { page } from '$app/state';
	import autoAnimate from '@formkit/auto-animate';

	let { data } = $props();

	let searchQuery = $state(data.filters.search || '');

	function handleSearch(e: Event) {
		e.preventDefault();
		const url = new URL(window.location.href);
		if (searchQuery.trim()) {
			url.searchParams.set('search', searchQuery.trim());
		} else {
			url.searchParams.delete('search');
		}
		url.searchParams.delete('offset');
		window.location.href = url.toString();
	}
</script>

<svelte:head>
	<title>Professional Profiles | JapaConnect</title>
	<meta name="description" content="Explore professional profiles of Nigerians preparing for relocation abroad." />
</svelte:head>

<main class="mx-auto max-w-6xl px-4 py-8">
	<header class="mb-8 flex items-start justify-between">
		<div>
			<h1 class="text-2xl font-bold sm:text-3xl" style="color: var(--color-foreground)">Professional Profiles</h1>
			<p class="mt-2 text-sm" style="color: var(--color-muted)">
				Discover professionals ready for their next chapter abroad.
			</p>
		</div>
		<a
			href="/professional/new"
			class="flex items-center gap-1.5 rounded-lg px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90"
			style="background-color: var(--color-primary)"
		>
			<Plus class="h-4 w-4" />
			Create profile
		</a>
	</header>

	<!-- Search -->
	<form onsubmit={handleSearch} class="mb-6 flex gap-2">
		<div class="relative flex-1">
			<Search class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2" style="color: var(--color-muted)" />
			<input
				type="text"
				bind:value={searchQuery}
				placeholder="Search by name or skill..."
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

	{#if data.profiles.length === 0}
		<EmptyState
			icon={Briefcase}
			title="No professional profiles yet"
			message="Be the first to create a professional profile."
		/>
	{:else}
		<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3" use:autoAnimate>
			{#each data.profiles as profile (profile.id)}
				<a
					href="/professional/{profile.slug}"
					class="group rounded-xl border p-5 transition-all hover:shadow-md"
					style="border-color: var(--color-border); background: var(--color-surface)"
				>
					<div class="flex items-center gap-3">
						<div
							class="flex h-10 w-10 items-center justify-center rounded-full text-sm font-bold text-white"
							style="background-color: var(--color-primary)"
						>
							{profile.name.charAt(0).toUpperCase()}
						</div>
						<div class="min-w-0 flex-1">
							<h3
								class="truncate text-base font-semibold group-hover:underline"
								style="color: var(--color-foreground)"
							>
								{profile.name}
							</h3>
							<p class="truncate text-sm" style="color: var(--color-muted)">
								{profile.headline}
							</p>
						</div>
					</div>

					{#if profile.skills.length > 0}
						<div class="mt-3 flex flex-wrap gap-1">
							{#each profile.skills.slice(0, 5) as skill}
								<span
									class="rounded-full px-2 py-0.5 text-xs"
									style="background-color: var(--color-background); color: var(--color-muted)"
								>
									{skill}
								</span>
							{/each}
							{#if profile.skills.length > 5}
								<span class="text-xs" style="color: var(--color-muted)">+{profile.skills.length - 5}</span>
							{/if}
						</div>
					{/if}

					<div class="mt-4 flex items-center justify-between text-xs" style="color: var(--color-muted)">
						<span
							class="rounded-full px-2 py-0.5"
							style="background-color: {profile.is_public ? 'color-mix(in srgb, var(--color-success, #16a34a) 15%, transparent)' : 'var(--color-background)'}; color: {profile.is_public ? 'var(--color-success, #16a34a)' : 'var(--color-muted)'}"
						>
							{profile.is_public ? 'Public' : 'Private'}
						</span>
					</div>
				</a>
			{/each}
		</div>

		{#if data.totalPages > 1}
			<div class="mt-6 flex items-center justify-center text-sm" style="color: var(--color-muted)">
				Showing {data.profiles.length} of {data.total} profiles
			</div>
		{/if}
	{/if}
</main>
