<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { parseCardStyleValue, buildCardStyleValue } from '$lib/feed/cardStyleValue';

	interface Props {
		value: string;
		contentTypes: { value: string; label: string }[];
		onchange: (value: string) => void;
	}
	let { value, contentTypes, onchange }: Props = $props();

	const styleOptions = $derived([
		{ value: 'reddit', label: t('settings.card_style_reddit', 'Reddit') },
		{ value: 'compact', label: t('settings.card_style_compact', 'Compact') },
		{ value: 'grid', label: t('settings.card_style_grid', 'Grid') },
		{ value: 'magazine', label: t('settings.card_style_magazine', 'Magazine') }
	]);

	const parsed = $derived(parseCardStyleValue(value));

	const selectClass =
		'border-border-input bg-background text-foreground w-40 rounded-md border px-2 py-1 text-sm';

	function setDefault(v: string) {
		onchange(buildCardStyleValue(v, parsed.byType));
	}
	function setType(ct: string, v: string) {
		const next = { ...parsed.byType };
		if (v === '') delete next[ct];
		else next[ct] = v;
		onchange(buildCardStyleValue(parsed.default, next));
	}
</script>

<div class="space-y-3">
	<label class="flex items-center justify-between gap-4">
		<span class="text-sm font-medium">{t('settings.card_style_default', 'Default card style')}</span>
		<select
			class={selectClass}
			value={parsed.default}
			onchange={(e) => setDefault((e.currentTarget as HTMLSelectElement).value)}
		>
			{#each styleOptions as o (o.value)}<option value={o.value}>{o.label}</option>{/each}
		</select>
	</label>

	<p class="text-muted-foreground text-xs">
		{t(
			'settings.card_style_grid_note',
			'In the mixed home feed a “grid” type shows as a full-width card; a single-type listing shows a 2-column grid.'
		)}
	</p>

	{#each contentTypes as ct (ct.value)}
		<label class="flex items-center justify-between gap-4">
			<span class="text-sm">{ct.label}</span>
			<select
				class={selectClass}
				value={parsed.byType[ct.value] ?? ''}
				onchange={(e) => setType(ct.value, (e.currentTarget as HTMLSelectElement).value)}
			>
				<option value="">{t('settings.card_style_use_default', 'Use default')}</option>
				{#each styleOptions as o (o.value)}<option value={o.value}>{o.label}</option>{/each}
			</select>
		</label>
	{/each}
</div>
