<script lang="ts">
	/**
	 * /japa-stage hero scene — full halftone dotted world map
	 * (/illustrations/dotted-world-map.png) tinted lavender as the backdrop
	 * of the right side, a dotted flight arc rising into a solid plane icon,
	 * and solid teardrop pins — behind the team portrait cutout
	 * (/illustrations/japa-stage-hero.webp) that anchors the panel's right
	 * edge and is cropped by its bottom edge.
	 */
	let {
		dots = '#a5b4fc',
		pin = '#4f46e5',
		plane = '#5f9cf5'
	}: { dots?: string; pin?: string; plane?: string } = $props();
</script>

<div class="relative h-full w-full" role="presentation">
	<!-- Full dotted world map — masked image keeps the real halftone dot
	     field; a soft lavender→sand gradient mirrors the mockup tint. -->
	<div
		class="hero-map absolute top-0 right-0 aspect-[327/179] w-[56%] opacity-45"
		style="background: linear-gradient(100deg, {dots} 15%, #b8bdf2 55%, #ddccb4 100%); -webkit-mask-image: url('/illustrations/dotted-world-map.png'); mask-image: url('/illustrations/dotted-world-map.png'); -webkit-mask-size: 100% 100%; mask-size: 100% 100%; -webkit-mask-repeat: no-repeat; mask-repeat: no-repeat;"
	></div>

	<!-- Flight route — a dotted arc from the map's lower-left rising into
	     the top-right corner where the solid plane flies. -->
	<svg viewBox="0 0 380 224" class="absolute inset-0 h-full w-full" preserveAspectRatio="none" fill="none">
		<path
			d="M 228 84 Q 296 4 366 22"
			stroke={pin}
			stroke-width="3"
			stroke-linecap="round"
			stroke-dasharray="0.5 7"
			opacity="0.6"
		/>
	</svg>

	<!-- Solid plane at the arc's crest, nose east-northeast. -->
	<svg
		viewBox="0 0 24 24"
		class="absolute"
		style="left: 79%; top: 6%; width: 22px; height: 22px; transform: rotate(22deg);"
		aria-hidden="true"
	>
		<path
			fill={plane}
			d="M21.5 15.5v-2l-8.5-5V3.7a1.7 1.7 0 0 0-3.4 0v4.8l-8.5 5v2l8.5-2.7v5.6l-2.4 1.7v1.6l3.2-.9 3.2.9v-1.6l-2.4-1.7v-5.6l8.5 2.7z"
		/>
	</svg>

	<!-- Solid teardrop pins with white centres. -->
	<svg viewBox="0 0 24 24" class="absolute" style="left: 54.5%; top: 26%; width: 18px; height: 18px;" aria-hidden="true">
		<path fill={pin} d="M12 2C8.1 2 5 5.1 5 9c0 5.2 7 13 7 13s7-7.8 7-13c0-3.9-3.1-7-7-7z" />
		<circle cx="12" cy="9" r="2.4" fill="var(--color-surface)" />
	</svg>
	<svg viewBox="0 0 24 24" class="absolute" style="right: 8.5%; top: 13%; width: 22px; height: 22px;" aria-hidden="true">
		<path fill={pin} d="M12 2C8.1 2 5 5.1 5 9c0 5.2 7 13 7 13s7-7.8 7-13c0-3.9-3.1-7-7-7z" />
		<circle cx="12" cy="9" r="2.4" fill="var(--color-surface)" />
	</svg>
	<svg viewBox="0 0 24 24" class="absolute" style="left: 70%; top: 20%; width: 13px; height: 13px; opacity: 0.85;" aria-hidden="true">
		<path fill={pin} d="M12 2C8.1 2 5 5.1 5 9c0 5.2 7 13 7 13s7-7.8 7-13c0-3.9-3.1-7-7-7z" />
		<circle cx="12" cy="9" r="2.4" fill="var(--color-surface)" />
	</svg>

	<!-- The whole scene sits inside a `hidden lg:block` container, but a
	     plain <img> would still be fetched on mobile — the media-gated
	     source means the ~145 KB webp is only requested at lg+ viewports;
	     the 1px data-URI fallback keeps the img element valid elsewhere. -->
	<picture>
		<source srcset="/illustrations/japa-stage-hero.webp" media="(min-width: 1024px)" />
		<img
			src="data:image/gif;base64,R0lGODlhAQABAAAAACH5BAEKAAEALAAAAAABAAEAAAICTAEAOw=="
			alt=""
			class="absolute right-[1%] bottom-[-15%] h-[86%] w-auto object-contain object-bottom"
			width="1120"
			height="640"
			decoding="async"
			fetchpriority="low"
		/>
	</picture>
</div>

<style>
	/* Dark mode: soften the dotted map — lower opacity and a light blur
	   take the edge off the crisp lavender halftone. */
	:global([data-theme='dark']) .hero-map {
		opacity: 0.3;
		filter: blur(0.6px) saturate(0.85);
	}
</style>
