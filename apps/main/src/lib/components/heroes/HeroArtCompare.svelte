<script lang="ts">
	/**
	 * /compare + /comparisons hero scene — a fanned stack of entity cards
	 * (header band + two dot-metric rows each) over the orbit + "VS" node.
	 */
	let { accent = '#3b49df' }: { accent?: string } = $props();

	// Three stacked cards — depth order back→front.
	const cards = [
		{ x: 58, y: 62, rot: -7, flag: '#e5453c' },
		{ x: 140, y: 48, rot: 0, flag: '#3b49df' },
		{ x: 226, y: 62, rot: 7, flag: '#e8a13a' }
	];
</script>

<svg viewBox="0 0 380 224" class="h-full w-full" role="presentation">
	<circle cx="190" cy="112" r="104" fill={accent} opacity="0.05" />
	<ellipse
		cx="190"
		cy="112"
		rx="160"
		ry="82"
		fill="none"
		stroke={accent}
		stroke-width="1"
		opacity="0.22"
		stroke-dasharray="2 6"
		transform="rotate(-6 190 112)"
	/>

	{#each cards as card, i (i)}
		<g transform="rotate({card.rot} {card.x + 52} {card.y + 60})">
			<rect x={card.x} y={card.y} width="104" height="122" rx="12"
				fill="var(--color-surface)" stroke="var(--color-border)" />
			<!-- header band (skyline stand-in) -->
			<rect x={card.x} y={card.y} width="104" height="42" rx="12" fill={accent} opacity="0.16" />
			<rect x={card.x} y={card.y + 30} width="104" height="12" fill={accent} opacity="0.16" />
			<!-- flag chip -->
			<circle cx={card.x + 16} cy={card.y + 14} r="8" fill={card.flag} opacity="0.85" />
			<rect x={card.x + 28} y={card.y + 11} width="40" height="6" rx="3" fill="var(--color-text)" opacity="0.55" />
			<!-- metric rows: label bar + dot rating -->
			{#each [0, 1] as row (row)}
				{@const y = card.y + 62 + row * 26}
				<rect x={card.x + 12} {y} width="52" height="6" rx="3" fill={accent} opacity="0.28" />
				{#each [0, 1, 2, 3] as dot (dot)}
					<circle cx={card.x + 70 + dot * 9} cy={y + 3} r="3"
						fill={accent} opacity={dot < 3 - row ? 0.7 : 0.18} />
				{/each}
			{/each}
			<!-- footer line -->
			<rect x={card.x + 12} y={card.y + 108} width="80" height="5" rx="2.5" fill={accent} opacity="0.12" />
		</g>
	{/each}

	<!-- center "vs" node stays the signature -->
	<circle cx="190" cy="150" r="17" fill="var(--color-surface)" stroke={accent} stroke-width="1.5" />
	<text
		x="190"
		y="154"
		text-anchor="middle"
		font-size="11"
		font-weight="700"
		fill={accent}
		font-family="system-ui, sans-serif"
	>
		VS
	</text>
</svg>
