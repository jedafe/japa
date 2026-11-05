<script lang="ts">
	/**
	 * /sources hero scene — a stack of publisher cards broadcasting into
	 * an RSS hub.
	 */
	import { Newspaper, Rss } from 'lucide-svelte';

	let { accent = '#e11d48' }: { accent?: string } = $props();
</script>

<svg viewBox="0 0 380 224" class="h-full w-full" role="presentation">
	<circle cx="310" cy="50" r="90" fill={accent} opacity="0.06" />
	<circle cx="60" cy="190" r="60" fill={accent} opacity="0.05" />

	<!-- stacked publisher cards -->
	<g transform="rotate(-7 120 120)">
		<rect x="58" y="66" width="124" height="86" rx="12" fill="var(--color-surface)" stroke="var(--color-border)" />
		<rect x="70" y="80" width="30" height="30" rx="6" fill={accent} opacity="0.18" />
		<Newspaper x={76} y={86} size={18} color={accent} />
		<rect x="108" y="84" width="60" height="7" rx="3.5" fill="var(--color-muted)" opacity="0.5" />
		<rect x="108" y="97" width="44" height="6" rx="3" fill="var(--color-muted)" opacity="0.3" />
		<rect x="70" y="120" width="98" height="5" rx="2.5" fill="var(--color-muted)" opacity="0.25" />
		<rect x="70" y="131" width="84" height="5" rx="2.5" fill="var(--color-muted)" opacity="0.25" />
	</g>
	<g transform="rotate(6 250 140)">
		<rect x="190" y="98" width="118" height="78" rx="12" fill="var(--color-surface)" stroke="var(--color-border)" />
		<circle cx="214" cy="124" r="13" fill={accent} opacity="0.18" />
		<circle cx="214" cy="124" r="6" fill={accent} opacity="0.5" />
		<rect x="234" y="114" width="58" height="7" rx="3.5" fill="var(--color-muted)" opacity="0.5" />
		<rect x="234" y="127" width="40" height="6" rx="3" fill="var(--color-muted)" opacity="0.3" />
		<rect x="202" y="146" width="94" height="5" rx="2.5" fill="var(--color-muted)" opacity="0.25" />
		<rect x="202" y="157" width="70" height="5" rx="2.5" fill="var(--color-muted)" opacity="0.25" />
	</g>

	<!-- RSS hub -->
	<circle cx="330" cy="66" r="22" fill={accent} />
	<circle cx="330" cy="66" r="28" fill="none" stroke={accent} stroke-width="1.2" opacity="0.35" stroke-dasharray="3 5" />
	<Rss x={319} y={55} size={22} color="var(--hero-contrast)" />

	<!-- broadcast arcs -->
	<path d="M 306 78 Q 270 92 258 104" fill="none" stroke={accent} stroke-width="1.4" stroke-dasharray="4 4" opacity="0.55" />
	<path d="M 312 50 Q 240 40 186 78" fill="none" stroke={accent} stroke-width="1.4" stroke-dasharray="4 4" opacity="0.55" />

	<!-- signal dots -->
	<circle cx="52" cy="52" r="4" fill={accent} opacity="0.45" />
	<circle cx="176" cy="182" r="4" fill={accent} opacity="0.45" />
	<circle cx="344" cy="150" r="4" fill={accent} opacity="0.45" />
	<circle cx="36" cy="140" r="6" fill="none" stroke={accent} stroke-width="1.4" opacity="0.5" />
</svg>
