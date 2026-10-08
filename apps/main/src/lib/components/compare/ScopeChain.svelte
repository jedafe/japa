<script lang="ts">
	import { api } from '$lib/api/client';
	import { searchEntities } from '$lib/api/jce';
	import { t } from '$lib/i18n';
	import { AutocompleteSelect } from '$lib/components/ui';
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

	function toOption(raw: ResolvedItem): ScopeOption {
		return {
			id: raw.id,
			title: raw.title,
			level: (raw.metadata?.level as string) ?? undefined,
			geo_code: (raw.metadata?.geo_code as string) ?? undefined
		};
	}

	function labelFor(step: string): string {
		return t(`jce.compare.scope.select_${step}`);
	}

	function selectedOption(step: string): ScopeOption | undefined {
		const id = selections[step];
		if (!id) return undefined;
		return options[step]?.find((o) => o.id === id);
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

	async function loadOptions(step: string) {
		const index = chain.indexOf(step);
		if (index < 0) return;
		const parentKey = chain[index - 1];

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

		if (parentKey && !selections[parentKey]) {
			options[step] = [];
			return;
		}

		const filters = filtersFor(step);
		const items = await searchEntities(step, '', filters, { pageSize: 1000 });
		options[step] = items.map(toOption);
	}

	$effect(() => {
		for (let i = 0; i < chain.length; i++) {
			const step = chain[i];
			const parentKey = chain[i - 1];
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

	let prevLabels = '';

	$effect(() => {
		// Surface id→title for selected steps so parents can render a summary
		// (e.g. "Nigeria · Lagos"). Unresolved ids simply omit until options load.
		const next: Record<string, string> = {};
		for (const step of chain) {
			const id = selections[step];
			if (!id) continue;
			const opt = options[step]?.find((o) => o.id === id);
			if (opt) next[step] = opt.title;
		}
		const serialized = JSON.stringify(next);
		if (serialized !== prevLabels) {
			prevLabels = serialized;
			labels = next;
		}
	});
</script>

<div class="flex flex-col gap-3 sm:flex-row">
	{#each chain as step, index (step)}
		{@const parentKey = chain[index - 1]}
		{@const disabled = index > 0 && !selections[parentKey]}
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
	{/each}
</div>
