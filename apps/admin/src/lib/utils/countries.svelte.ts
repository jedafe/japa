import { geoApi, type GeoCountry } from '$lib/api';
import type { AutocompleteItem } from '$lib/components/ui/Autocomplete.svelte';

export type CountryValueType = 'id' | 'name' | 'geo_code';

let countries = $state<GeoCountry[]>([]);
let loading = $state(false);

/**
 * Shared country selector state and fetchers. Loads the full 250 primary geo
 * space countries once from GET /api/v1/geo/countries and filters them
 * client-side for live debounced Autocomplete usage.
 */
export function useCountries(valueType: CountryValueType = 'id') {
	async function load() {
		if (loading || countries.length) return;
		loading = true;
		try {
			const res = await geoApi.listCountries();
			if (res.ok && Array.isArray(res.data)) {
				countries = (res.data as GeoCountry[]).filter((c) => c.level === 'country');
			}
		} finally {
			loading = false;
		}
	}

	function toItem(c: GeoCountry): AutocompleteItem {
		const id = valueType === 'name' ? c.name : valueType === 'geo_code' ? c.geo_code : c.id;
		return { id, label: c.name, sublabel: c.geo_code };
	}

	async function fetcher(q: string, limit = 50): Promise<AutocompleteItem[]> {
		await load();
		const needle = q.toLowerCase().trim();
		return countries
			.filter((c) => {
				if (!needle) return true;
				return (
					c.name.toLowerCase().includes(needle) ||
					c.geo_code.toLowerCase().includes(needle) ||
					c.slug.toLowerCase().includes(needle)
				);
			})
			.slice(0, limit)
			.map(toItem);
	}

	async function initialFetcher(): Promise<AutocompleteItem[]> {
		return fetcher('', 20);
	}

	function getSelectedLabel(id: string): string {
		const c = countries.find((c) => {
			if (valueType === 'name') return c.name === id;
			if (valueType === 'geo_code') return c.geo_code === id;
			return c.id === id;
		});
		return c?.name ?? id;
	}

	const options = $derived(
		countries.map((c) => ({
			value: valueType === 'name' ? c.name : valueType === 'geo_code' ? c.geo_code : c.id,
			label: c.name
		}))
	);

	const ready = $derived(countries.length > 0);

	return {
		get countries() {
			return countries;
		},
		get loading() {
			return loading;
		},
		get ready() {
			return ready;
		},
		get options() {
			return options;
		},
		load,
		fetcher,
		initialFetcher,
		getSelectedLabel
	};
}
