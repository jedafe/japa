<script lang="ts">
	import { env } from '$env/dynamic/public';
	import { ExternalLink } from 'lucide-svelte';

	// Self-hosted Umami dashboard embedded in the Control Panel (§40).
	// The iframe provides quick access; the link opens the dashboard in a new tab
	// in case the iframe is blocked by X-Frame-Options in a given deployment.
	const umamiUrl = $derived((env.PUBLIC_JC_UMAMI_URL ?? '').replace(/\/$/, ''));
	const dashboardUrl = $derived(umamiUrl ? `${umamiUrl}/dashboard` : '');
</script>

<div class="space-y-4">
	<div>
		<h1 class="text-2xl font-bold tracking-tight">Analytics</h1>
		<p class="text-foreground-alt text-sm">
			Umami page-view dashboard. Login with the Umami admin credentials if the
			dashboard is not already authenticated.
		</p>
	</div>

	{#if dashboardUrl}
		<a
			href={dashboardUrl}
			target="_blank"
			rel="noopener noreferrer"
			class="inline-flex items-center gap-2 rounded-md bg-accent px-3 py-2 text-sm font-medium text-accent-foreground transition-colors hover:bg-accent/90"
		>
			Open Umami dashboard
			<ExternalLink class="size-4" />
		</a>

		<div class="border-border-card overflow-hidden rounded-lg border">
			<iframe
				src={dashboardUrl}
				title="Umami analytics dashboard"
				class="h-[calc(100vh-14rem)] w-full bg-background"
				sandbox="allow-same-origin allow-scripts allow-forms allow-popups"
				loading="lazy"
			></iframe>
		</div>
	{:else}
		<div class="border-border-card rounded-lg border p-6">
			<p class="text-muted-foreground text-sm">
				Umami is not configured. Set <code>PUBLIC_JC_UMAMI_URL</code> and
				<code>PUBLIC_JC_UMAMI_SITE_ID_CONTROL</code> to enable this dashboard.
			</p>
		</div>
	{/if}
</div>
