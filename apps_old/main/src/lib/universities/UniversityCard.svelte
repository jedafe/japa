<script lang="ts">
	import { MapPin, Users, Star, ExternalLink } from 'lucide-svelte';

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
			top_programs?: string[];
		};
	} = $props();
</script>

<a
	href="/universities/{university.slug}"
	class="card-interactive group block overflow-hidden rounded-2xl border"
	style="border-color: var(--color-border); background-color: var(--color-surface)"
>
	<div class="p-5">
		<div class="flex items-start gap-4">
			{#if university.logo_url}
				<img
					src={university.logo_url}
					alt="{university.name} logo"
					class="h-14 w-14 rounded-xl object-contain shadow-sm"
				/>
			{:else}
				<div
					class="flex h-14 w-14 items-center justify-center rounded-xl text-xl font-bold text-white shadow-sm"
					style="background: var(--gradient-primary)"
				>
					{university.name.charAt(0)}
				</div>
			{/if}

			<div class="min-w-0 flex-1">
				<h3 class="truncate text-lg font-semibold leading-snug group-hover:underline" style="color: var(--color-foreground)">
					{university.name}
				</h3>
				{#if university.country}
					<div class="mt-1.5 flex items-center gap-1 text-sm" style="color: var(--color-muted)">
						<MapPin class="h-3.5 w-3.5" />
						{university.country}
					</div>
				{/if}
			</div>
		</div>

		{#if university.world_ranking || university.country_ranking}
			<div class="mt-4 flex gap-4 text-sm">
				{#if university.world_ranking}
					<div class="flex items-center gap-1.5">
						<Star class="h-4 w-4" style="color: var(--color-warning)" />
						<span style="color: var(--color-muted)">World:</span>
						<span class="font-semibold">#{university.world_ranking}</span>
					</div>
				{/if}
				{#if university.country_ranking}
					<div class="flex items-center gap-1.5">
						<span style="color: var(--color-muted)">Country:</span>
						<span class="font-semibold">#{university.country_ranking}</span>
					</div>
				{/if}
			</div>
		{/if}

		{#if university.top_programs && university.top_programs.length > 0}
			<div class="mt-3 flex flex-wrap gap-1.5">
				{#each university.top_programs.slice(0, 3) as program}
					<span
						class="rounded-full px-2.5 py-0.5 text-xs font-medium"
						style="background: color-mix(in srgb, var(--color-primary) 10%, transparent); color: var(--color-primary)"
					>
						{program}
					</span>
				{/each}
				{#if university.top_programs.length > 3}
					<span class="text-xs font-medium" style="color: var(--color-muted)">
						+{university.top_programs.length - 3} more
					</span>
				{/if}
			</div>
		{/if}

		<div class="mt-4 flex items-center justify-between border-t pt-4" style="border-color: var(--color-border)">
			<div class="flex items-center gap-3 text-sm" style="color: var(--color-muted)">
				<span class="flex items-center gap-1">
					<Users class="h-4 w-4" />
					{university.member_count ?? 0}
				</span>
				<span>{university.follower_count ?? 0} followers</span>
			</div>
			<span
				class="flex items-center gap-1 text-sm font-medium transition-all group-hover:gap-2"
				style="color: var(--color-primary)"
			>
				View
				<ExternalLink class="h-3.5 w-3.5" />
			</span>
		</div>
	</div>
</a>
