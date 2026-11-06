<script lang="ts">
	/**
	 * /japa-stage hero scene — dotted world map with a flight arc and stage
	 * pins behind the team portrait cutout (static asset
	 * /illustrations/japa-stage-hero.webp).
	 */
	import { MapPin, Plane } from 'lucide-svelte';

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
		'...................#####....############....###...####..........',
		'..................#######...############....###...#####.........',
		'..................#######...############....###....#####........',
		'...................######....##########............######.......',
		'...................#####......#######...............########....',
		'...................####........#######...............########....',
		'...................###.........#######...............########....',
		'....................##..........##..............................',
		'.....................#...........#..............................'
	];

	const CELL = 4.4;
	const OX = 28;
	const OY = 8;

	const dots = MAP.flatMap((row, y) =>
		[...row].flatMap((c, x) => (c === '#' ? [{ x: OX + x * CELL, y: OY + y * CELL }] : []))
	);
</script>

<div class="relative h-full w-full" role="presentation">
	<svg viewBox="0 0 380 224" class="absolute inset-0 h-full w-full">
		<g fill={accent} opacity="0.18">
			{#each dots as d (d.x + '-' + d.y)}
				<circle cx={d.x} cy={d.y} r="1.6" />
			{/each}
		</g>
		<!-- Flight arc across the map. -->
		<path
			d="M 40 180 Q 160 40 320 60"
			fill="none"
			stroke={accent}
			stroke-width="2"
			stroke-dasharray="6 6"
			opacity="0.55"
		/>
		<circle cx="40" cy="180" r="4" fill={accent} opacity="0.7" />
		<circle cx="320" cy="60" r="4" fill={accent} opacity="0.7" />
	</svg>
	<Plane
		size={18}
		class="absolute text-[var(--hero-accent)] opacity-70"
		style="left: 44%; top: 14%; transform: rotate(18deg); color: {accent}"
	/>
	<MapPin
		size={16}
		class="absolute opacity-80"
		style="right: 6%; top: 20%; color: {accent}"
	/>
	<img
		src="/illustrations/japa-stage-hero.webp"
		alt=""
		class="absolute bottom-0 right-0 h-full w-auto object-contain object-bottom"
		loading="eager"
		decoding="async"
	/>
</div>
