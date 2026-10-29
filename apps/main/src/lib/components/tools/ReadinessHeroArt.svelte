<script lang="ts">
	// Decorative travel illustration for the readiness wizard aside.
	// Colours derive from the app's theme tokens so the art follows the site
	// palette (and dark mode) instead of a fixed brand scheme.
	let { class: className = '' }: { class?: string } = $props();
</script>

<svg
	class={className}
	style="
		--a-ink: var(--color-primary);
		--a-ink-d: color-mix(in srgb, var(--color-primary) 78%, black);
		--a-ink-l: color-mix(in srgb, var(--color-primary) 55%, white);
		--a-land: color-mix(in srgb, var(--color-secondary) 45%, var(--color-surface));
		--a-land-2: color-mix(in srgb, var(--color-secondary) 65%, var(--color-surface));
		--a-globe: color-mix(in srgb, var(--color-secondary) 14%, var(--color-surface));
		--a-grid: color-mix(in srgb, var(--color-secondary) 35%, var(--color-surface));
		--a-accent: var(--color-secondary);
		--a-warn: var(--color-warning);
		--a-card: var(--color-surface);
		--a-line: var(--color-border);
		--a-soft: var(--color-surface-subtle);
	"
	viewBox="0 0 520 470"
	fill="none"
	xmlns="http://www.w3.org/2000/svg"
	role="presentation"
	aria-hidden="true"
