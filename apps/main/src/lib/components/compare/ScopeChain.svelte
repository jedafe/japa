<script lang="ts">
	import { api } from '$lib/api/client';
	import { searchEntities } from '$lib/api/jce';
	import { t } from '$lib/i18n';
	import { AutocompleteSelect } from '$lib/components/ui';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '@japaconnect/ui';
	import type { ResolvedItem } from '$lib/types/jce';

	type ScopeOption = {
		id: string;
		title: string;
		level?: string;
		geo_code?: string;
	};

	let {
		chain,
		selections = $bindable({}),
		labels = $bindable({}),
		onChange
	} = $props<{
		chain: string[];
		selections: Record<string, string>;
		labels?: Record<string, string>;
		onChange: (key: string, value: string) => void;
	}>();

	let options = $state<Record<string, ScopeOption[]>>({});

	// Region/subregion are small bounded string lists — static select.
	// Entity steps (country/state/city/university/…) go through the entity
	// resolver, whose universe can far exceed any page-size cap (universities
	// alone are ~10k rows), so they use a remote debounced search instead.
	function isStaticStep(step: string): boolean {
		return step === 'region' || step === 'subregion';
	}

	function labelFor(step: string): string {
		return t(`jce.compare.scope.select_${step}`);
	}

	function filtersFor(step: string): Record<string, string> {
		const filters: Record<string, string> = {};
		switch (step) {
			case 'country':
				if (selections.region) filters.region = selections.region;
				if (selections.subregion) filters.subregion = selections.subregion;
				break;
			case 'state':
				if (selections.country) filters.country_id = selections.country;
				break;
			case 'city':
				if (selections.state) filters.state_id = selections.state;
				break;
			case 'university':
				if (selections.city) filters.city_id = selections.city;
				else if (selections.state) filters.state_id = selections.state;
				else if (selections.country) filters.country_id = selections.country;
				break;
		}
		return filters;
	}

	function toAutocompleteItem(raw: ResolvedItem): AutocompleteItem {
		const sub = (raw.metadata?.geo_code as string) ?? (raw.metadata?.country as string) ?? '';
		return { id: raw.id, label: raw.title, ...(sub ? { sublabel: String(sub) } : {}) };
	}

	function stepFetcher(step: string) {
		return async (q: string) => {
			const items = await searchEntities(step, q, filtersFor(step), { pageSize: 20 });
			return items.map(toAutocompleteItem);
		};
	}

	// Resolve the display label for a URL-restored selection (the remote
	// search never sees the picked id otherwise). resolvedFor tracks which id
	// the label belongs to so a changed selection re-resolves.
	let resolvedFor = $state<Record<string, string>>({});
	async function resolveLabel(step: string, id: string) {
		if (resolvedFor[step] === id && labels[step]) return;
		resolvedFor[step] = id;
		try {
			const res = await api.get<ResolvedItem>(`/jce/entities/${step}/${encodeURIComponent(id)}`);
			if (res.data?.title && selections[step] === id) {
				labels = { ...labels, [step]: res.data.title };
			}
		} catch {
			// Unresolvable scope id — the summary simply omits it.
		}
	}

	function selectItem(step: string, item: AutocompleteItem) {
		resolvedFor[step] = item.id;
		labels = { ...labels, [step]: item.label };
		onChange(step, item.id);
	}

	function clearItem(step: string) {
		const next = { ...labels };
		delete next[step];
		labels = next;
		delete resolvedFor[step];
		onChange(step, '');
	}

	async function loadOptions(step: string) {
		const index = chain.indexOf(step);
		if (index < 0) return;

		if (step === 'region') {
			const res = await api.get<string[]>('/spaces/regions');
			options[step] = (res.data ?? []).map((r) => ({ id: r, title: r }));
			return;
		}

		if (step === 'subregion') {
			const region = selections.region;
			if (!region) {
				options[step] = [];
				return;
			}
			const res = await api.get<string[]>(`/spaces/subregions?region=${encodeURIComponent(region)}`);
			options[step] = (res.data ?? []).map((r) => ({ id: r, title: r }));
			return;
		}
	}

	$effect(() => {
		for (let i = 0; i < chain.length; i++) {
			const step = chain[i];
			const parentKey = chain[i - 1];
			if (!isStaticStep(step)) {
				if (selections[step]) resolveLabel(step, selections[step]);
				continue;
			}
			if (i === 0) {
				loadOptions(step);
				continue;
			}
			if (!selections[parentKey]) {
				options[step] = [];
				continue;
			}
			loadOptions(step);
		}
	});

	$effect(() => {
		// Surface id→title for static-step selections (region/subregion) so the
		// parent summary renders names, not ids. Entity steps write labels
		// imperatively on select/resolve.
		const next: Record<string, string> = { ...labels };
		for (const step of chain) {
			const id = selections[step];
			if (!id) {
				delete next[step];
				delete resolvedFor[step];
				continue;
			}
			if (!isStaticStep(step)) continue;
			const opt = options[step]?.find((o) => o.id === id);
			if (opt) next[step] = opt.title;
		}
		const serialized = JSON.stringify(next);
		if (serialized !== JSON.stringify(labels)) labels = next;
	});
</script>

<div class="flex flex-col gap-3 sm:flex-row">
	{#each chain as step, index (step)}
		{@const parentKey = chain[index - 1]}
		{@const disabled = index > 0 && !selections[parentKey]}
		{#if isStaticStep(step)}
			<AutocompleteSelect
				class="w-full min-w-0 sm:flex-1"
				value={selections[step] ?? ''}
				options={[
					{ value: '', label: labelFor(step) },
					...(options[step] ?? []).map((o) => ({ value: o.id, label: o.title }))
				]}
				onValueChange={(v) => onChange(step, v)}
				{disabled}
			/>
		{:else}
			<Autocomplete
				class="w-full min-w-0 sm:flex-1"
				placeholder={labelFor(step)}
				emptyMessage={t('common.no_results', 'No results found')}
				fetcher={stepFetcher(step)}
				initialFetcher={() => stepFetcher(step)('')}
				selectedId={selections[step] ?? null}
				selectedLabel={labels[step] ?? null}
				onSelect={(item) => selectItem(step, item)}
				onClear={() => clearItem(step)}
				{disabled}
			/>
		{/if}
	{/each}
</div>
