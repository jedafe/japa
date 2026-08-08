<script lang="ts">
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Button } from "$lib/components/ui/button";
	import { DataTable } from "$lib/components/ui/data-table";
	import type { ActionItem } from "$lib/components/ui/data-table";
	import { TableSkeleton } from "$lib/components/ui/skeleton";
	import { Search, Globe, MapPin, ChevronRight, Loader2, Plus } from "lucide-svelte";
	import { geoApi } from "$lib/api";
	import { goto } from "$app/navigation";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";

	type GeoContext = {
		country_code: string;
		country_name: string;
		city: string;
		subdivision: string;
		time_zone: string;
		latitude: number;
		longitude: number;
		is_valid: boolean;
	};

	type GeoSpace = {
		id: string;
		name: string;
		slug: string;
		level: string;
		geo_code?: string | null;
		member_count?: number;
	};

	let activeTab = $state<'lookup' | 'hierarchy'>('lookup');

	// IP Lookup
	let ipQuery = $state('');
	let lookupResult = $state<GeoContext | null>(null);
	let lookupLoading = $state(false);
	let lookupError = $state('');

	const canLookup = $derived(hasPermission('geo.lookup'));
	const canManageSpaces = $derived(hasPermission('spaces.admin') || hasPermission('*'));

	function createGeoUrl(level: string, parentId?: string): string {
		const params = new URLSearchParams({ level, from: '/geo' });
		if (parentId) params.set('parent_id', parentId);
		return `/spaces/new?${params.toString()}`;
	}

	// Hierarchy browser
	let countries = $state<GeoSpace[]>([]);
	let states = $state<GeoSpace[]>([]);
	let cities = $state<GeoSpace[]>([]);
	let selectedCountry = $state<GeoSpace | null>(null);
	let selectedState = $state<GeoSpace | null>(null);
	let hierarchyLoading = $state(false);
	let hierarchyError = $state('');

	// Search queries — DataTable delegates filtering to the parent via onsearch.
	let countrySearch = $state('');
	let stateSearch = $state('');
	let citySearch = $state('');

	const filteredCountries = $derived(
		countrySearch
			? countries.filter(c =>
				c.name.toLowerCase().includes(countrySearch.toLowerCase()) ||
				(c.geo_code ?? '').toLowerCase().includes(countrySearch.toLowerCase())
			)
			: countries
	);
	const filteredStates = $derived(
		stateSearch
			? states.filter(s =>
				s.name.toLowerCase().includes(stateSearch.toLowerCase()) ||
				(s.geo_code ?? '').toLowerCase().includes(stateSearch.toLowerCase())
			)
			: states
	);
	const filteredCities = $derived(
		citySearch
			? cities.filter(c =>
				c.name.toLowerCase().includes(citySearch.toLowerCase())
			)
			: cities
	);

	const countryColumns = [
		{ key: 'name', label: t('geo.country', 'Country'), sortable: true, render: (v: string) => `<span class="font-medium">${v}</span>` },
		{ key: 'geo_code', label: t('geo.code', 'ISO Code'), sortable: true, render: (v: string) => `<span class="font-mono text-xs">${v ?? '—'}</span>` },
		{ key: 'member_count', label: t('geo.members', 'Members'), sortable: true, render: (v: number) => `<span class="text-xs">${(v ?? 0).toLocaleString()}</span>` }
	];

	const countryRowActions: ActionItem[] = [
		{ label: t('geo.view_states', 'States'), icon: ChevronRight, onclick: (row: GeoSpace) => selectCountry(row) }
	];

	const stateColumns = [
		{ key: 'name', label: t('geo.state', 'State/Province'), sortable: true, render: (v: string) => `<span class="font-medium">${v}</span>` },
		{ key: 'geo_code', label: t('geo.code', 'ISO Code'), render: (v: string) => `<span class="font-mono text-xs">${v ?? '—'}</span>` }
	];

	const stateRowActions: ActionItem[] = [
		{ label: t('geo.view_cities', 'Cities'), icon: ChevronRight, onclick: (row: GeoSpace) => selectState(row) }
	];

	const cityColumns = [
		{ key: 'name', label: t('geo.city', 'City'), sortable: true, render: (v: string) => `<span class="font-medium">${v}</span>` },
		{ key: 'member_count', label: t('geo.members', 'Members'), sortable: true, render: (v: number) => `<span class="text-xs">${(v ?? 0).toLocaleString()}</span>` }
	];

	async function doLookup() {
		if (!ipQuery.trim()) return;
		lookupLoading = true;
		lookupError = '';
		lookupResult = null;
		try {
			const result = await geoApi.lookup(ipQuery.trim());
			if (result.ok) {
				lookupResult = result.data as GeoContext;
			} else {
				lookupError = result.error;
			}
		} catch {
			lookupError = t('geo.lookup_error', 'Lookup failed');
		} finally {
			lookupLoading = false;
		}
	}

	async function loadCountries() {
		hierarchyLoading = true;
		hierarchyError = '';
		try {
			const result = await geoApi.listCountries();
			if (result.ok) {
				countries = (result.data as GeoSpace[]) ?? [];
			} else {
				hierarchyError = result.error;
			}
		} catch {
			hierarchyError = t('common.error', 'Failed to load countries');
		} finally {
			hierarchyLoading = false;
		}
	}

	async function selectCountry(c: GeoSpace) {
		selectedCountry = c;
		selectedState = null;
		cities = [];
		stateSearch = '';
		citySearch = '';
		hierarchyLoading = true;
		hierarchyError = '';
		try {
			const result = await geoApi.listStates(c.geo_code ?? c.slug);
			if (result.ok) {
				states = (result.data as GeoSpace[]) ?? [];
			} else {
				hierarchyError = result.error;
			}
		} catch {
			hierarchyError = t('common.error', 'Failed to load states');
		} finally {
			hierarchyLoading = false;
		}
	}

	async function selectState(s: GeoSpace) {
		selectedState = s;
		citySearch = '';
		hierarchyLoading = true;
		hierarchyError = '';
		try {
			const result = await geoApi.listCities(s.id);
			if (result.ok) {
				cities = (result.data as GeoSpace[]) ?? [];
			} else {
				hierarchyError = result.error;
			}
		} catch {
			hierarchyError = t('common.error', 'Failed to load cities');
		} finally {
			hierarchyLoading = false;
		}
	}

	$effect(() => {
		if (activeTab === 'hierarchy' && countries.length === 0) {
			loadCountries();
		}
	});