>
	<!-- soft clouds -->
	<ellipse cx="80" cy="60" rx="60" ry="26" fill="var(--a-soft)" />
	<ellipse cx="440" cy="150" rx="70" ry="30" fill="var(--a-soft)" />
	<ellipse cx="360" cy="40" rx="44" ry="18" fill="var(--a-soft)" opacity="0.7" />

	<!-- skyline silhouette -->
	<g fill="var(--a-soft)">
		<rect x="66" y="300" width="20" height="70" />
		<rect x="92" y="282" width="14" height="88" />
		<rect x="112" y="312" width="24" height="58" />
		<rect x="142" y="292" width="12" height="78" />
		<path d="M60 300 L76 282 L92 300 Z" />
	</g>

	<!-- globe -->
	<circle cx="190" cy="200" r="128" fill="var(--a-globe)" />
	<circle cx="190" cy="200" r="128" stroke="var(--a-grid)" stroke-width="4" />
	<!-- continents (stylised) -->
	<path
		d="M150 108c14-10 40-14 58-8 10 3 16 12 12 22-4 12-20 14-26 24-6 12 2 26-6 36-9 11-26 8-34-2-8-11-4-24-10-34-5-9-14-12-12-24 1-7 10-9 18-14Z"
		fill="var(--a-land-2)"
	/>
	<path
		d="M236 132c12-4 30-2 38 8 6 9 2 20-6 26-10 7-14 16-12 26 2 12-6 22-16 24-11 2-20-6-20-18 0-10 8-16 8-26 0-11-12-16-10-28 1-6 10-10 18-12Z"
		fill="var(--a-land)"
	/>
	<path
		d="M140 226c10-6 26-4 32 6 6 9 2 20-4 28-7 9-8 20-16 26-7 5-16 0-18-9-3-10 4-18 2-28-2-9-4-17 4-23Z"
		fill="var(--a-land)"
	/>
	<path
		d="M238 246c12-2 24 6 26 18 2 11-6 20-14 26-9 7-12 16-20 18-9 3-16-4-14-14 2-9 10-12 12-20 2-9-2-18 10-28Z"
		fill="var(--a-land-2)"
	/>
	<!-- graticule arcs -->
	<path d="M78 200a128 128 0 0 1 224 0" stroke="var(--a-grid)" stroke-width="3" opacity="0.7" />
	<ellipse cx="190" cy="200" rx="52" ry="128" stroke="var(--a-grid)" stroke-width="3" opacity="0.6" />

	<!-- map pin on the globe -->
	<path
		d="M176 128c0-15 12-27 27-27s27 12 27 27c0 20-27 44-27 44s-27-24-27-44Z"
		fill="var(--a-warn)"
	/>
	<circle cx="203" cy="128" r="11" fill="var(--a-card)" />

	<!-- dashed flight route -->
	<path
		d="M226 148 C 300 110, 360 100, 436 64"
		stroke="var(--a-ink)"
		stroke-width="3"
		stroke-dasharray="2 12"
		stroke-linecap="round"
	/>

	<!-- jet -->
	<g transform="translate(432 44) rotate(-16)">
		<path d="M0 16 L52 0 L30 16 L52 32 Z" fill="var(--a-ink)" />
		<path d="M12 22 L30 16 L20 30 Z" fill="var(--a-ink-d)" />
	</g>

	<!-- floating checklist card -->
	<g transform="translate(402 208) rotate(7)">
		<rect width="104" height="128" rx="12" fill="var(--a-card)" stroke="var(--a-line)" stroke-width="2" />
		<circle cx="18" cy="26" r="8" fill="var(--a-accent)" />
		<path d="M14 26l3 3 6-6" stroke="var(--a-ink)" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round" />
		<rect x="34" y="21" width="52" height="9" rx="4.5" fill="var(--a-line)" />
		<circle cx="18" cy="58" r="8" fill="var(--a-accent)" />
		<path d="M14 58l3 3 6-6" stroke="var(--a-ink)" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round" />
		<rect x="34" y="53" width="44" height="9" rx="4.5" fill="var(--a-line)" />
		<circle cx="18" cy="90" r="8" fill="var(--a-soft)" />
		<rect x="34" y="85" width="56" height="9" rx="4.5" fill="var(--a-line)" />
	</g>

	<!-- passport -->
	<g transform="translate(286 264) rotate(-9)">
		<rect width="118" height="152" rx="12" fill="var(--a-ink)" />
		<rect x="10" y="10" width="98" height="132" rx="8" stroke="var(--a-ink-d)" stroke-width="2" fill="none" />
		<!-- emblem -->
		<circle cx="59" cy="62" r="24" stroke="var(--a-warn)" stroke-width="3" />
		<ellipse cx="59" cy="62" rx="10" ry="24" stroke="var(--a-warn)" stroke-width="2.4" />
		<path d="M35 62h48M40 48c12 6 26 6 38 0M40 76c12-6 26-6 38 0" stroke="var(--a-warn)" stroke-width="2.4" fill="none" />
		<rect x="30" y="104" width="58" height="8" rx="4" fill="var(--a-warn)" opacity="0.85" />
		<rect x="40" y="120" width="38" height="6" rx="3" fill="var(--a-ink-d)" />
	</g>

	<!-- boarding pass peeking from suitcase -->
	<g transform="translate(196 316) rotate(14)">
		<rect width="96" height="46" rx="8" fill="var(--a-card)" stroke="var(--a-line)" stroke-width="2" />
		<rect x="12" y="12" width="40" height="7" rx="3.5" fill="var(--a-accent)" />
		<rect x="12" y="26" width="56" height="6" rx="3" fill="var(--a-line)" />
		<path d="M78 6v34" stroke="var(--a-line)" stroke-width="2" stroke-dasharray="4 4" />
	</g>

	<!-- suitcase -->
	<g transform="translate(168 330)">
		<rect x="34" y="-16" width="44" height="20" rx="9" stroke="var(--a-ink-d)" stroke-width="7" fill="none" />
		<rect width="112" height="96" rx="14" fill="var(--a-ink-d)" />
		<rect x="14" y="0" width="8" height="96" fill="var(--a-ink)" opacity="0.45" />
		<rect x="90" y="0" width="8" height="96" fill="var(--a-ink)" opacity="0.45" />
		<rect x="46" y="0" width="20" height="96" fill="var(--a-ink)" opacity="0.25" />
		<circle cx="22" cy="100" r="7" fill="var(--a-ink-d)" />
		<circle cx="90" cy="100" r="7" fill="var(--a-ink-d)" />
	</g>

	<!-- foliage -->
	<g stroke-linecap="round">
		<path d="M64 436 C 60 396, 44 372, 22 356" stroke="var(--a-ink)" stroke-width="6" fill="none" />
		<ellipse cx="40" cy="380" rx="26" ry="11" transform="rotate(-52 40 380)" fill="var(--a-ink-d)" />
		<ellipse cx="30" cy="360" rx="22" ry="9" transform="rotate(-40 30 360)" fill="var(--a-ink-l)" />
		<ellipse cx="58" cy="408" rx="26" ry="11" transform="rotate(-70 58 408)" fill="var(--a-ink-d)" />
		<ellipse cx="76" cy="424" rx="20" ry="9" transform="rotate(-80 76 424)" fill="var(--a-ink-l)" />
	</g>
	<g stroke-linecap="round">
		<path d="M470 430 C 474 390, 490 366, 512 350" stroke="var(--a-ink)" stroke-width="6" fill="none" />
		<ellipse cx="494" cy="374" rx="26" ry="11" transform="rotate(-128 494 374)" fill="var(--a-ink-d)" />
		<ellipse cx="504" cy="352" rx="22" ry="9" transform="rotate(-140 504 352)" fill="var(--a-ink-l)" />
		<ellipse cx="476" cy="402" rx="24" ry="10" transform="rotate(-108 476 402)" fill="var(--a-ink-d)" />
	</g>

	<!-- ground accents -->
	<path d="M30 452 C 120 440, 400 440, 490 452" stroke="var(--a-line)" stroke-width="6" stroke-linecap="round" />
	<circle cx="120" cy="428" r="5" fill="var(--a-accent)" />
	<circle cx="428" cy="434" r="5" fill="var(--a-warn)" />
	<circle cx="330" cy="448" r="4" fill="var(--a-accent)" />
</svg>
