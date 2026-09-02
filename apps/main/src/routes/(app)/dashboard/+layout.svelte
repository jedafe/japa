<script lang="ts">
	import { page } from '$app/state';
	import HubRail from '$lib/components/hub/HubRail.svelte';
	import { viewsByGroup } from '$lib/components/hub/dashboard/dashboardViews';

	let { data, children } = $props();

	const menu = $derived(data.dashboardMenu);
	const groups = $derived(viewsByGroup(menu));

	// The [view] route populates page.params.view; the literal route folders
	// (tickets/bookmarks/media/spaces/organizations/profiles) do not, so derive
	// the active slug from the first path segment under /dashboard instead.
	const active = $derived(
		page.params.view ?? page.url.pathname.split('/dashboard/')[1]?.split('/')[0] ?? ''
	);
</script>

<svelte:head>
	<!-- ADR-060: Dashboard is private — not indexed, no ad slots. -->
	<meta name="robots" content="noindex, nofollow" />
</svelte:head>

<div class="mx-auto my-6 max-w-[1400px] grid grid-cols-1 gap-6 md:grid-cols-[220px_1fr] px-4">
	<HubRail {groups} basePath="/dashboard" {active} labelKey="dashboard.nav.label" />
	<div class="min-w-0 py-2">{@render children()}</div>
</div>
