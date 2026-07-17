<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { MapPin, Star, Users, ExternalLink } from 'lucide-svelte';

	let {
		university
	}: {
		university: {
			id: string;
			slug: string;
			name: string;
			description?: string;
			logo_url?: string;
			country?: string;
			world_ranking?: number;
			country_ranking?: number;
			member_count?: number;
			follower_count?: number;
			rating?: number;
			review_count?: number;
		};
	} = $props();
</script>

<div class="rounded-xl border p-6" style="border-color: var(--color-border); background-color: var(--color-surface)">
	<div class="flex flex-col items-start gap-6 sm:flex-row sm:items-center">
		{#if university.logo_url}
			<img
				src={university.logo_url}
				alt="{university.name} logo"
				class="h-20 w-20 rounded-xl object-contain"
			/>
		{:else}
			<div
				class="flex h-20 w-20 items-center justify-center rounded-xl text-3xl font-bold"
				style="background-color: var(--color-primary); color: white"
			>
				{university.name.charAt(0)}
			</div>
		{/if}

		<div class="flex-1">
			<h1 class="text-2xl font-bold sm:text-3xl" style="color: var(--color-foreground)">
				{university.name}
			</h1>
			{#if university.country}
				<div class="mt-2 flex items-center gap-2" style="color: var(--color-muted)">
					<MapPin class="h-4 w-4" />
					<span>{university.country}</span>
				</div>
			{/if}
		</div>
	</div>

	<div class="mt-6 grid grid-cols-2 gap-4 sm:grid-cols-4">
		{#if university.world_ranking}
			<div class="rounded-lg p-3 text-center" style="background-color: var(--color-background)">
				<div class="flex items-center justify-center gap-1 text-sm" style="color: var(--color-muted)">
					<Star class="h-4 w-4" style="color: var(--color-warning)" />
					World Rank
				</div>
				<div class="mt-1 text-xl font-bold" style="color: var(--color-foreground)">
					#{university.world_ranking}
				</div>
			</div>
		{/if}

		{#if university.country_ranking}
			<div class="rounded-lg p-3 text-center" style="background-color: var(--color-background)">
				<div class="text-sm" style="color: var(--color-muted)">{t('lib.universities.UniversityHeader.svelte.country_rank')}</div>
				<div class="mt-1 text-xl font-bold" style="color: var(--color-foreground)">
					#{university.country_ranking}
				</div>
			</div>
		{/if}

		{#if university.rating}
			<div class="rounded-lg p-3 text-center" style="background-color: var(--color-background)">
				<div class="text-sm" style="color: var(--color-muted)">{t('lib.universities.UniversityHeader.svelte.rating')}</div>
				<div class="mt-1 text-xl font-bold" style="color: var(--color-foreground)">
					{university.rating.toFixed(1)}
					<span class="text-sm font-normal" style="color: var(--color-muted)">
						({university.review_count ?? 0})
					</span>
				</div>
			</div>
		{/if}

		{#if university.member_count}
			<div class="rounded-lg p-3 text-center" style="background-color: var(--color-background)">
				<div class="flex items-center justify-center gap-1 text-sm" style="color: var(--color-muted)">
					<Users class="h-4 w-4" />
					Members
				</div>
				<div class="mt-1 text-xl font-bold" style="color: var(--color-foreground)">
					{university.member_count}
				</div>
			</div>
		{/if}
	</div>
</div>
