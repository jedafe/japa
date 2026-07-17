<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { Grid3X3, Plus, Loader2, Users } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { api } from '$lib/api';
	import AdSlot from '$lib/components/AdSlot.svelte';
	import autoAnimate from '@formkit/auto-animate';
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

	const TYPE_LABELS: Record<string, string> = {
		country: 'Countries',
		interest: 'Interests',
		profession: 'Professions',
		university: 'Universities'
	};

	const spaceTypes = $derived([
		{ value: 'all', label: 'All Spaces' },
		...Array.from(new Set(spaces.map(s => s.space_type)))
			.filter(Boolean)
			.sort()
			.map(type => ({
				value: type,
				label: TYPE_LABELS[type] ?? type.charAt(0).toUpperCase() + type.slice(1) + 's'
			}))
	]);

	onMount(async () => {
		try {
			spaces = await api.listSpaces({ page_size: 50 }) as Space[];
		} catch {
			spaces = [];
		} finally {
			loading = false;
		}
	});

	const filteredSpaces = $derived(
		filter === 'all' ? spaces : spaces.filter((s) => s.space_type === filter)
	);

	const typeColors: Record<string, string> = {
		country: '#3b82f6',
		interest: '#8b5cf6',
		profession: '#059669',
		university: '#d97706'
	};
</script>

<svelte:head>
	<title>Spaces | JapaConnect</title>
	<meta name="description" content="Join communities and connect with others on JapaConnect." />
</svelte:head>

<main class="mx-auto max-w-6xl px-4 py-8">
	<header class="mb-8 flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
		<div>
			<h1 class="text-2xl font-bold tracking-tight sm:text-3xl" style="color: var(--color-foreground)">{t('spaces.spaces')}</h1>
			<p class="mt-2 text-sm" style="color: var(--color-muted)">
				Join communities that matter to you.
			</p>
		</div>
		<a
			href="/spaces/new"
			class="btn btn-primary shrink-0"
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
				class="shrink-0 rounded-xl px-4 py-2 text-sm font-medium transition-all"
				style="background-color: {filter === type.value ? 'var(--color-primary)' : 'var(--color-surface)'}; color: {filter === type.value ? 'white' : 'var(--color-foreground)'}; border: 1px solid {filter === type.value ? 'transparent' : 'var(--color-border)'}; {filter === type.value ? 'box-shadow: var(--shadow-sm)' : ''}"
			>
				{type.label}
			</button>
		{/each}
	</div>

	{#if loading}
		<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
			{#each Array(6) as _, i}
				<div
					class="rounded-2xl border p-5"
					style="border-color: var(--color-border); background: var(--color-surface)"
				>
					<div class="skeleton mb-3 h-5 w-3/4 rounded-lg"></div>
					<div class="skeleton mb-2 h-4 w-full rounded-lg"></div>
					<div class="skeleton h-4 w-5/6 rounded-lg"></div>
					<div class="mt-4 flex gap-3">
						<div class="skeleton h-6 w-16 rounded-full"></div>
						<div class="skeleton h-6 w-20 rounded-full"></div>
					</div>
				</div>
			{/each}
		</div>
	{:else if filteredSpaces.length === 0}
		<EmptyState
			icon={Grid3X3}
			title="No spaces found"
			message="Be the first to create a space in this category."
		/>
	{:else}
		<!-- Ad slot -->
		<div class="mb-6">
			<AdSlot slotName="spaces_top" />
		</div>

		<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3" use:autoAnimate>
			{#each filteredSpaces as space, i (space.id)}
				<a
					href="/spaces/{space.slug}"
					class="card-interactive group relative overflow-hidden rounded-2xl border p-5"
					style="border-color: var(--color-border); background: var(--color-surface); animation: slide-up 0.3s ease-out {i * 50}ms both"
				>
					<!-- Accent bar -->
					<div
						class="absolute inset-x-0 top-0 h-0.5 opacity-0 transition-opacity group-hover:opacity-100"
						style="background: {typeColors[space.space_type] ?? 'var(--color-primary)'}"
					></div>

					<div class="flex items-start justify-between">
						<h3
							class="text-base font-semibold group-hover:underline"
							style="color: var(--color-foreground)"
						>
							{space.name}
						</h3>
						<span
							class="shrink-0 rounded-full px-2.5 py-0.5 text-xs font-medium capitalize"
							style="background: color-mix(in srgb, {typeColors[space.space_type] ?? 'var(--color-primary)'} 12%, transparent); color: {typeColors[space.space_type] ?? 'var(--color-primary)'}"
						>
							{space.space_type}
						</span>
					</div>
					<p class="mt-2 text-sm leading-relaxed line-clamp-2" style="color: var(--color-muted)">
						{space.description}
					</p>
					<div class="mt-4 flex items-center gap-2 text-sm font-medium" style="color: var(--color-muted)">
						<Users class="h-4 w-4" />
						<span>{space.member_count} members</span>
					</div>
				</a>
			{/each}
		</div>
	{/if}
</main>
