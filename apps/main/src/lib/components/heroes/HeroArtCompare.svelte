<script lang="ts">
	/**
	 * /compare + /comparisons hero scene — two floating profile cards pinned
	 * against each other with a center "vs" node and connector rails.
	 */
	let { accent = '#3b49df' }: { accent?: string } = $props();
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

	<!-- connector rail between the two cards -->
	<line x1="128" y1="112" x2="252" y2="112" stroke={accent} stroke-width="1.5" stroke-dasharray="4 4" opacity="0.45" />

	<!-- left card -->
	<g transform="rotate(-4 108 112)">
		<rect x="56" y="52" width="108" height="120" rx="14" fill="var(--color-surface)" stroke="var(--color-border)" />
		<rect x="56" y="52" width="108" height="120" rx="14" fill={accent} opacity="0.06" />
		<circle cx="86" cy="80" r="14" fill={accent} opacity="0.22" />
		<rect x="104" y="70" width="44" height="8" rx="4" fill={accent} opacity="0.32" />
		<rect x="104" y="82" width="30" height="6" rx="3" fill={accent} opacity="0.18" />
		<rect x="68" y="106" width="84" height="6" rx="3" fill={accent} opacity="0.16" />
		<rect x="68" y="118" width="84" height="6" rx="3" fill={accent} opacity="0.10" />
		<rect x="68" y="136" width="60" height="6" rx="3" fill={accent} opacity="0.24" />
		<rect x="68" y="148" width="72" height="6" rx="3" fill={accent} opacity="0.10" />
	</g>

	<!-- right card -->
	<g transform="rotate(4 272 112)">
		<rect x="216" y="52" width="108" height="120" rx="14" fill="var(--color-surface)" stroke="var(--color-border)" />
		<rect x="216" y="52" width="108" height="120" rx="14" fill={accent} opacity="0.06" />
		<circle cx="246" cy="80" r="14" fill={accent} opacity="0.22" />
		<rect x="264" y="70" width="44" height="8" rx="4" fill={accent} opacity="0.32" />
		<rect x="264" y="82" width="30" height="6" rx="3" fill={accent} opacity="0.18" />
		<rect x="228" y="106" width="84" height="6" rx="3" fill={accent} opacity="0.10" />
		<rect x="228" y="118" width="84" height="6" rx="3" fill={accent} opacity="0.22" />
		<rect x="228" y="136" width="60" height="6" rx="3" fill={accent} opacity="0.10" />
		<rect x="228" y="148" width="72" height="6" rx="3" fill={accent} opacity="0.20" />
	</g>

	<!-- center "vs" node -->
	<circle cx="190" cy="112" r="17" fill="var(--color-surface)" stroke={accent} stroke-width="1.5" />
	<text
		x="190"
		y="116"
		text-anchor="middle"
		font-size="11"
		font-weight="700"
		fill={accent}
		font-family="system-ui, sans-serif"
	>
		VS
	</text>

	<!-- floating dimension ticks -->
	{#each [0, 1, 2] as i}
		<rect x={170 + i * 20} y={36} width="12" height="4" rx="2" fill={accent} opacity={0.35 - i * 0.08} />
		<rect x={170 + i * 20} y={184} width="12" height="4" rx="2" fill={accent} opacity={0.2 + i * 0.08} />
	{/each}
</svg>
