<script lang="ts">
	/**
	 * /radar hero scene — a radar sweep with opportunity blips and a
	 * tracked-signal card.
	 */
	import { Radar, TrendingUp } from 'lucide-svelte';

	let { accent = '#059669' }: { accent?: string } = $props();

	const blips = [
		{ x: 132, y: 74, r: 4 },
		{ x: 168, y: 108, r: 3 },
		{ x: 92, y: 122, r: 3.5 },
		{ x: 150, y: 152, r: 2.5 },
		{ x: 74, y: 84, r: 2.5 }
	];
</script>

<svg viewBox="0 0 380 224" class="h-full w-full" role="presentation">
	<circle cx="330" cy="170" r="80" fill={accent} opacity="0.06" />

	<!-- radar dish -->
	<g transform="translate(120 112)">
		<circle r="92" fill="none" stroke={accent} stroke-width="1.3" opacity="0.35" />
		<circle r="64" fill="none" stroke={accent} stroke-width="1.2" opacity="0.4" />
		<circle r="36" fill="none" stroke={accent} stroke-width="1.1" opacity="0.45" />
		<line x1="-92" y1="0" x2="92" y2="0" stroke={accent} stroke-width="1" opacity="0.25" />
		<line x1="0" y1="-92" x2="0" y2="92" stroke={accent} stroke-width="1" opacity="0.25" />
		<!-- sweep wedge -->
		<path d="M 0 0 L 88 -28 A 92 92 0 0 0 65 -65 Z" fill={accent} opacity="0.18" />
		<line x1="0" y1="0" x2="88" y2="-28" stroke={accent} stroke-width="1.8" opacity="0.8" />
		<circle r="5" fill={accent} />
	</g>

	<!-- blips -->
	{#each blips as b}
		<circle cx={b.x} cy={b.y} r={b.r} fill={accent} />
		<circle cx={b.x} cy={b.y} r={b.r + 4} fill="none" stroke={accent} stroke-width="1" opacity="0.4" />
	{/each}

	<!-- tracked signal card -->
	<g>
		<rect x="238" y="52" width="116" height="66" rx="12" fill="var(--color-surface)" stroke="var(--color-border)" />
		<circle cx="260" cy="76" r="11" fill={accent} opacity="0.16" />
		<Radar x={253} y={69} size={14} color={accent} />
		<rect x="278" y="70" width="62" height="7" rx="3.5" fill="var(--color-muted)" opacity="0.5" />
		<rect x="278" y="83" width="44" height="6" rx="3" fill="var(--color-muted)" opacity="0.3" />
		<rect x="250" y="98" width="88" height="5" rx="2.5" fill="var(--color-muted)" opacity="0.25" />
	</g>
	<g>
		<rect x="252" y="140" width="96" height="50" rx="12" fill="var(--color-surface)" stroke="var(--color-border)" transform="rotate(4 300 165)" />
		<TrendingUp x={264} y={152} size={16} color={accent} />
		<rect x="286" y="156" width="48" height="6" rx="3" fill="var(--color-muted)" opacity="0.45" transform="rotate(4 310 159)" />
		<rect x="264" y="172" width="66" height="5" rx="2.5" fill="var(--color-muted)" opacity="0.3" transform="rotate(4 297 174)" />
	</g>

	<!-- link from scope to card -->
	<path d="M 196 84 Q 216 70 236 80" fill="none" stroke={accent} stroke-width="1.4" stroke-dasharray="4 4" opacity="0.55" />
</svg>
