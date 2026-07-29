<script lang="ts">
	import { page } from '$app/state';
	import HubRail from '$lib/components/hub/HubRail.svelte';
	import { viewsByGroup } from '$lib/components/hub/dashboard/dashboardViews';
	let { children } = $props();
	const groups = viewsByGroup();
	// The [view] route populates page.params.view; the literal route folders
	// (tickets/bookmarks/media/spaces/organizations) do not, so derive the
	// active slug from the first path segment under /dashboard instead.
	const active = $derived(
		page.params.view ?? page.url.pathname.split('/dashboard/')[1]?.split('/')[0] ?? ''
	);
</script>

<svelte:head>
	<!-- ADR-060: Dashboard is private — not indexed, no ad slots. -->
	<meta name="robots" content="noindex, nofollow" />
</svelte:head>

<div class="grid grid-cols-1 gap-6 md:grid-cols-[220px_1fr]">
	<HubRail {groups} basePath="/dashboard" {active} labelKey="dashboard.nav.label" />
	<div class="min-w-0 px-1 py-4">{@render children()}</div>
</div>
