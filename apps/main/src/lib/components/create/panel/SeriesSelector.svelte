<script lang="ts">
	import { t } from '$lib/i18n';
	import type { BlogSeries } from '$lib/types';

	interface Props {
		series: BlogSeries[];
		seriesId: string | null;
		onselect: (id: string | null) => void;
		onfocus?: () => void;
	}

	let { series, seriesId, onselect, onfocus }: Props = $props();
</script>

<select
	class="series-select"
	value={seriesId ?? ''}
	onfocus={onfocus}
	onchange={(e) => onselect(e.currentTarget.value || null)}
>
	<option value="">{t('create.placeholder.series_none')}</option>
	{#each series as s (s.id)}
		<option value={s.id}>{s.title}</option>
	{/each}
</select>

<style>
	.series-select {
		width: 100%;
		padding: 10px 14px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		color: var(--color-text);
		font-size: 1rem;
		outline: none;
		cursor: pointer;
		transition: border-color 0.15s, box-shadow 0.15s;
	}
	.series-select:focus {
		border-color: var(--color-primary);
		box-shadow: 0 0 0 3px color-mix(in srgb, var(--color-primary) 15%, transparent);
	}
</style>
