<script lang="ts">
	// ProgressRing — circular progress indicator using SVG.
	interface Props {
		/** 0–100. */
		value: number;
		size?: number;
		strokeWidth?: number;
		/** Optional center label. */
		label?: string;
	}
	let { value, size = 64, strokeWidth = 6, label }: Props = $props();

	const radius = $derived((size - strokeWidth) / 2);
	const circumference = $derived(2 * Math.PI * radius);
	const clamped = $derived(Math.max(0, Math.min(100, value)));
	const dashOffset = $derived(circumference - (clamped / 100) * circumference);
</script>

<div class="relative inline-flex items-center justify-center" style="width: {size}px; height: {size}px">
	<svg width={size} height={size} class="-rotate-90">
		<circle
			cx={size / 2}
			cy={size / 2}
			{radius}
			fill="none"
			stroke="var(--color-surface-subtle)"
			stroke-width={strokeWidth}
		/>
		<circle
			cx={size / 2}
			cy={size / 2}
			{radius}
			fill="none"
			stroke="var(--color-primary)"
			stroke-width={strokeWidth}
			stroke-linecap="round"
			stroke-dasharray={circumference}
			stroke-dashoffset={dashOffset}
			style="transition: stroke-dashoffset 0.3s ease"
		/>
	</svg>
	<span class="absolute text-xs font-semibold text-[var(--color-text)]">
		{label ?? `${Math.round(clamped)}%`}
	</span>
</div>
