<script lang="ts">
	import { resolveView } from '$lib/components/hub/dashboard/dashboardViews';
	let { data } = $props();
	const meta = $derived(resolveView(data.view));
	const ViewComponent = $derived(meta?.component ?? null);
</script>

{#if ViewComponent}
	<ViewComponent data={data.viewData} />
{/if}
