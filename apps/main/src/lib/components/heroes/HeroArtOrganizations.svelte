<script lang="ts">
	/**
	 * /org hero scene — a skyline of organisation towers linked by dashed
	 * routes to a verified HQ card.
	 */
	import { BadgeCheck, Building2 } from 'lucide-svelte';

	let { accent = '#0284c7' }: { accent?: string } = $props();

	const buildings = [
		{ x: 30, w: 34, h: 78, rows: 5 },
		{ x: 74, w: 40, h: 108, rows: 7 },
		{ x: 250, w: 36, h: 92, rows: 6 },
		{ x: 296, w: 44, h: 70, rows: 4 },
		{ x: 348, w: 28, h: 96, rows: 6 }
	];
	const BASE = 200;
</script>

<svg viewBox="0 0 380 224" class="h-full w-full" role="presentation">
	<circle cx="320" cy="36" r="80" fill={accent} opacity="0.06" />
	<circle cx="40" cy="60" r="26" fill={accent} opacity="0.08" />

	<!-- skyline -->
	{#each buildings as b}
		<rect x={b.x} y={BASE - b.h} width={b.w} height={b.h} rx="4" fill={accent} opacity="0.12" />
		{#each Array(b.rows) as _, r}
			{#each Array(Math.max(2, Math.floor(b.w / 14))) as _, c}
				<rect
					x={b.x + 7 + c * 14}
					y={BASE - b.h + 10 + r * 13}
					width="6"
					height="6"
					rx="1"
					fill={accent}
					opacity="0.35"
				/>
			{/each}
		{/each}
	{/each}
	<line x1="16" y1={BASE} x2="372" y2={BASE} stroke={accent} stroke-width="1.5" opacity="0.3" />

	<!-- network links to HQ -->
	<path d="M 96 92 Q 140 60 176 84" fill="none" stroke={accent} stroke-width="1.4" stroke-dasharray="4 4" opacity="0.6" />
	<path d="M 268 108 Q 238 70 208 84" fill="none" stroke={accent} stroke-width="1.4" stroke-dasharray="4 4" opacity="0.6" />
	<path d="M 318 130 Q 260 150 216 116" fill="none" stroke={accent} stroke-width="1.4" stroke-dasharray="4 4" opacity="0.5" />
	<circle cx="96" cy="92" r="3.5" fill={accent} />
	<circle cx="268" cy="108" r="3.5" fill={accent} />
	<circle cx="318" cy="130" r="3.5" fill={accent} />

	<!-- HQ card -->
	<rect x="156" y="70" width="72" height="56" rx="12" fill="var(--color-surface)" stroke="var(--color-border)" />
	<rect x="156" y="70" width="72" height="20" rx="10" fill={accent} opacity="0.12" />
	<Building2 x={184} y={76} size={16} color={accent} />
	<rect x="170" y="98" width="44" height="6" rx="3" fill="var(--color-muted)" opacity="0.45" />
	<rect x="176" y="110" width="32" height="5" rx="2.5" fill="var(--color-muted)" opacity="0.3" />
	<BadgeCheck x={214} y={66} size={18} color={accent} />
</svg>
