<script lang="ts">
	/**
	 * /scholarships hero scene — mortarboard, diploma scroll and floating
	 * award cards with coins.
	 */
	import { Award, Medal } from 'lucide-svelte';

	let { accent = '#d97706' }: { accent?: string } = $props();
</script>

<svg viewBox="0 0 380 224" class="h-full w-full" role="presentation">
	<circle cx="300" cy="60" r="90" fill={accent} opacity="0.07" />
	<circle cx="70" cy="190" r="60" fill={accent} opacity="0.05" />

	<!-- mortarboard -->
	<g transform="translate(150 66)">
		<polygon points="60,0 118,26 60,52 2,26" fill={accent} />
		<polygon points="60,8 104,27 60,46 16,27" fill={accent} opacity="0.75" />
		<path d="M 34 38 L 34 58 Q 60 72 86 58 L 86 38 Q 60 50 34 38 Z" fill={accent} opacity="0.55" />
		<line x1="118" y1="26" x2="118" y2="58" stroke={accent} stroke-width="2.5" />
		<circle cx="118" cy="62" r="5" fill={accent} />
	</g>

	<!-- diploma scroll -->
	<g transform="translate(60 150)">
		<rect x="0" y="0" width="74" height="26" rx="13" fill="var(--color-surface)" stroke="var(--color-border)" />
		<rect x="30" y="-4" width="14" height="34" rx="4" fill={accent} />
		<polygon points="37,30 32,44 37,40 42,44" fill={accent} />
	</g>

	<!-- award cards -->
	<g>
		<rect x="278" y="112" width="76" height="54" rx="10" fill="var(--color-surface)" stroke="var(--color-border)" transform="rotate(-6 316 139)" />
		<Medal x={298} y={124} size={20} color={accent} />
		<rect x="292" y="148" width="42" height="6" rx="3" fill="var(--color-muted)" opacity="0.45" transform="rotate(-6 313 151)" />
	</g>
	<g>
		<rect x="44" y="52" width="70" height="50" rx="10" fill="var(--color-surface)" stroke="var(--color-border)" transform="rotate(5 79 77)" />
		<Award x={62} y={62} size={20} color={accent} />
		<rect x="58" y="86" width="40" height="6" rx="3" fill="var(--color-muted)" opacity="0.45" transform="rotate(5 78 89)" />
	</g>

	<!-- coins -->
	<circle cx="330" cy="52" r="10" fill={accent} opacity="0.3" />
	<circle cx="330" cy="52" r="6" fill="none" stroke={accent} stroke-width="1.5" opacity="0.7" />
	<circle cx="140" cy="180" r="8" fill={accent} opacity="0.3" />
	<circle cx="140" cy="180" r="4.5" fill="none" stroke={accent} stroke-width="1.2" opacity="0.7" />
	<circle cx="352" cy="180" r="6" fill={accent} opacity="0.35" />

	<!-- dashed route between scenes -->
	<path d="M 122 84 Q 140 60 156 68" fill="none" stroke={accent} stroke-width="1.4" stroke-dasharray="4 4" opacity="0.55" />
	<path d="M 268 100 Q 290 104 296 116" fill="none" stroke={accent} stroke-width="1.4" stroke-dasharray="4 4" opacity="0.55" />
</svg>