</script>

<svelte:head>
	<title>{t('geo.title', 'Geo Tools')} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t('geo.title', 'Geo Tools')}
	description={t('geo.description', 'IP geolocation lookup and geo space hierarchy browser')}
/>

<!-- Tabs -->
<div class="mb-6 flex gap-1 rounded-lg border p-1" style="border-color: var(--color-border)">
	<button
		class="flex items-center gap-2 rounded-md px-4 py-2 text-sm font-medium transition-colors"
		style={activeTab === 'lookup' ? 'background-color: var(--color-primary); color: white' : ''}
		onclick={() => { activeTab = 'lookup'; }}
	>
		<Search class="h-4 w-4" />
		{t('geo.ip_lookup', 'IP Lookup')}
	</button>
	<button
		class="flex items-center gap-2 rounded-md px-4 py-2 text-sm font-medium transition-colors"
		style={activeTab === 'hierarchy' ? 'background-color: var(--color-primary); color: white' : ''}
		onclick={() => { activeTab = 'hierarchy'; }}
	>
		<Globe class="h-4 w-4" />
		{t('geo.hierarchy', 'Geo Hierarchy')}
	</button>
</div>

{#if activeTab === 'lookup'}
	<div class="mx-auto max-w-2xl space-y-4">
		{#if !canLookup}
			<div class="rounded-lg border p-6 text-center" style="border-color: var(--color-border); background-color: var(--color-surface)">
				<p class="text-sm" style="color: var(--color-muted)">{t('geo.no_permission', 'You need the geo.lookup permission to use this tool.')}</p>
			</div>
		{:else}
			<div class="rounded-lg border p-6" style="border-color: var(--color-border); background-color: var(--color-surface)">
				<label for="geo-ip" class="mb-2 block text-sm font-medium">{t('geo.ip_address', 'IP Address')}</label>
				<div class="flex gap-2">
					<input
						id="geo-ip"
						type="text"
						class="flex-1 rounded-md border px-3 py-2 text-sm font-mono"
						style="border-color: var(--color-border); background-color: var(--color-background)"
						placeholder="8.8.8.8"
						bind:value={ipQuery}
						onkeydown={(e) => { if (e.key === 'Enter') doLookup(); }}
					/>
					<Button onclick={doLookup} disabled={lookupLoading || !ipQuery.trim()}>
						{#if lookupLoading}
							<Loader2 class="h-3.5 w-3.5 animate-spin" />
						{:else}
							<Search class="h-3.5 w-3.5" />
						{/if}
						{t('geo.lookup_btn', 'Lookup')}
					</Button>
				</div>
				<p class="mt-2 text-xs" style="color: var(--color-muted)">{t('geo.lookup_helper', 'Resolve any IP address to its geographic location using the MaxMind GeoLite2 database.')}</p>
			</div>

			{#if lookupError}
				<div class="rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
					{lookupError}
				</div>
			{/if}

			{#if lookupResult}
				<div class="rounded-lg border p-6" style="border-color: var(--color-border); background-color: var(--color-surface)">
					<h3 class="mb-4 flex items-center gap-2 text-sm font-semibold">
						<MapPin class="h-4 w-4" style="color: var(--color-primary)" />
						{t('geo.result', 'Lookup Result')}
					</h3>
					<div class="grid grid-cols-2 gap-4">
						<div>
							<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('geo.country', 'Country')}</p>
							<p class="mt-1 text-sm font-medium">{lookupResult.country_name} ({lookupResult.country_code})</p>
						</div>
						<div>
							<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('geo.subdivision', 'State/Province')}</p>
							<p class="mt-1 text-sm">{lookupResult.subdivision || '—'}</p>
						</div>
						<div>
							<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('geo.city', 'City')}</p>
							<p class="mt-1 text-sm">{lookupResult.city || '—'}</p>
						</div>
						<div>
							<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('geo.timezone', 'Time Zone')}</p>
							<p class="mt-1 text-sm font-mono">{lookupResult.time_zone || '—'}</p>
						</div>
						<div>
							<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('geo.coordinates', 'Coordinates')}</p>
							<p class="mt-1 text-sm font-mono">{lookupResult.latitude.toFixed(4)}, {lookupResult.longitude.toFixed(4)}</p>
						</div>
					</div>
				</div>
			{/if}
		{/if}
	</div>
{:else}
	<div class="space-y-4">
		{#if hierarchyError}
			<div class="rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
				{hierarchyError}
			</div>
		{/if}

		<!-- Breadcrumb + create actions -->
		<div class="flex items-center justify-between gap-2">
			<div class="flex items-center gap-1 text-sm" style="color: var(--color-muted)">
				<button class="hover:text-foreground" onclick={() => { selectedCountry = null; selectedState = null; cities = []; stateSearch = ''; citySearch = ''; }}>
					{t('geo.countries', 'Countries')}
				</button>
				{#if selectedCountry}
					<ChevronRight class="h-3 w-3" />
					<button class="hover:text-foreground" onclick={() => { selectedState = null; cities = []; citySearch = ''; }}>
						{selectedCountry.name}
					</button>
				{/if}
				{#if selectedState}
					<ChevronRight class="h-3 w-3" />
					<span style="color: var(--color-foreground)">{selectedState.name}</span>
				{/if}
			</div>
			{#if canManageSpaces}
				{#if !selectedCountry}
					<Button onclick={() => goto(createGeoUrl('country'))} size="sm">
						<Plus class="mr-1 h-3.5 w-3.5" />
						{t('geo.create_country', 'Create Country')}
					</Button>
				{:else if !selectedState}
					<Button onclick={() => goto(createGeoUrl('state', selectedCountry?.id))} size="sm">
						<Plus class="mr-1 h-3.5 w-3.5" />
						{t('geo.create_state', 'Create State')}
					</Button>
				{:else}
					<Button onclick={() => goto(createGeoUrl('city', selectedState?.id))} size="sm">
						<Plus class="mr-1 h-3.5 w-3.5" />
						{t('geo.create_city', 'Create City')}
					</Button>
				{/if}
			{/if}
		</div>

		{#if hierarchyLoading}
			<TableSkeleton rows={5} />
		{:else if !selectedCountry}
			<DataTable
				columns={countryColumns}
				data={filteredCountries}
				actions={countryRowActions}
				searchable={true}
				searchPlaceholder={t('geo.search_countries', 'Search countries...')}
				onsearch={(q: string) => { countrySearch = q; }}
				pageSize={25}
				emptyMessage={t('geo.no_countries', 'No countries found')}
			/>
		{:else if !selectedState}
			<DataTable
				columns={stateColumns}
				data={filteredStates}
				actions={stateRowActions}
				searchable={true}
				searchPlaceholder={t('geo.search_states', 'Search states...')}
				onsearch={(q: string) => { stateSearch = q; }}
				pageSize={25}
				emptyMessage={t('geo.no_states', 'No states found for this country')}
			/>
		{:else}
			<DataTable
				columns={cityColumns}
				data={filteredCities}
				searchable={true}
				searchPlaceholder={t('geo.search_cities', 'Search cities...')}
				onsearch={(q: string) => { citySearch = q; }}
				pageSize={25}
				emptyMessage={t('geo.no_cities', 'No cities found for this state')}
			/>
		{/if}
	</div>
{/if}
