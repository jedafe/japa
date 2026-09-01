<script lang="ts">
	import { api } from '$lib/api/client';
	import { searchEntities } from '$lib/api/jce';
	import { t } from '$lib/i18n';
	import type { ResolvedItem } from '$lib/types/jce';

	type ScopeOption = {
		id: string;
		title: string;
		level?: string;
		geo_code?: string;
	};

	let { chain, selections = $bindable({}), onChange } = $props<{
		chain: string[];
		selections: Record<string, string>;
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
</script>

<div class="flex flex-wrap gap-3">
	{#each chain as step, index (step)}
		{@const parentKey = chain[index - 1]}
		{@const disabled = index > 0 && !selections[parentKey]}
		<select
			class="rounded-[var(--radius-button)] border bg-[var(--color-surface)] px-3 py-2 text-sm"
			style="border-color: var(--color-border); color: var(--color-text)"
			{disabled}
			value={selections[step] ?? ''}
			onchange={(e) => onChange(step, e.currentTarget.value)}
		>
			<option value="">{labelFor(step)}</option>
			{#each options[step] ?? [] as opt (opt.id)}
				<option value={opt.id}>{opt.title}</option>
			{/each}
		</select>
	{/each}
</div>
