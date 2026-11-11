<script lang="ts">
	// Theme-aware platform logo. Renders both light and dark variants; CSS
	// keyed on html[data-theme] shows the correct one without any client JS.
	// Falls back to the bundled /logos/*.png assets when settings are empty.
	import { page } from '$app/state';
	import { mediaSized } from '@japaconnect/ui';

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

	const lightSrc = $derived(mediaSized(logoUrl, { h: 56 }) || '/logos/logo-light.png');
	const darkSrc = $derived(mediaSized(logoDarkUrl || logoUrl, { h: 56 }) || '/logos/logo-dark.png');
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
	// `h-7 w-auto max-w-[140px]` header slot) so the img still carries
	// explicit width/height for CLS + Lighthouse instead of none.
	const fallbackDims = { width: 140, height: 28 };
	const lightDims = $derived(
		logoUrl
			? lw && lh
				? { width: lw, height: lh }
				: fallbackDims
			: { width: 512, height: 488 }
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
				: { width: 512, height: 492 }
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
