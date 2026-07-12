<script lang="ts">
	import type { PageData } from './$types';
	import { MetadataHead, JsonLd } from '$lib/seo';
	import { createOrganizationSchema } from '$lib/seo/structuredData';
	import { UniversityCard, UniversityFilters } from '$lib/universities';
	import { EmptyState, LoadingSkeleton } from '$lib/components/ui';
	import { GraduationCap } from 'lucide-svelte';

	let { data }: { data: PageData } = $props();

	// svelte-ignore state_referenced_locally
	let selectedCountry = $state(data.filters.country || '');
	// svelte-ignore state_referenced_locally
	let sortBy = $state(data.filters.sort || 'world_ranking');

	const organizationSchema = createOrganizationSchema();
</script>

<MetadataHead
	metadata={{
		title: 'Universities | JapaConnect',
		description: 'Explore universities for Nigerian students abroad. Compare rankings, tuition, programs, and more.'
	}}
/>

<JsonLd schema={organizationSchema} />

<div class="mx-auto max-w-6xl px-4 py-8">
	<header class="mb-8">
		<h1 class="text-3xl font-bold sm:text-4xl" style="color: var(--color-foreground)">
			Universities
		</h1>
		<p class="mt-2 text-lg" style="color: var(--color-muted)">
			Find the right university for your study abroad journey.
		</p>
	</header>

	<div class="mb-6">
		<UniversityFilters bind:country={selectedCountry} bind:sort={sortBy} countries={data.countries} />
	</div>

	{#if data.universities.length > 0}
		<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
			{#each data.universities as university (university.id)}
				<UniversityCard {university} />
			{/each}
		</div>

		<div class="mt-8 flex justify-center">
			<div class="flex gap-2">
				{#if Number(data.filters.page) > 1}
					<a
						href="?page={Number(data.filters.page) - 1}{data.filters.country ? `&country=${data.filters.country}` : ''}{data.filters.sort ? `&sort=${data.filters.sort}` : ''}"
						class="rounded-lg border px-4 py-2 text-sm font-medium transition-colors hover:bg-[var(--color-surface)]"
						style="border-color: var(--color-border); color: var(--color-foreground)"
					>
						Previous
					</a>
				{/if}
				<span
					class="rounded-lg px-4 py-2 text-sm font-medium"
					style="background-color: var(--color-primary); color: white"
				>
					Page {data.filters.page || 1}
				</span>
				{#if data.universities.length >= 12}
					<a
						href="?page={Number(data.filters.page || 1) + 1}{data.filters.country ? `&country=${data.filters.country}` : ''}{data.filters.sort ? `&sort=${data.filters.sort}` : ''}"
						class="rounded-lg border px-4 py-2 text-sm font-medium transition-colors hover:bg-[var(--color-surface)]"
						style="border-color: var(--color-border); color: var(--color-foreground)"
					>
						Next
					</a>
				{/if}
			</div>
		</div>
	{:else}
		<EmptyState
			icon={GraduationCap}
			title="No universities found"
			message="Try adjusting your filters to find universities."
		/>
	{/if}
</div>
