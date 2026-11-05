<script lang="ts">
	/**
	 * /tools hero scene — a floating grid of tool tiles (calculator,
	 * checklist, compass, currency, route, gauge) with orbit accents.
	 */
	import {
		Banknote,
		Calculator,
		ClipboardCheck,
		Compass,
		Gauge,
		Route
	} from 'lucide-svelte';

	let { accent = '#0d9488' }: { accent?: string } = $props();

	const tiles = [
		{ x: 56, y: 44, icon: 'checklist', rot: -6 },
		{ x: 158, y: 30, icon: 'calc', rot: 4 },
		{ x: 262, y: 48, icon: 'gauge', rot: -4 },
		{ x: 74, y: 132, icon: 'currency', rot: 5 },
		{ x: 180, y: 122, icon: 'compass', rot: -3 },
		{ x: 284, y: 140, icon: 'route', rot: 6 }
	];
	const SIZE = 62;
</script>

<svg viewBox="0 0 380 224" class="h-full w-full" role="presentation">
	<circle cx="190" cy="112" r="104" fill={accent} opacity="0.05" />
	<ellipse cx="200" cy="112" rx="160" ry="82" fill="none" stroke={accent} stroke-width="1" opacity="0.22" stroke-dasharray="2 6" transform="rotate(-8 200 112)" />

	{#each tiles as tile}
		<g transform="rotate({tile.rot} {tile.x + SIZE / 2} {tile.y + SIZE / 2})">
			<rect x={tile.x} y={tile.y} width={SIZE} height={SIZE} rx="14" fill="var(--color-surface)" stroke="var(--color-border)" />
			<rect x={tile.x} y={tile.y} width={SIZE} height={SIZE} rx="14" fill={accent} opacity="0.07" />
			{#if tile.icon === 'checklist'}
				<ClipboardCheck x={tile.x + 18} y={tile.y + 18} size={26} color={accent} />
			{:else if tile.icon === 'calc'}
				<Calculator x={tile.x + 18} y={tile.y + 18} size={26} color={accent} />
			{:else if tile.icon === 'gauge'}
				<Gauge x={tile.x + 18} y={tile.y + 18} size={26} color={accent} />
			{:else if tile.icon === 'currency'}
				<Banknote x={tile.x + 18} y={tile.y + 18} size={26} color={accent} />
			{:else if tile.icon === 'compass'}
				<Compass x={tile.x + 18} y={tile.y + 18} size={26} color={accent} />
			{:else}
				<Route x={tile.x + 18} y={tile.y + 18} size={26} color={accent} />
			{/if}
		</g>
	{/each}

	<!-- sparkles -->
	<circle cx="140" cy="60" r="3" fill={accent} opacity="0.5" />
	<circle cx="240" cy="112" r="3.5" fill={accent} opacity="0.4" />
	<circle cx="330" cy="96" r="3" fill={accent} opacity="0.5" />
	<circle cx="52" cy="172" r="4" fill={accent} opacity="0.35" />
	<circle cx="352" cy="176" r="5" fill="none" stroke={accent} stroke-width="1.3" opacity="0.5" />
	<circle cx="120" cy="190" r="5" fill="none" stroke={accent} stroke-width="1.3" opacity="0.5" />
</svg>
