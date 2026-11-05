<script lang="ts">
	/**
	 * /universities hero scene — a columned campus hall with flag, flanked
	 * by smaller halls and a floating ranking card.
	 */
	import { GraduationCap, Star } from 'lucide-svelte';

	let { accent = '#2563eb' }: { accent?: string } = $props();

	const columns = [164, 186, 208, 230];
	const BASE = 188;
</script>

<svg viewBox="0 0 380 224" class="h-full w-full" role="presentation">
	<circle cx="70" cy="50" r="70" fill={accent} opacity="0.06" />
	<circle cx="330" cy="170" r="60" fill={accent} opacity="0.06" />

	<!-- flag -->
	<line x1="203" y1="34" x2="203" y2="62" stroke={accent} stroke-width="2" />
	<polygon points="203,34 222,39 203,46" fill={accent} />

	<!-- pediment + roof -->
	<polygon points="140,90 203,58 266,90" fill={accent} opacity="0.85" />
	<polygon points="152,86 203,62 254,86" fill={accent} opacity="0.55" />
	<rect x="136" y="90" width="134" height="8" rx="2" fill={accent} />

	<!-- columns -->
	{#each columns as x}
		<rect x={x} y="102" width="12" height={BASE - 108} rx="2" fill={accent} opacity="0.3" />
		<rect x={x - 2} y="98" width="16" height="5" rx="2" fill={accent} opacity="0.5" />
	{/each}
	<rect x="140" y={BASE - 6} width="126" height="6" rx="2" fill={accent} opacity="0.5" />
	<rect x="132" y={BASE} width="142" height="7" rx="2" fill={accent} opacity="0.7" />
	<rect x="124" y={BASE + 7} width="158" height="6" rx="2" fill={accent} opacity="0.4" />

	<!-- side halls -->
	<g opacity="0.85">
		<rect x="52" y="120" width="56" height={BASE - 120} fill={accent} opacity="0.14" />
		<polygon points="46,120 80,102 114,120" fill={accent} opacity="0.3" />
		<rect x="64" y="132" width="8" height="8" fill={accent} opacity="0.4" />
		<rect x="86" y="132" width="8" height="8" fill={accent} opacity="0.4" />
		<rect x="64" y="150" width="8" height="8" fill={accent} opacity="0.4" />
		<rect x="86" y="150" width="8" height="8" fill={accent} opacity="0.4" />
	</g>
	<g opacity="0.85">
		<rect x="292" y="128" width="50" height={BASE - 128} fill={accent} opacity="0.14" />
		<polygon points="286,128 317,112 348,128" fill={accent} opacity="0.3" />
		<rect x="304" y="140" width="8" height="8" fill={accent} opacity="0.4" />
		<rect x="324" y="140" width="8" height="8" fill={accent} opacity="0.4" />
	</g>

	<!-- floating cards -->
	<g>
		<rect x="28" y="40" width="86" height="46" rx="10" fill="var(--color-surface)" stroke="var(--color-border)" transform="rotate(-5 71 63)" />
		<GraduationCap x={40} y={52} size={18} color={accent} />
		<rect x="62" y="56" width="40" height="6" rx="3" fill="var(--color-muted)" opacity="0.45" transform="rotate(-5 82 59)" />
		<rect x="44" y="72" width="56" height="5" rx="2.5" fill="var(--color-muted)" opacity="0.3" transform="rotate(-5 72 74)" />
	</g>
	<g>
		<rect x="284" y="44" width="80" height="46" rx="10" fill="var(--color-surface)" stroke="var(--color-border)" transform="rotate(6 324 67)" />
		<Star x={296} y={56} size={16} color={accent} />
		<rect x="316" y="58" width="36" height="6" rx="3" fill="var(--color-muted)" opacity="0.45" transform="rotate(6 334 61)" />
		<rect x="298" y="74" width="52" height="5" rx="2.5" fill="var(--color-muted)" opacity="0.3" transform="rotate(6 324 76)" />
	</g>
</svg>
