<script lang="ts">
	import { page } from '$app/state';
	import HubRail from '$lib/components/hub/HubRail.svelte';
	import { sectionsByGroup } from '$lib/components/hub/settings/settingsSections';
	let { children } = $props();
	const groups = sectionsByGroup();
	const active = $derived(page.params.section ?? '');
</script>

<div class="grid grid-cols-1 gap-6 md:grid-cols-[220px_1fr]">
	<HubRail {groups} basePath="/settings" {active} labelKey="settings.nav.label" />
	<div class="min-w-0 px-1 py-4">{@render children()}</div>
</div>
