<script lang="ts">
	import { env } from '$env/dynamic/public';
	import { ExternalLink } from 'lucide-svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import { t } from '$lib/stores/i18n.svelte';

	// Self-hosted Umami dashboard embedded in the Control Panel (§40).
	// The iframe provides quick access; the link opens the dashboard in a new tab
	// in case the iframe is blocked by X-Frame-Options in a given deployment.
	const umamiUrl = $derived((env.PUBLIC_JC_UMAMI_URL ?? '').replace(/\/$/, ''));
	const dashboardUrl = $derived(umamiUrl ? `${umamiUrl}/dashboard` : '');

	let frameError = $state(false);
</script>

<PageHeader title={t('analytics.title', 'Analytics')} description={t('analytics.description', 'Umami page-view dashboard. Login with the Umami admin credentials if the dashboard is not already authenticated.')} />

<div class="space-y-4">
	{#if dashboardUrl}
		<a
			href={dashboardUrl}
			target="_blank"
			rel="noopener noreferrer"
			class="inline-flex items-center gap-2 rounded-md bg-accent px-3 py-2 text-sm font-medium text-accent-foreground transition-colors hover:bg-accent/90"
		>
			{t('analytics.open_dashboard', 'Open Umami dashboard')}
			<ExternalLink class="size-4 rtl:-scale-x-100" />
		</a>

		{#if frameError}
			<div class="border-border-card rounded-lg border p-6">
				<p class="text-muted-foreground text-sm">
					{t('analytics.embed_failed', 'The dashboard could not be embedded (blocked by the browser or Umami is unreachable). Use the button above to open it in a new tab.')}
				</p>
			</div>
		{:else}
			<div class="border-border-card overflow-hidden rounded-lg border">
				<iframe
					src={dashboardUrl}
					title={t('analytics.iframe_title', 'Umami analytics dashboard')}
					class="h-[calc(100vh-14rem)] min-h-[480px] w-full bg-background"
					sandbox="allow-same-origin allow-scripts allow-forms allow-popups"
					loading="lazy"
					onerror={() => (frameError = true)}
				></iframe>
			</div>
		{/if}
	{:else}
		<div class="border-border-card rounded-lg border p-6">
			<p class="text-muted-foreground text-sm">
				{t('analytics.not_configured', 'Umami is not configured. Set the Umami URL and site ID environment variables to enable this dashboard.')}
			</p>
		</div>
	{/if}
</div>
