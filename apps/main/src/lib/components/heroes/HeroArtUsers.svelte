<script lang="ts">
	/**
	 * /users hero scene — dot-grid world map with member avatar chips
	 * linked by dashed arcs around a central accent hub.
	 */
	import { Users } from 'lucide-svelte';

	let { accent = '#4f46e5' }: { accent?: string } = $props();

	// Stylised equirectangular landmass grid (decorative, not geographic data).
	const MAP = [
		'................................................................',
		'........................#####...................................',
		'...###########...........####.................#################.',
		'..###############.............#######.....#####################.',
		'...#############.............#################################..',
		'....###########..............##############################.....',
		'.....########.................#######.###################.......',
		'......######..................######...###############..........',
		'.......####..................##########.###########.....##......',
		'........##...................##########..#########..............',
		'...................#####....############....###...####..........',
		'..................#######...############....###...#####.........',
		'..................#######...############....###....#####........',
		'..................#######....##########............######.......',
		'...................######....##########.............########....',
		'...................######.....#######...............########....',
		'...................#####......#######...............########....',
		'....................###........####..................#####......',
		'....................##..........##..............................',
		'.....................#...........#..............................'
	];

	const CELL = 4.4;
	const OX = 48;
	const OY = 8;

	const dots = MAP.flatMap((row, y) =>
		[...row].flatMap((c, x) => (c === '#' ? [{ x: OX + x * CELL, y: OY + y * CELL }] : []))
	);

	const avatars = [
		{ x: 76, y: 62, initials: 'AO' },
		{ x: 196, y: 30, initials: 'JM' },
		{ x: 316, y: 56, initials: 'SK' },
		{ x: 262, y: 116, initials: 'TM' }
	];
</script>

<svg viewBox="0 0 380 224" class="h-full w-full" role="presentation">
	<circle cx="330" cy="40" r="90" fill={accent} opacity="0.06" />
	<circle cx="60" cy="200" r="70" fill={accent} opacity="0.05" />

	{#each dots as dot}
		<circle cx={dot.x} cy={dot.y} r="1.5" fill={accent} opacity="0.28" />
	{/each}

	<!-- connection arcs between members -->
	<path d="M 96 78 Q 140 40 196 46" fill="none" stroke={accent} stroke-width="1.4" stroke-dasharray="4 4" opacity="0.6" />
	<path d="M 216 46 Q 270 30 316 70" fill="none" stroke={accent} stroke-width="1.4" stroke-dasharray="4 4" opacity="0.6" />
	<path d="M 210 130 Q 240 150 268 132" fill="none" stroke={accent} stroke-width="1.4" stroke-dasharray="4 4" opacity="0.5" />
	<path d="M 190 96 Q 160 140 196 158" fill="none" stroke={accent} stroke-width="1.4" stroke-dasharray="4 4" opacity="0.5" />

	<!-- floating member chips -->
	{#each avatars as a}
		<g>
			<rect x={a.x - 18} y={a.y - 18} width="36" height="36" rx="10" fill="var(--color-surface)" stroke="var(--color-border)" />
			<circle cx={a.x} cy={a.y} r="11" fill={accent} opacity="0.15" />
			<text x={a.x} y={a.y + 3.5} text-anchor="middle" font-size="9" font-weight="700" fill={accent}>{a.initials}</text>
			<circle cx={a.x + 13} cy={a.y - 13} r="4" fill={accent} />
		</g>
	{/each}

	<!-- central hub -->
	<circle cx="196" cy="112" r="24" fill={accent} />
	<circle cx="196" cy="112" r="30" fill="none" stroke={accent} stroke-width="1.2" opacity="0.35" stroke-dasharray="3 5" />
	<Users x={184} y={100} size={24} color="var(--hero-contrast)" />
</svg>
