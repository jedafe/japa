<script lang="ts">
	// Theme-aware platform logo. Renders both light and dark variants; CSS
	// keyed on html[data-theme] shows the correct one without any client JS.
	// Falls back to the bundled /logos/*.png assets when settings are empty.
	let {
		logoUrl = '',
		logoDarkUrl = '',
		alt = '',
		class: className = ''
	}: {
		logoUrl?: string;
		logoDarkUrl?: string;
		alt?: string;
		class?: string;
	} = $props();

	const lightSrc = $derived(logoUrl || '/logos/logo-light.png');
	const darkSrc = $derived(logoDarkUrl || logoUrl || '/logos/logo-dark.png');
</script>

<img
	src={lightSrc}
	{alt}
	class="{className} theme-logo-light"
	loading="eager"
	decoding="async"
/>
<img
	src={darkSrc}
	{alt}
	class="{className} theme-logo-dark"
	loading="eager"
	decoding="async"
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
