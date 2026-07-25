<script lang="ts">
	import { page } from '$app/state';
	import HubRail from '$lib/components/hub/HubRail.svelte';
	import { viewsByGroup } from '$lib/components/hub/dashboard/dashboardViews';
	let { children } = $props();
	const groups = viewsByGroup();
	const active = $derived(page.params.view ?? '');
</script>

<div class="grid grid-cols-1 gap-6 md:grid-cols-[220px_1fr]">
	<HubRail {groups} basePath="/dashboard" {active} labelKey="dashboard.nav.label" />
	<div class="min-w-0 px-1 py-4">{@render children()}</div>
</div>
