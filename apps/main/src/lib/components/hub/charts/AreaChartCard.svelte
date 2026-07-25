<script lang="ts">
	import { Chart, Svg, Area, Axis } from 'layerchart';
	import { formatMetric } from './format';

	/** Time-series for an area chart (e.g. impressions over time). */
	interface SeriesPoint {
		x: number; // timestamp or index
		y: number;
	}
	interface Props {
		data: SeriesPoint[];
		label?: string;
		color?: string;
	}
	let { data, label, color = 'var(--chart-1)' }: Props = $props();
</script>

<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
	{#if label}
		<p class="mb-2 text-xs text-[var(--color-muted)]">{label}</p>
	{/if}
	<div class="h-48">
		<Chart {data} x="x" y="y">
			<Svg>
				<Axis placement="left" ticks={4} format={(v: number) => formatMetric(v)} />
				<Area fill={color} fillOpacity={0.15} stroke={color} strokeWidth={2} />
			</Svg>
		</Chart>
	</div>
</div>
