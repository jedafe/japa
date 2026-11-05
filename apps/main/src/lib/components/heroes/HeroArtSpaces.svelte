<script lang="ts">
	/**
	 * /spaces hero scene — a constellation of community nodes (country,
	 * city, topic) linked around a central space hub.
	 */
	import { Globe, Hash, Layers, MapPin, MessageSquare } from 'lucide-svelte';

	let { accent = '#7c3aed' }: { accent?: string } = $props();

	const nodes = [
		{ x: 84, y: 56, r: 18 },
		{ x: 56, y: 138, r: 14 },
		{ x: 158, y: 184, r: 15 },
		{ x: 296, y: 170, r: 16 },
		{ x: 326, y: 70, r: 14 },
		{ x: 246, y: 30, r: 12 }
	];
</script>

<svg viewBox="0 0 380 224" class="h-full w-full" role="presentation">
	<circle cx="200" cy="112" r="110" fill={accent} opacity="0.05" />
	<circle cx="200" cy="112" r="72" fill="none" stroke={accent} stroke-width="1" opacity="0.25" stroke-dasharray="2 6" />

	<!-- constellation links -->
	<path d="M 100 62 Q 140 84 176 100" fill="none" stroke={accent} stroke-width="1.4" stroke-dasharray="4 4" opacity="0.55" />
	<path d="M 70 128 Q 120 118 176 112" fill="none" stroke={accent} stroke-width="1.4" stroke-dasharray="4 4" opacity="0.55" />
	<path d="M 170 172 Q 184 146 192 128" fill="none" stroke={accent} stroke-width="1.4" stroke-dasharray="4 4" opacity="0.55" />
	<path d="M 224 126 Q 260 150 282 162" fill="none" stroke={accent} stroke-width="1.4" stroke-dasharray="4 4" opacity="0.55" />
	<path d="M 224 96 Q 276 76 312 72" fill="none" stroke={accent} stroke-width="1.4" stroke-dasharray="4 4" opacity="0.55" />
	<path d="M 212 96 Q 228 62 240 42" fill="none" stroke={accent} stroke-width="1.4" stroke-dasharray="4 4" opacity="0.55" />

	<!-- member nodes -->
	{#each nodes as n}
		<circle cx={n.x} cy={n.y} r={n.r} fill="var(--color-surface)" stroke={accent} stroke-width="1.4" opacity="1" />
	{/each}
	<Globe x={nodes[0].x - 8} y={nodes[0].y - 8} size={16} color={accent} />
	<MapPin x={nodes[1].x - 6} y={nodes[1].y - 6} size={12} color={accent} />
	<Hash x={nodes[2].x - 6} y={nodes[2].y - 6} size={12} color={accent} />
	<MessageSquare x={nodes[3].x - 7} y={nodes[3].y - 7} size={14} color={accent} />
	<Globe x={nodes[4].x - 6} y={nodes[4].y - 6} size={12} color={accent} />
	<Hash x={nodes[5].x - 5} y={nodes[5].y - 5} size={10} color={accent} />

	<!-- central hub -->
	<circle cx="200" cy="110" r="24" fill={accent} />
	<circle cx="200" cy="110" r="30" fill="none" stroke={accent} stroke-width="1.2" opacity="0.35" stroke-dasharray="3 5" />
	<Layers x={189} y={99} size={22} color="var(--hero-contrast)" />

	<!-- ambient dots -->
	<circle cx="120" cy="190" r="3" fill={accent} opacity="0.4" />
	<circle cx="340" cy="120" r="3" fill={accent} opacity="0.4" />
	<circle cx="280" cy="200" r="4" fill={accent} opacity="0.3" />
	<circle cx="40" cy="80" r="4" fill={accent} opacity="0.3" />
</svg>
