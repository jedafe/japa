<script lang="ts">
	// GeoSpaceCascade — cascading country → state → city picker backed by the
	// public /geo/* endpoints (same source as the member location picker,
	// ADR-064a). `value` binds the deepest selected space UUID; '' = unset.
	import { geoApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

	type GeoSpace = { id: string; name: string; slug: string; level: string; geo_code?: string | null };

	let {
		value = $bindable<string>(''),
		id = 'geo-cascade'
	}: {
		value?: string;
		id?: string;
	} = $props();

	let countries = $state<GeoSpace[]>([]);
	let states = $state<GeoSpace[]>([]);
	let cities = $state<GeoSpace[]>([]);
	let countryId = $state('');
	let stateId = $state('');
	let cityId = $state('');
	let loading = $state(false);

	$effect(() => {
		(async () => {
			loading = true;
			try {
				const res = await geoApi.listCountries();
				if (res.ok) {
					const data = res.data as GeoSpace[] | { data?: GeoSpace[] };
					countries = Array.isArray(data) ? data : (data.data ?? []);
				}
			} finally {
				loading = false;
			}
		})();
	});

	// listStates keys off the country's ISO geo_code, not its id.
	function geoCodeOf(idVal: string): string {
		return countries.find((c) => c.id === idVal)?.geo_code ?? '';
	}

	async function pickCountry(idVal: string) {
		countryId = idVal;
		stateId = '';
		cityId = '';
		cities = [];
		value = idVal;
		if (!idVal) {
			states = [];
			return;
		}
		const res = await geoApi.listStates(geoCodeOf(idVal));
		if (res.ok) {
			const data = res.data as GeoSpace[] | { data?: GeoSpace[] };
			states = Array.isArray(data) ? data : (data.data ?? []);
		}
	}

	async function pickState(idVal: string) {
		stateId = idVal;
		cityId = '';
		value = idVal || countryId;
		if (!idVal) {
			cities = [];
			return;
		}
		const res = await geoApi.listCities(idVal);
		if (res.ok) {
			const data = res.data as GeoSpace[] | { data?: GeoSpace[] };
			cities = Array.isArray(data) ? data : (data.data ?? []);
		}
	}

	function pickCity(idVal: string) {
		cityId = idVal;
		value = idVal || stateId || countryId;
	}

	const selectCls = 'w-full rounded-md border px-3 py-1.5 text-sm';
	const selectStyle =
		'border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)';
</script>

<div class="grid gap-3 sm:grid-cols-3">
	<select
		id={`${id}-country`}
		class={selectCls}
		style={selectStyle}
		value={countryId}
		onchange={(e) => pickCountry((e.target as HTMLSelectElement).value)}
	>
		<option value="">{t('personas.geo_country', 'Country')}</option>
		{#each countries as c (c.id)}
			<option value={c.id}>{c.name}</option>
		{/each}
	</select>
	<select
		id={`${id}-state`}
		class={selectCls}
		style={selectStyle}
		value={stateId}
		disabled={states.length === 0}
		onchange={(e) => pickState((e.target as HTMLSelectElement).value)}
	>
		<option value="">{t('personas.geo_state', 'State / Region')}</option>
		{#each states as s (s.id)}
			<option value={s.id}>{s.name}</option>
		{/each}
	</select>
	<select
		id={`${id}-city`}
		class={selectCls}
		style={selectStyle}
		value={cityId}
		disabled={cities.length === 0}
		onchange={(e) => pickCity((e.target as HTMLSelectElement).value)}
	>
		<option value="">{t('personas.geo_city', 'City')}</option>
		{#each cities as c (c.id)}
			<option value={c.id}>{c.name}</option>
		{/each}
	</select>
</div>
{#if loading}
	<p class="mt-1 text-[10px]" style="color: var(--color-muted)">{t('common.loading', 'Loading...')}</p>
{/if}
