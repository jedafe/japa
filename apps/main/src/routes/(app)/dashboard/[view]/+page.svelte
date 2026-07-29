<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { resolveView } from '$lib/components/hub/dashboard/dashboardViews';
	let { data } = $props();
	const meta = $derived(resolveView(data.view));
	const ViewComponent = $derived(meta?.component ?? null);

	// L2 — Client-side auth guard: if the user is not authenticated (e.g. session
	// expired during a client-side navigation), redirect to login immediately
	// rather than rendering a partial/empty dashboard view.
	$effect(() => {
		if (!page.data.user) {
			goto(`/login?redirect=${encodeURIComponent(page.url.pathname)}`);
		}
	});
</script>

{#if ViewComponent && page.data.user}
	<ViewComponent data={data.viewData} />
{/if}
