<script lang="ts">
	import { Chart, Svg, Bars, Axis } from 'layerchart';
	import { formatMetric } from './format';

	interface BarPoint {
		x: string; // category label
		y: number;
	}
	interface Props {
		data: BarPoint[];
		label?: string;
		color?: string;
	}
	let { data, label, color = 'var(--chart-3)' }: Props = $props();
</script>

<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
	{#if label}
		<p class="mb-2 text-xs text-[var(--color-muted)]">{label}</p>
	{/if}
	<div class="h-48">
		<Chart {data} x="x" y="y">
			<Svg>
				<Axis placement="left" ticks={4} format={(v: number) => formatMetric(v)} />
				<Bars fill={color} />
			</Svg>
		</Chart>
	</div>
</div>
