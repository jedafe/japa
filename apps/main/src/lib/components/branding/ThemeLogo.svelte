<script lang="ts">
	// Ref: japaconnect-engineering/sections/02-authentication-sessions.md
	// Theme-aware platform logo. Renders both light and dark variants; CSS
	// keyed on html[data-theme] shows the correct one without any client JS.
	// Falls back to the bundled homepage wordmark when settings are empty —
	// never the square icon mark (that's reserved for favicon contexts).
	import { page } from '$app/state';
	import { mediaSized } from '@japaconnect/ui';
	import wordmarkLight from '#lib/assets/branding/wordmark-light.webp';
	import wordmarkDark from '#lib/assets/branding/wordmark-dark.webp';

	let {
		logoUrl = '',
		logoDarkUrl = '',
		logoWidth = 0,
		logoHeight = 0,
		logoDarkWidth = 0,
		logoDarkHeight = 0,
		alt = '',
		class: className = ''
	}: {
		logoUrl?: string;
		logoDarkUrl?: string;
		logoWidth?: number;
		logoHeight?: number;
		logoDarkWidth?: number;
		logoDarkHeight?: number;
		alt?: string;
		class?: string;
	} = $props();

	const lightSrc = $derived(mediaSized(logoUrl, { h: 56 }) || wordmarkLight);
	const darkSrc = $derived(mediaSized(logoDarkUrl || logoUrl, { h: 56 }) || wordmarkDark);
	// Intrinsic dims of the bundled defaults — reserves the aspect ratio so
	// theme-swap and image fetch don't shift layout. Admin-supplied logo URLs
	// keep their own proportions via w-auto anyway.
	// Admin-supplied logo URLs emit their intrinsic dims — props win, else the
	// values are read from page.data.siteSettings (appearance.logo_*_width/
	// height, populated by resolveLogoDims at write time). Covers every call
	// site without threading props through shell components.
	const site = $derived(page.data?.siteSettings as Record<string, unknown> | undefined);
	const lw = $derived(logoWidth || Number(site?.['appearance.logo_width']) || 0);
	const lh = $derived(logoHeight || Number(site?.['appearance.logo_height']) || 0);
	const ldw = $derived(logoDarkWidth || Number(site?.['appearance.logo_dark_width']) || 0);
	const ldh = $derived(logoDarkHeight || Number(site?.['appearance.logo_dark_height']) || 0);
	// Custom logos uploaded before resolveLogoDims existed may have no
	// recorded intrinsic dims — emit the rendered display box (the
	// `h-9 w-auto max-w-[120px]` topbar slot) so the img still carries
	// explicit width/height for CLS + Lighthouse instead of none.
	const fallbackDims = { width: 120, height: 36 };
	const lightDims = $derived(
		logoUrl
			? lw && lh
				? { width: lw, height: lh }
				: fallbackDims
			: { width: 590, height: 384 }
	);
	// A custom dark logo only emits dims when ITS OWN are known — borrowing
	// the light logo's dims would reserve the wrong aspect for a different
	// image. When darkSrc falls back to logoUrl (same image), light dims apply.
	const darkDims = $derived(
		logoDarkUrl
			? ldw && ldh
				? { width: ldw, height: ldh }
				: fallbackDims
			: logoUrl
				? lw && lh
					? { width: lw, height: lh }
					: fallbackDims
				: { width: 794, height: 384 }
	);
</script>

<img
	src={lightSrc}
	{alt}
	class="{className} theme-logo-light"
	loading="eager"
	decoding="async"
	{...lightDims}
/>
<img
	src={darkSrc}
	{alt}
	class="{className} theme-logo-dark"
	loading="eager"
	decoding="async"
	{...darkDims}
/>

<style>
	.theme-logo-light {
		display: block;
	}
	.theme-logo-dark {
		display: none;
	}
	:global([data-theme='dark']) .theme-logo-light {
		display: none;
	}
	:global([data-theme='dark']) .theme-logo-dark {
		display: block;
	}
</style>
