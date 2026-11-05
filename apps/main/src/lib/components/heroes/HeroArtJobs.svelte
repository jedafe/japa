<script lang="ts">
	/**
	 * /jobs hero scene — a briefcase surrounded by floating listing cards
	 * and a hired check badge.
	 */
	import { BadgeCheck, Briefcase, MapPin } from 'lucide-svelte';

	let { accent = '#ea580c' }: { accent?: string } = $props();
</script>

<svg viewBox="0 0 380 224" class="h-full w-full" role="presentation">
	<circle cx="80" cy="60" r="80" fill={accent} opacity="0.06" />
	<circle cx="320" cy="180" r="70" fill={accent} opacity="0.05" />

	<!-- briefcase -->
	<g transform="translate(152 84)">
		<rect x="22" y="-14" width="32" height="16" rx="7" fill="none" stroke={accent} stroke-width="4" />
		<rect x="0" y="0" width="76" height="56" rx="10" fill={accent} />
		<rect x="0" y="0" width="76" height="22" rx="10" fill={accent} opacity="0.75" />
		<rect x="30" y="18" width="16" height="10" rx="3" fill="var(--hero-contrast)" opacity="0.85" />
	</g>

	<!-- listing cards -->
	<g>
		<rect x="36" y="46" width="108" height="52" rx="10" fill="var(--color-surface)" stroke="var(--color-border)" transform="rotate(-6 90 72)" />
		<rect x="48" y="58" width="24" height="24" rx="6" fill={accent} opacity="0.16" />
		<Briefcase x={53} y={63} size={14} color={accent} />
		<rect x="78" y="60" width="52" height="6" rx="3" fill="var(--color-muted)" opacity="0.5" transform="rotate(-6 104 63)" />
		<rect x="78" y="72" width="36" height="5" rx="2.5" fill="var(--color-muted)" opacity="0.3" transform="rotate(-6 96 74)" />
	</g>
	<g>
		<rect x="248" y="50" width="110" height="54" rx="10" fill="var(--color-surface)" stroke="var(--color-border)" transform="rotate(5 303 77)" />
		<rect x="260" y="62" width="24" height="24" rx="6" fill={accent} opacity="0.16" />
		<MapPin x={265} y={67} size={14} color={accent} />
		<rect x="290" y="64" width="54" height="6" rx="3" fill="var(--color-muted)" opacity="0.5" transform="rotate(5 317 67)" />
		<rect x="290" y="76" width="38" height="5" rx="2.5" fill="var(--color-muted)" opacity="0.3" transform="rotate(5 309 78)" />
	</g>
	<g>
		<rect x="252" y="140" width="100" height="50" rx="10" fill="var(--color-surface)" stroke="var(--color-border)" transform="rotate(-4 302 165)" />
		<rect x="264" y="152" width="20" height="20" rx="5" fill={accent} opacity="0.16" />
		<rect x="290" y="154" width="48" height="6" rx="3" fill="var(--color-muted)" opacity="0.5" transform="rotate(-4 314 157)" />
		<rect x="290" y="166" width="34" height="5" rx="2.5" fill="var(--color-muted)" opacity="0.3" transform="rotate(-4 307 168)" />
	</g>

	<!-- hired badge -->
	<circle cx="112" cy="156" r="20" fill="var(--color-surface)" stroke="var(--color-border)" />
	<BadgeCheck x={100} y={144} size={24} color={accent} />

	<!-- connectors -->
	<path d="M 140 84 Q 146 76 152 80" fill="none" stroke={accent} stroke-width="1.4" stroke-dasharray="4 4" opacity="0.55" />
	<path d="M 230 86 Q 240 80 248 82" fill="none" stroke={accent} stroke-width="1.4" stroke-dasharray="4 4" opacity="0.55" />
	<path d="M 126 168 Q 190 190 250 170" fill="none" stroke={accent} stroke-width="1.4" stroke-dasharray="4 4" opacity="0.45" />
</svg>
