<script lang="ts">
	import { Search, ChevronDown } from 'lucide-svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';

	let {
		country = $bindable(''),
		sort = $bindable('world_ranking')
	}: {
		country?: string;
		sort?: string;
	} = $props();

	const countries = [
		{ value: '', label: 'All Countries' },
		{ value: 'Canada', label: 'Canada' },
		{ value: 'United Kingdom', label: 'United Kingdom' },
		{ value: 'United States', label: 'United States' },
		{ value: 'Germany', label: 'Germany' },
		{ value: 'Ireland', label: 'Ireland' },
		{ value: 'Australia', label: 'Australia' },
		{ value: 'Netherlands', label: 'Netherlands' },
		{ value: 'New Zealand', label: 'New Zealand' }
	];

	const sortOptions = [
		{ value: 'world_ranking', label: 'World Ranking' },
		{ value: 'country_ranking', label: 'Country Ranking' },
		{ value: 'name', label: 'Name' },
		{ value: 'member_count', label: 'Members' },
		{ value: 'follower_count', label: 'Followers' }
	];

	let searchQuery = $state('');

	function updateFilters() {
		const params = new URLSearchParams(page.url.searchParams);
		if (country) params.set('country', country);
		else params.delete('country');
		if (sort) params.set('sort', sort);
		else params.delete('sort');
		if (searchQuery) params.set('q', searchQuery);
		else params.delete('q');
		goto(`?${params.toString()}`, { replaceState: true });
	}

	function handleCountryChange(e: Event) {
		country = (e.target as HTMLSelectElement).value;
		updateFilters();
	}

	function handleSortChange(e: Event) {
		sort = (e.target as HTMLSelectElement).value;
		updateFilters();
	}
</script>

<div class="flex flex-wrap gap-3">
	<div class="relative min-w-[200px] flex-1">
		<Search
			class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2"
			style="color: var(--color-muted)"
		/>
		<input
			type="text"
			placeholder="Search universities..."
			bind:value={searchQuery}
			onchange={updateFilters}
			class="w-full rounded-lg border py-2.5 pl-10 pr-4 text-sm outline-none transition-colors focus:border-[var(--color-primary)]"
			style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
		/>
	</div>

	<div class="relative">
		<select
			value={country}
			onchange={handleCountryChange}
			class="appearance-none rounded-lg border py-2.5 pl-4 pr-10 text-sm outline-none transition-colors focus:border-[var(--color-primary)]"
			style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
		>
			{#each countries as c}
				<option value={c.value}>{c.label}</option>
			{/each}
		</select>
		<ChevronDown
			class="pointer-events-none absolute right-3 top-1/2 h-4 w-4 -translate-y-1/2"
			style="color: var(--color-muted)"
		/>
	</div>

	<div class="relative">
		<select
			value={sort}
			onchange={handleSortChange}
			class="appearance-none rounded-lg border py-2.5 pl-4 pr-10 text-sm outline-none transition-colors focus:border-[var(--color-primary)]"
			style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
		>
			{#each sortOptions as s}
				<option value={s.value}>{s.label}</option>
			{/each}
		</select>
		<ChevronDown
			class="pointer-events-none absolute right-3 top-1/2 h-4 w-4 -translate-y-1/2"
			style="color: var(--color-muted)"
		/>
	</div>
</div>
