<script lang="ts">
	import { t } from '$lib/i18n';
</script>

<svelte:head>
	<!-- Legacy alias — always 301s to /deal/{provider}/{slug}; never indexed. -->
	<meta name="robots" content="noindex, nofollow" />
</svelte:head>

<!-- Unreachable: +page.server.ts always 301s to the canonical /deal/{provider}/{slug}
	URL or throws 404 before this component renders (§55 Deal Pages). -->
<p class="p-4 text-sm text-[var(--color-text-subtle)]">
	{t('coupon.redirecting', 'Redirecting…')}
</p>
