<script lang="ts">
	import { t } from '$lib/i18n';
	import type { BlogSeries } from '$lib/types';

	interface Props {
		series: BlogSeries[];
		seriesId: string | null;
		onselect: (id: string | null) => void;
	}

	let { series, seriesId, onselect }: Props = $props();
</script>

<div class="flex flex-col gap-1">
	<label class="text-sm font-medium text-[var(--color-text)]">{t('create.label.series')}</label>
	<select
		class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm outline-none focus:border-[var(--color-primary)]"
		value={seriesId ?? ''}
		onchange={(e) => onselect(e.currentTarget.value || null)}
	>
		<option value="">{t('create.placeholder.space')}</option>
		{#each series as s (s.id)}
			<option value={s.id}>{s.title}</option>
		{/each}
	</select>
</div>
